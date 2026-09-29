package mongodb

import (
	"context"
	"time"

	"github.com/dani-zion/ganja-livre/internal/config"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.uber.org/zap"
)

// Client wraps the official MongoDB driver with helpers.
type Client struct {
	client   *mongo.Client
	database *mongo.Database
	log      *zap.Logger
}

// Collections exposes typed collection accessors.
type Collections struct {
	Users    *mongo.Collection
	Products *mongo.Collection
	Orders   *mongo.Collection
}

// New creates a secured, pool-tuned MongoDB connection.
func New(cfg config.MongoConfig, log *zap.Logger) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
	defer cancel()

	opts := options.Client().
		ApplyURI(cfg.URI).
		SetMaxPoolSize(cfg.MaxPoolSize).
		SetMinPoolSize(2).
		SetMaxConnIdleTime(60 * time.Second).
		SetServerSelectionTimeout(cfg.ConnectTimeout).
		SetConnectTimeout(cfg.ConnectTimeout).
		SetReadPreference(readpref.PrimaryPreferred())

	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, err
	}

	if err = client.Ping(ctx, readpref.Primary()); err != nil {
		return nil, err
	}

	log.Info("connected to MongoDB", zap.String("database", cfg.Database))

	db := client.Database(cfg.Database)
	c := &Client{client: client, database: db, log: log}

	if err = c.createIndexes(ctx); err != nil {
		return nil, err
	}

	return c, nil
}

// Collections returns strongly typed collection handles.
func (c *Client) Collections() *Collections {
	return &Collections{
		Users:    c.database.Collection("users"),
		Products: c.database.Collection("products"),
		Orders:   c.database.Collection("orders"),
	}
}

// Disconnect cleanly closes the connection pool.
func (c *Client) Disconnect(ctx context.Context) error {
	return c.client.Disconnect(ctx)
}

// createIndexes enforces uniqueness and query performance indexes.
func (c *Client) createIndexes(ctx context.Context) error {
	cols := c.Collections()

	// ── Users ────────────────────────────────────────────────────────────────
	_, err := cols.Users.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "email", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("unique_email"),
		},
		{
			Keys:    bson.D{{Key: "role", Value: 1}},
			Options: options.Index().SetName("idx_role"),
		},
	})
	if err != nil {
		return err
	}

	// ── Products ─────────────────────────────────────────────────────────────
	_, err = cols.Products.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "seller_id", Value: 1}},
			Options: options.Index().SetName("idx_seller"),
		},
		{
			Keys:    bson.D{{Key: "category", Value: 1}, {Key: "is_active", Value: 1}},
			Options: options.Index().SetName("idx_category_active"),
		},
		{
			Keys:    bson.D{{Key: "price", Value: 1}},
			Options: options.Index().SetName("idx_price"),
		},
		// Text index for full-text search on name and description
		{
			Keys:    bson.D{{Key: "name", Value: "text"}, {Key: "description", Value: "text"}},
			Options: options.Index().SetName("text_search"),
		},
	})
	if err != nil {
		return err
	}

	// ── Orders ───────────────────────────────────────────────────────────────
	_, err = cols.Orders.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "buyer_id", Value: 1}},
			Options: options.Index().SetName("idx_buyer"),
		},
		{
			Keys:    bson.D{{Key: "status", Value: 1}},
			Options: options.Index().SetName("idx_status"),
		},
		{
			Keys:    bson.D{{Key: "temporal_workflow_id", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("unique_workflow_id"),
		},
		{
			Keys:    bson.D{{Key: "created_at", Value: -1}},
			Options: options.Index().SetName("idx_created_desc"),
		},
	})

	c.log.Info("MongoDB indexes ensured")
	return err
}
