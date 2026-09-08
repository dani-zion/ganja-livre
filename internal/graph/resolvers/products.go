//go:build ignore
// +build ignore

package resolvers

import (
	"context"
	"time"

	"github.com/dani-zion/ganja-livre/internal/graph/model"
	"github.com/dani-zion/ganja-livre/internal/middleware"
	"github.com/dani-zion/ganja-livre/internal/validator"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const defaultPageSize = 20

// CreateProduct allows sellers and admins to add products.
func (r *Resolver) CreateProduct(ctx context.Context, input CreateProductInput) (*model.Product, error) {
	claims, err := middleware.RequireRole(ctx, model.RoleSeller, model.RoleAdmin)
	if err != nil {
		return nil, err
	}

	if err = validator.Price(input.Price); err != nil {
		return nil, err
	}
	if err = validator.Stock(input.Stock); err != nil {
		return nil, err
	}
	if input.THCContent != nil {
		if err = validator.PercentContent(*input.THCContent, "thcContent"); err != nil {
			return nil, err
		}
	}

	sellerID, _ := primitive.ObjectIDFromHex(claims.UserID)
	now := time.Now().UTC()
	product := &model.Product{
		ID:          primitive.NewObjectID(),
		Name:        input.Name,
		Description: input.Description,
		Category:    input.Category,
		Price:       input.Price,
		Stock:       input.Stock,
		THCContent:  input.THCContent,
		CBDContent:  input.CBDContent,
		Strain:      input.Strain,
		Origin:      input.Origin,
		ImageURLs:   input.ImageURLs,
		SellerID:    sellerID,
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if _, err = r.cols.Products.InsertOne(ctx, product); err != nil {
		return nil, errInternal
	}
	return product, nil
}

// UpdateProduct lets sellers edit their own products; admins can edit any.
func (r *Resolver) UpdateProduct(ctx context.Context, id string, input UpdateProductInput) (*model.Product, error) {
	claims, err := middleware.RequireRole(ctx, model.RoleSeller, model.RoleAdmin)
	if err != nil {
		return nil, err
	}

	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, errNotFound
	}

	filter := bson.M{"_id": oid}
	if claims.Role == model.RoleSeller {
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
	var updated model.Product
	if err = r.cols.Products.FindOneAndUpdate(ctx, filter, bson.M{"$set": set}, opts).Decode(&updated); err != nil {
		return nil, errNotFound
	}
	return &updated, nil
}

// DeleteProduct soft-deletes (deactivates) a product.
func (r *Resolver) DeleteProduct(ctx context.Context, id string) (bool, error) {
	claims, err := middleware.RequireRole(ctx, model.RoleSeller, model.RoleAdmin)
	if err != nil {
		return false, err
	}

	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return false, errNotFound
	}

	filter := bson.M{"_id": oid}
	if claims.Role == model.RoleSeller {
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
func (r *Resolver) Products(ctx context.Context, filter *ProductFilterInput, first *int, after *string) (*ProductConnection, error) {
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

	// Cursor: use ObjectID as cursor for stable pagination
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
		SetLimit(limit + 1). // fetch one extra to determine hasNextPage
		SetSort(bson.D{{Key: "_id", Value: 1}})

	cursor, err := r.cols.Products.Find(ctx, query, findOpts)
	if err != nil {
		return nil, errInternal
	}
	defer cursor.Close(ctx)

	var products []model.Product
	if err = cursor.All(ctx, &products); err != nil {
		return nil, errInternal
	}

	hasNext := len(products) > int(limit)
	if hasNext {
		products = products[:limit]
	}

	edges := make([]*ProductEdge, len(products))
	for i, p := range products {
		p := p
		edges[i] = &ProductEdge{Node: &p, Cursor: p.ID.Hex()}
	}

	var startCursor, endCursor *string
	if len(edges) > 0 {
		s := edges[0].Cursor
		e := edges[len(edges)-1].Cursor
		startCursor = &s
		endCursor = &e
	}

	total, _ := r.cols.Products.CountDocuments(ctx, bson.M{"is_active": true})

	return &ProductConnection{
		Edges: edges,
		PageInfo: &PageInfo{
			HasNextPage:     hasNext,
			HasPreviousPage: after != nil && *after != "",
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: int(total),
	}, nil
}

// ─── GQL return types for pagination ─────────────────────────────────────────

type ProductConnection struct {
	Edges      []*ProductEdge
	PageInfo   *PageInfo
	TotalCount int
}

type ProductEdge struct {
	Node   *model.Product
	Cursor string
}

type PageInfo struct {
	HasNextPage     bool
	HasPreviousPage bool
	StartCursor     *string
	EndCursor       *string
}

// ─── Input types ─────────────────────────────────────────────────────────────

type CreateProductInput struct {
	Name        string
	Description string
	Category    model.ProductCategory
	Price       float64
	Stock       int
	THCContent  *float64
	CBDContent  *float64
	Strain      *string
	Origin      *string
	ImageURLs   []string
}

type UpdateProductInput struct {
	Name        *string
	Description *string
	Price       *float64
	Stock       *int
	THCContent  *float64
	CBDContent  *float64
	Strain      *string
	Origin      *string
	ImageURLs   []string
	IsActive    *bool
}

type ProductFilterInput struct {
	Category *model.ProductCategory
	MinPrice *float64
	MaxPrice *float64
	Search   *string
}
