package resolvers

import (
	"context"
	"time"

	"github.com/dani-zion/ganja-livre/internal/graph/model"
	"github.com/dani-zion/ganja-livre/internal/middleware"
	dbmodel "github.com/dani-zion/ganja-livre/internal/model"
	"github.com/dani-zion/ganja-livre/internal/validator"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const defaultPageSize = 20

// CreateProduct allows sellers and admins to add products.
func (r *mutationResolver) CreateProduct(ctx context.Context, input model.CreateProductInput) (*model.Product, error) {
	claims, err := middleware.RequireRole(ctx, model.UserRoleSeller, model.UserRoleAdmin)
	if err != nil {
		return nil, err
	}

	if err = validator.Price(input.Price); err != nil {
		return nil, err
	}
	if err = validator.Stock(input.Stock); err != nil {
		return nil, err
	}
	if input.ThcContent != nil {
		if err = validator.PercentContent(*input.ThcContent, "thcContent"); err != nil {
			return nil, err
		}
	}

	sellerID, _ := primitive.ObjectIDFromHex(claims.UserID)
	now := time.Now().UTC()
	dbProduct := dbmodel.Product{
		ID:          primitive.NewObjectID(),
		Name:        input.Name,
		Description: input.Description,
		Category:    dbmodel.ProductCategory(input.Category),
		Price:       input.Price,
		Stock:       input.Stock,
		THCContent:  input.ThcContent,
		CBDContent:  input.CbdContent,
		Strain:      input.Strain,
		Origin:      input.Origin,
		ImageURLs:   input.ImageURLs,
		SellerID:    sellerID,
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if _, err = r.cols.Products.InsertOne(ctx, dbProduct); err != nil {
		return nil, errInternal
	}

	return &model.Product{
		ID:          dbProduct.ID.Hex(),
		Name:        dbProduct.Name,
		Description: dbProduct.Description,
		Category:    model.ProductCategory(dbProduct.Category),
		Price:       dbProduct.Price,
		Stock:       dbProduct.Stock,
		ThcContent:  dbProduct.THCContent,
		CbdContent:  dbProduct.CBDContent,
		Strain:      dbProduct.Strain,
		Origin:      dbProduct.Origin,
		ImageURLs:   dbProduct.ImageURLs,
		SellerID:    dbProduct.SellerID.Hex(),
		IsActive:    dbProduct.IsActive,
		CreatedAt:   dbProduct.CreatedAt,
		UpdatedAt:   dbProduct.UpdatedAt,
	}, nil
}

// UpdateProduct lets sellers edit their own products; admins can edit any.
func (r *mutationResolver) UpdateProduct(ctx context.Context, id string, input model.UpdateProductInput) (*model.Product, error) {
	claims, err := middleware.RequireRole(ctx, model.UserRoleSeller, model.UserRoleAdmin)
	if err != nil {
		return nil, err
	}

	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, errNotFound
	}

	filter := bson.M{"_id": oid}
	if !hasRole(claims.Roles, model.UserRoleAdmin) {
		sellerID, _ := primitive.ObjectIDFromHex(claims.UserID)
		filter["seller_id"] = sellerID
	}

	set := bson.M{"updated_at": time.Now().UTC()}
	if input.Name != nil {
		set["name"] = *input.Name
	}
	if input.Description != nil {
		set["description"] = *input.Description
	}
	if input.Price != nil {
		if err = validator.Price(*input.Price); err != nil {
			return nil, err
		}
		set["price"] = *input.Price
	}
	if input.Stock != nil {
		if err = validator.Stock(*input.Stock); err != nil {
			return nil, err
		}
		set["stock"] = *input.Stock
	}
	if input.IsActive != nil {
		set["is_active"] = *input.IsActive
	}
	if input.ImageURLs != nil {
		set["image_urls"] = input.ImageURLs
	}

	after := options.After
	opts := options.FindOneAndUpdate().SetReturnDocument(after)
	var updated dbmodel.Product
	if err = r.cols.Products.FindOneAndUpdate(ctx, filter, bson.M{"$set": set}, opts).Decode(&updated); err != nil {
		return nil, errNotFound
	}

	return &model.Product{
		ID:          updated.ID.Hex(),
		Name:        updated.Name,
		Description: updated.Description,
		Category:    model.ProductCategory(updated.Category),
		Price:       updated.Price,
		Stock:       updated.Stock,
		ThcContent:  updated.THCContent,
		CbdContent:  updated.CBDContent,
		Strain:      updated.Strain,
		Origin:      updated.Origin,
		ImageURLs:   updated.ImageURLs,
		SellerID:    updated.SellerID.Hex(),
		IsActive:    updated.IsActive,
		CreatedAt:   updated.CreatedAt,
		UpdatedAt:   updated.UpdatedAt,
	}, nil
}

// DeleteProduct soft-deletes (deactivates) a product.
func (r *mutationResolver) DeleteProduct(ctx context.Context, id string) (bool, error) {
	claims, err := middleware.RequireRole(ctx, model.UserRoleSeller, model.UserRoleAdmin)
	if err != nil {
		return false, err
	}

	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return false, errNotFound
	}

	filter := bson.M{"_id": oid}
	if !hasRole(claims.Roles, model.UserRoleAdmin) {
		sellerID, _ := primitive.ObjectIDFromHex(claims.UserID)
		filter["seller_id"] = sellerID
	}

	res, err := r.cols.Products.UpdateOne(ctx, filter,
		bson.M{"$set": bson.M{"is_active": false, "updated_at": time.Now().UTC()}},
	)
	if err != nil {
		return false, errInternal
	}
	return res.MatchedCount > 0, nil
}

