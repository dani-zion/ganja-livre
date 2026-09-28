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

	var orderItems []dbmodel.OrderItem
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
		orderItems = append(orderItems, dbmodel.OrderItem{
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

	workflowID := fmt.Sprintf("order-%s", uuid.New().String())
	now := time.Now().UTC()

	order := dbmodel.Order{
		ID:                 primitive.NewObjectID(),
		BuyerID:            buyerID,
		Items:              orderItems,
		TotalAmount:        totalAmount,
		Status:             dbmodel.StatusPending,
		ShippingAddress:    toDBAddress(input.ShippingAddress),
		TemporalWorkflowID: workflowID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if _, err = r.cols.Orders.InsertOne(ctx, order); err != nil {
		return nil, errInternal
	}

	_, err = r.temporal.ExecuteWorkflow(ctx,
		client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: workflows.TaskQueueName,
		},
		"OrderWorkflow",
		workflows.OrderWorkflowInput{
			OrderID: order.ID.Hex(),
			BuyerID: claims.UserID,
			Amount:  totalAmount,
			Items:   workflowItems,
		},
	)
	if err != nil {
		_, _ = r.cols.Orders.DeleteOne(ctx, bson.M{"_id": order.ID})
		return nil, fmt.Errorf("failed to start order workflow: %w", err)
	}

	orders, err := r.toGraphOrders(ctx, []dbmodel.Order{order})
	if err != nil {
		return nil, err
	}
	return orders[0], nil
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

	var order dbmodel.Order
	if err = r.cols.Orders.FindOne(ctx, filter).Decode(&order); err != nil {
		return nil, errNotFound
	}

	if order.Status != dbmodel.StatusPending && order.Status != dbmodel.StatusPaymentProcessing {
		return nil, fmt.Errorf("order cannot be cancelled in status: %s", order.Status)
	}

	if err = r.temporal.SignalWorkflow(ctx,
		order.TemporalWorkflowID, "",
		workflows.SignalOrderCancelled, nil,
	); err != nil {
		return nil, fmt.Errorf("failed to send cancellation signal: %w", err)
	}

	return r.singleOrder(ctx, order)
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
	var updated dbmodel.Order
	if err = r.cols.Orders.FindOneAndUpdate(ctx,
		bson.M{"_id": oid},
		bson.M{"$set": bson.M{"status": string(status), "updated_at": now}},
		opts,
	).Decode(&updated); err != nil {
		return nil, errNotFound
	}
	return r.singleOrder(ctx, updated)
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

	return toGraphUser(user), nil
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

	var dbOrders []dbmodel.Order
	if err = cursor.All(ctx, &dbOrders); err != nil {
		return nil, errInternal
	}
	return r.toGraphOrders(ctx, dbOrders)
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

	var dbOrder dbmodel.Order
	if err = r.cols.Orders.FindOne(ctx, filter).Decode(&dbOrder); err != nil {
		return nil, errNotFound
	}
	return r.singleOrder(ctx, dbOrder)
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

	var dbOrders []dbmodel.Order
	if err = cursor.All(ctx, &dbOrders); err != nil {
		return nil, errInternal
	}
	return r.toGraphOrders(ctx, dbOrders)
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

// singleOrder maps one persisted order with its relations resolved.
func (r *Resolver) singleOrder(ctx context.Context, dbOrder dbmodel.Order) (*model.Order, error) {
	orders, err := r.toGraphOrders(ctx, []dbmodel.Order{dbOrder})
	if err != nil {
		return nil, err
	}
	return orders[0], nil
}

// toGraphOrders maps persisted orders to their GraphQL shape and hydrates the
// buyer and product relations with one batched query per relation.
func (r *Resolver) toGraphOrders(ctx context.Context, dbOrders []dbmodel.Order) ([]*model.Order, error) {
	orders := make([]*model.Order, len(dbOrders))
	productIDs := make(map[string]primitive.ObjectID)
	buyerIDs := make(map[string]primitive.ObjectID)

	for i, dbOrder := range dbOrders {
		orders[i] = toGraphOrder(dbOrder)
		if !dbOrder.BuyerID.IsZero() {
			buyerIDs[dbOrder.BuyerID.Hex()] = dbOrder.BuyerID
		}
		for _, item := range dbOrder.Items {
			if !item.ProductID.IsZero() {
				productIDs[item.ProductID.Hex()] = item.ProductID
			}
		}
	}

	products, err := r.loadProducts(ctx, productIDs)
	if err != nil {
		return nil, err
	}
	buyers, err := r.loadUsers(ctx, buyerIDs)
	if err != nil {
		return nil, err
	}

	for _, order := range orders {
		order.Buyer = buyers[order.BuyerID]
		for _, item := range order.Items {
			item.Product = products[item.ProductID]
		}
	}
	return orders, nil
}

// loadProducts fetches products by ID, keyed by hex ID.
func (r *Resolver) loadProducts(ctx context.Context, ids map[string]primitive.ObjectID) (map[string]*model.Product, error) {
	products := make(map[string]*model.Product, len(ids))
	if len(ids) == 0 {
		return products, nil
	}

	cursor, err := r.cols.Products.Find(ctx, bson.M{"_id": bson.M{"$in": objectIDs(ids)}})
	if err != nil {
		return nil, errInternal
	}
	defer cursor.Close(ctx)

	var dbProducts []dbmodel.Product
	if err = cursor.All(ctx, &dbProducts); err != nil {
		return nil, errInternal
	}
	for i := range dbProducts {
		product := toGraphProduct(dbProducts[i])
		products[product.ID] = product
	}
	return products, nil
}

// loadUsers fetches users by ID, keyed by hex ID.
func (r *Resolver) loadUsers(ctx context.Context, ids map[string]primitive.ObjectID) (map[string]*model.User, error) {
	users := make(map[string]*model.User, len(ids))
	if len(ids) == 0 {
		return users, nil
	}

	cursor, err := r.cols.Users.Find(ctx, bson.M{"_id": bson.M{"$in": objectIDs(ids)}})
	if err != nil {
		return nil, errInternal
	}
	defer cursor.Close(ctx)

	var dbUsers []dbmodel.User
	if err = cursor.All(ctx, &dbUsers); err != nil {
		return nil, errInternal
	}
	for i := range dbUsers {
		user := toGraphUser(dbUsers[i])
		users[user.ID] = user
	}
	return users, nil
}

func objectIDs(ids map[string]primitive.ObjectID) []primitive.ObjectID {
	oids := make([]primitive.ObjectID, 0, len(ids))
	for _, id := range ids {
		oids = append(oids, id)
	}
	return oids
}

func toDBAddress(a model.AddressInput) dbmodel.Address {
	addr := dbmodel.Address{
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

func toGraphAddress(a dbmodel.Address) model.Address {
	addr := model.Address{
		Street:       a.Street,
		Number:       a.Number,
		Neighborhood: a.Neighborhood,
		City:         a.City,
		State:        a.State,
		ZipCode:      a.ZipCode,
		Country:      a.Country,
	}
	if a.Complement != "" {
		complement := a.Complement
		addr.Complement = &complement
	}
	return addr
}

// toGraphOrder maps a persisted order without its relations; use
// toGraphOrders to also resolve buyer and product.
func toGraphOrder(dbOrder dbmodel.Order) *model.Order {
	items := make([]*model.OrderItem, len(dbOrder.Items))
	for i, item := range dbOrder.Items {
		items[i] = &model.OrderItem{
			ProductID: item.ProductID.Hex(),
			Quantity:  item.Quantity,
			UnitPrice: item.UnitPrice,
			Subtotal:  item.Subtotal,
		}
	}

	return &model.Order{
		ID:                 dbOrder.ID.Hex(),
		BuyerID:            dbOrder.BuyerID.Hex(),
		Items:              items,
		TotalAmount:        dbOrder.TotalAmount,
		Status:             model.OrderStatus(dbOrder.Status),
		ShippingAddress:    toGraphAddress(dbOrder.ShippingAddress),
		TemporalWorkflowID: dbOrder.TemporalWorkflowID,
		PaymentIntentID:    dbOrder.PaymentIntentID,
		CreatedAt:          dbOrder.CreatedAt,
		UpdatedAt:          dbOrder.UpdatedAt,
	}
}
