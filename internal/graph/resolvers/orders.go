//go:build ignore
// +build ignore

package resolvers

import (
	"context"
	"fmt"
	"time"

	"github.com/dani-zion/ganja-livre/internal/graph/model"
	"github.com/dani-zion/ganja-livre/internal/middleware"
	"github.com/dani-zion/ganja-livre/internal/temporal/workflows"
	"github.com/dani-zion/ganja-livre/internal/validator"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.temporal.io/sdk/client"
)

// PlaceOrder creates an order and kicks off the Temporal workflow.
func (r *Resolver) PlaceOrder(ctx context.Context, input PlaceOrderInput) (*model.Order, error) {
	claims, err := middleware.RequireRole(ctx, model.RoleCustomer, model.RoleAdmin)
	if err != nil {
		return nil, err
	}

	if len(input.Items) == 0 {
		return nil, fmt.Errorf("order must contain at least one item")
	}

	buyerID, _ := primitive.ObjectIDFromHex(claims.UserID)

	// Build order items, resolving prices from DB to prevent client-side price manipulation
	var orderItems []model.OrderItem
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

		var product model.Product
		if err = r.cols.Products.FindOne(ctx, bson.M{"_id": pid, "is_active": true}).Decode(&product); err != nil {
			return nil, fmt.Errorf("product not found: %s", item.ProductID)
		}

		subtotal := product.Price * float64(item.Quantity)
		orderItems = append(orderItems, model.OrderItem{
			ProductID: pid,
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

	// Unique idempotency key for the Temporal workflow
	workflowID := fmt.Sprintf("order-%s", uuid.New().String())
	now := time.Now().UTC()

	order := &model.Order{
		ID:                 primitive.NewObjectID(),
		BuyerID:            buyerID,
		Items:              orderItems,
		TotalAmount:        totalAmount,
		Status:             model.StatusPending,
		ShippingAddress:    toAddress(input.ShippingAddress),
		TemporalWorkflowID: workflowID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if _, err = r.cols.Orders.InsertOne(ctx, order); err != nil {
		return nil, errInternal
	}

	// Start the Temporal workflow asynchronously
	_, err = r.temporal.ExecuteWorkflow(ctx,
		client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: workflows.TaskQueueName,
		},
		workflows.OrderWorkflow,
		workflows.OrderWorkflowInput{
			OrderID: order.ID.Hex(),
			BuyerID: claims.UserID,
			Amount:  totalAmount,
			Items:   workflowItems,
		},
	)
	if err != nil {
		// Roll back order insert on workflow start failure
		_, _ = r.cols.Orders.DeleteOne(ctx, bson.M{"_id": order.ID})
		return nil, fmt.Errorf("failed to start order workflow: %w", err)
	}

	return order, nil
}

// CancelOrder sends a cancellation signal to the running workflow.
func (r *Resolver) CancelOrder(ctx context.Context, id string) (*model.Order, error) {
	claims, err := middleware.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, errNotFound
	}

	filter := bson.M{"_id": oid}
	if claims.Role == model.RoleCustomer {
		buyerID, _ := primitive.ObjectIDFromHex(claims.UserID)
		filter["buyer_id"] = buyerID
	}

	var order model.Order
	if err = r.cols.Orders.FindOne(ctx, filter).Decode(&order); err != nil {
		return nil, errNotFound
	}

	// Only allow cancellation of pending/processing orders
	if order.Status != model.StatusPending && order.Status != model.StatusPaymentProcessing {
		return nil, fmt.Errorf("order cannot be cancelled in status: %s", order.Status)
	}

	// Signal the Temporal workflow
	if err = r.temporal.SignalWorkflow(ctx,
		order.TemporalWorkflowID, "",
		workflows.SignalOrderCancelled, nil,
	); err != nil {
		return nil, fmt.Errorf("failed to send cancellation signal: %w", err)
	}

	return &order, nil
}

// MyOrders returns orders belonging to the authenticated user.
func (r *Resolver) MyOrders(ctx context.Context) ([]*model.Order, error) {
	claims, err := middleware.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	buyerID, _ := primitive.ObjectIDFromHex(claims.UserID)
	cursor, err := r.cols.Orders.Find(ctx,
		bson.M{"buyer_id": buyerID},
		// Newest first
	)
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

// UpdateOrderStatus is an admin operation that also signals Temporal if needed.
func (r *Resolver) UpdateOrderStatus(ctx context.Context, id string, status model.OrderStatus) (*model.Order, error) {
	if _, err := middleware.RequireRole(ctx, model.RoleAdmin); err != nil {
		return nil, err
	}

	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, errNotFound
	}

	now := time.Now().UTC()
	after := true
	var updated model.Order
	opts := bson.M{"$set": bson.M{"status": status, "updated_at": now}}
	_ = after
	if err = r.cols.Orders.FindOneAndUpdate(ctx, bson.M{"_id": oid}, opts).Decode(&updated); err != nil {
		return nil, errNotFound
	}
	return &updated, nil
}

// ─── Input types ─────────────────────────────────────────────────────────────

type PlaceOrderInput struct {
	Items           []OrderItemInput
	ShippingAddress AddressInput
}

type OrderItemInput struct {
	ProductID string
	Quantity  int
}

type AddressInput struct {
	Street       string
	Number       string
	Complement   *string
	Neighborhood string
	City         string
	State        string
	ZipCode      string
	Country      string
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func toAddress(a AddressInput) model.Address {
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
		addr.Complement = *a.Complement
	}
	return addr
}