// Products lists active products with optional filtering and cursor-based pagination.
func (r *queryResolver) Products(ctx context.Context, filter *model.ProductFilterInput, first *int, after *string) (*model.ProductConnection, error) {
	query := bson.M{"is_active": true}

	if filter != nil {
		if filter.Category != nil {
			query["category"] = *filter.Category
		}
		priceFilter := bson.M{}
		if filter.MinPrice != nil {
			priceFilter["$gte"] = *filter.MinPrice
		}
		if filter.MaxPrice != nil {
			priceFilter["$lte"] = *filter.MaxPrice
		}
		if len(priceFilter) > 0 {
			query["price"] = priceFilter
		}
		if filter.Search != nil && *filter.Search != "" {
			query["$text"] = bson.M{"$search": *filter.Search}
		}
	}

	if after != nil && *after != "" {
		cursorID, err := primitive.ObjectIDFromHex(*after)
		if err == nil {
			query["_id"] = bson.M{"$gt": cursorID}
		}
	}

	limit := int64(defaultPageSize)
	if first != nil && *first > 0 && *first <= 100 {
		limit = int64(*first)
	}

	findOpts := options.Find().
		SetLimit(limit + 1).
		SetSort(bson.D{{Key: "_id", Value: 1}})

	cursor, err := r.cols.Products.Find(ctx, query, findOpts)
	if err != nil {
		return nil, errInternal
	}
	defer cursor.Close(ctx)

	var dbProducts []dbmodel.Product
	if err = cursor.All(ctx, &dbProducts); err != nil {
		return nil, errInternal
	}

	hasNext := len(dbProducts) > int(limit)
	if hasNext {
		dbProducts = dbProducts[:limit]
	}

	edges := make([]*model.ProductEdge, len(dbProducts))
	for i, p := range dbProducts {
		product := model.Product{
			ID:          p.ID.Hex(),
			Name:        p.Name,
			Description: p.Description,
			Category:    model.ProductCategory(p.Category),
			Price:       p.Price,
			Stock:       p.Stock,
			ThcContent:  p.THCContent,
			CbdContent:  p.CBDContent,
			Strain:      p.Strain,
			Origin:      p.Origin,
			ImageURLs:   p.ImageURLs,
			SellerID:    p.SellerID.Hex(),
			IsActive:    p.IsActive,
			CreatedAt:   p.CreatedAt,
			UpdatedAt:   p.UpdatedAt,
		}
		edges[i] = &model.ProductEdge{Node: &product, Cursor: p.ID.Hex()}
	}

	var startCursor, endCursor *string
	if len(edges) > 0 {
		s := edges[0].Cursor
		e := edges[len(edges)-1].Cursor
		startCursor = &s
		endCursor = &e
	}

	total, _ := r.cols.Products.CountDocuments(ctx, bson.M{"is_active": true})

	return &model.ProductConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage:     hasNext,
			HasPreviousPage: after != nil && *after != "",
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: int(total),
	}, nil
}

// Product returns a single product by ID.
func (r *queryResolver) Product(ctx context.Context, id string) (*model.Product, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, errNotFound
	}

	var p dbmodel.Product
	if err = r.cols.Products.FindOne(ctx, bson.M{"_id": oid, "is_active": true}).Decode(&p); err != nil {
		return nil, errNotFound
	}

	return &model.Product{
		ID:          p.ID.Hex(),
		Name:        p.Name,
		Description: p.Description,
		Category:    model.ProductCategory(p.Category),
		Price:       p.Price,
		Stock:       p.Stock,
		ThcContent:  p.THCContent,
		CbdContent:  p.CBDContent,
		Strain:      p.Strain,
		Origin:      p.Origin,
		ImageURLs:   p.ImageURLs,
		SellerID:    p.SellerID.Hex(),
		IsActive:    p.IsActive,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}, nil
}

// SellerProducts returns all products for the authenticated seller.
func (r *queryResolver) SellerProducts(ctx context.Context) ([]*model.Product, error) {
	claims, err := middleware.RequireRole(ctx, model.UserRoleSeller, model.UserRoleAdmin)
	if err != nil {
		return nil, err
	}

	sellerID, _ := primitive.ObjectIDFromHex(claims.UserID)
	cursor, err := r.cols.Products.Find(ctx, bson.M{"seller_id": sellerID})
	if err != nil {
		return nil, errInternal
	}
	defer cursor.Close(ctx)

	var dbProducts []dbmodel.Product
	if err = cursor.All(ctx, &dbProducts); err != nil {
		return nil, errInternal
	}

	products := make([]*model.Product, len(dbProducts))
	for i, p := range dbProducts {
		products[i] = &model.Product{
			ID:          p.ID.Hex(),
			Name:        p.Name,
			Description: p.Description,
			Category:    model.ProductCategory(p.Category),
			Price:       p.Price,
			Stock:       p.Stock,
			ThcContent:  p.THCContent,
			CbdContent:  p.CBDContent,
			Strain:      p.Strain,
			Origin:      p.Origin,
			ImageURLs:   p.ImageURLs,
			SellerID:    p.SellerID.Hex(),
			IsActive:    p.IsActive,
			CreatedAt:   p.CreatedAt,
			UpdatedAt:   p.UpdatedAt,
		}
	}
	return products, nil
}
