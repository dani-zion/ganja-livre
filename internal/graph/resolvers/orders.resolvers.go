package resolvers

import (
	"context"
	"fmt"
	"time"

	"github.com/dani-zion/ganja-livre/internal/graph/model"
	"github.com/dani-zion/ganja-livre/internal/middleware"
	dbmodel "github.com/dani-zion/ganja-livre/internal/model"
	"github.com/dani-zion/ganja-livre/internal/temporal/workflows"
	"github.com/dani-zion/ganja-livre/internal/validator"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.temporal.io/sdk/client"
)

// PlaceOrder creates an order and kicks off the Temporal workflow.
func (r *mutationResolver) PlaceOrder(ctx context.Context, input model.PlaceOrderInput) (*model.Order, error) {
	claims, err := middleware.RequireRole(ctx, model.UserRoleCustomer, model.UserRoleAdmin)
	if err != nil {
		return nil, err
	}

	if len(input.Items) == 0 {
		return nil, fmt.Errorf("order must contain at least one item")
	}

	buyerID, _ := primitive.ObjectIDFromHex(claims.UserID)

	var orderItems []*model.OrderItem
	var totalAmount float64
	var workflowItems []workflows.OrderItemInput

	for _, item := range input.Items {
		if err = validator.Quantity(item.Quantity); err != nil {
			return nil, err
		}

		pid, err := primitive.ObjectIDFromHex(item.ProductID)
		if err != nil {
			return nil, fmt.Errorf("invalid product id: %s", item.ProductID)
		}

		var product dbmodel.Product
		if err = r.cols.Products.FindOne(ctx, bson.M{"_id": pid, "is_active": true}).Decode(&product); err != nil {
			return nil, fmt.Errorf("product not found: %s", item.ProductID)
		}

		subtotal := product.Price * float64(item.Quantity)
		orderItems = append(orderItems, &model.OrderItem{
			ProductID: pid.Hex(),
			Quantity:  item.Quantity,
			UnitPrice: product.Price,
			Subtotal:  subtotal,
		})
		totalAmount += subtotal
		workflowItems = append(workflowItems, workflows.OrderItemInput{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
		})
	}

	workflowID := fmt.Sprintf("order-%s", uuid.New().String())
	now := time.Now().UTC()

	order := &model.Order{
		ID:                 primitive.NewObjectID().Hex(),
		BuyerID:            buyerID.Hex(),
		Items:              orderItems,
		TotalAmount:        totalAmount,
		Status:             model.OrderStatusPending,
		ShippingAddress:    toAddress(input.ShippingAddress),
		TemporalWorkflowID: workflowID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	orderOID, _ := primitive.ObjectIDFromHex(order.ID)
	dbOrder := bson.M{
		"_id":                  orderOID,
		"buyer_id":             buyerID,
		"items":                orderItems,
		"total_amount":         totalAmount,
		"status":               string(model.OrderStatusPending),
		"shipping_address":     toAddress(input.ShippingAddress),
		"temporal_workflow_id": workflowID,
		"created_at":           now,
		"updated_at":           now,
	}

	if _, err = r.cols.Orders.InsertOne(ctx, dbOrder); err != nil {
		return nil, errInternal
	}

	_, err = r.temporal.ExecuteWorkflow(ctx,
		client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: workflows.TaskQueueName,
		},
		"OrderWorkflow",
		workflows.OrderWorkflowInput{
			OrderID: order.ID,
			BuyerID: claims.UserID,
			Amount:  totalAmount,
			Items:   workflowItems,
		},
	)
	if err != nil {
		_, _ = r.cols.Orders.DeleteOne(ctx, bson.M{"_id": orderOID})
		return nil, fmt.Errorf("failed to start order workflow: %w", err)
	}

	return order, nil
}

// CancelOrder sends a cancellation signal to the running workflow.
func (r *mutationResolver) CancelOrder(ctx context.Context, id string) (*model.Order, error) {
	claims, err := middleware.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, errNotFound
	}

	filter := bson.M{"_id": oid}
	if hasRole(claims.Roles, model.UserRoleCustomer) && !hasRole(claims.Roles, model.UserRoleAdmin) {
		buyerID, _ := primitive.ObjectIDFromHex(claims.UserID)
		filter["buyer_id"] = buyerID
	}

	var order model.Order
	if err = r.cols.Orders.FindOne(ctx, filter).Decode(&order); err != nil {
		return nil, errNotFound
	}

	if order.Status != model.OrderStatusPending && order.Status != model.OrderStatusPaymentProcessing {
		return nil, fmt.Errorf("order cannot be cancelled in status: %s", order.Status)
	}

	if err = r.temporal.SignalWorkflow(ctx,
		order.TemporalWorkflowID, "",
		workflows.SignalOrderCancelled, nil,
	); err != nil {
		return nil, fmt.Errorf("failed to send cancellation signal: %w", err)
	}

	return &order, nil
}

// UpdateOrderStatus is an admin operation.
func (r *mutationResolver) UpdateOrderStatus(ctx context.Context, id string, status model.OrderStatus) (*model.Order, error) {
	if _, err := middleware.RequireRole(ctx, model.UserRoleAdmin); err != nil {
		return nil, err
	}

	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, errNotFound
	}

	now := time.Now().UTC()
	after := options.After
	opts := options.FindOneAndUpdate().SetReturnDocument(after)
	var updated model.Order
	if err = r.cols.Orders.FindOneAndUpdate(ctx,
		bson.M{"_id": oid},
		bson.M{"$set": bson.M{"status": string(status), "updated_at": now}},
		opts,
	).Decode(&updated); err != nil {
		return nil, errNotFound
	}
	return &updated, nil
}

// Me returns the currently authenticated user.
func (r *queryResolver) Me(ctx context.Context) (*model.User, error) {
	claims, err := middleware.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	uid, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return nil, errNotFound
	}

	var user dbmodel.User
	if err = r.cols.Users.FindOne(ctx, bson.M{"_id": uid}).Decode(&user); err != nil {
		return nil, errNotFound
	}

	return &model.User{
		ID:        user.ID.Hex(),
		Email:     user.Email,
		Name:      user.Name,
		Roles:     toGraphRoles(user.Roles),
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}, nil
}

// MyOrders returns orders belonging to the authenticated user.
func (r *queryResolver) MyOrders(ctx context.Context) ([]*model.Order, error) {
	claims, err := middleware.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	buyerID, _ := primitive.ObjectIDFromHex(claims.UserID)
	cursor, err := r.cols.Orders.Find(ctx, bson.M{"buyer_id": buyerID})
	if err != nil {
		return nil, errInternal
	}
	defer cursor.Close(ctx)

	var orders []*model.Order
	if err = cursor.All(ctx, &orders); err != nil {
		return nil, errInternal
	}
	return orders, nil
}

// Order returns a single order by ID.
func (r *queryResolver) Order(ctx context.Context, id string) (*model.Order, error) {
	claims, err := middleware.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, errNotFound
	}

	filter := bson.M{"_id": oid}
	if hasRole(claims.Roles, model.UserRoleCustomer) && !hasRole(claims.Roles, model.UserRoleAdmin) {
		buyerID, _ := primitive.ObjectIDFromHex(claims.UserID)
		filter["buyer_id"] = buyerID
	}

	var order model.Order
	if err = r.cols.Orders.FindOne(ctx, filter).Decode(&order); err != nil {
		return nil, errNotFound
	}
	return &order, nil
}

// AllOrders returns all orders (admin only).
func (r *queryResolver) AllOrders(ctx context.Context, status *model.OrderStatus) ([]*model.Order, error) {
	if _, err := middleware.RequireRole(ctx, model.UserRoleAdmin); err != nil {
		return nil, err
	}

	query := bson.M{}
	if status != nil {
		query["status"] = string(*status)
	}

	cursor, err := r.cols.Orders.Find(ctx, query)
	if err != nil {
		return nil, errInternal
	}
	defer cursor.Close(ctx)

	var orders []*model.Order
	if err = cursor.All(ctx, &orders); err != nil {
		return nil, errInternal
	}
	return orders, nil
}

// Document is the resolver for the document field.
func (r *userResolver) Document(ctx context.Context, obj *model.User) (*string, error) {
	panic(fmt.Errorf("not implemented: Document - document"))
}

// Phone is the resolver for the phone field.
func (r *userResolver) Phone(ctx context.Context, obj *model.User) (*string, error) {
	panic(fmt.Errorf("not implemented: Phone - phone"))
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func toAddress(a model.AddressInput) model.Address {
	addr := model.Address{
		Street:       a.Street,
		Number:       a.Number,
		Neighborhood: a.Neighborhood,
		City:         a.City,
		State:        a.State,
		ZipCode:      a.ZipCode,
		Country:      a.Country,
	}
	if a.Complement != nil {
		c := *a.Complement
		addr.Complement = &c
	}
	return addr
}
