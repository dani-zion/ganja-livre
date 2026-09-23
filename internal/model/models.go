package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ─── Enums ──────────────────────────────────────────────────────────────────

type ProductCategory string

const (
	CategoryFlower     ProductCategory = "FLOWER"
	CategoryOil        ProductCategory = "OIL"
	CategoryEdible     ProductCategory = "EDIBLE"
	CategoryExtraction ProductCategory = "EXTRACTION"
)

type OrderStatus string

const (
	StatusPending           OrderStatus = "PENDING"
	StatusPaymentProcessing OrderStatus = "PAYMENT_PROCESSING"
	StatusPaymentConfirmed  OrderStatus = "PAYMENT_CONFIRMED"
	StatusPreparing         OrderStatus = "PREPARING"
	StatusShipped           OrderStatus = "SHIPPED"
	StatusDelivered         OrderStatus = "DELIVERED"
	StatusCancelled         OrderStatus = "CANCELLED"
	StatusRefunded          OrderStatus = "REFUNDED"
)

type UserRole string

const (
	RoleCustomer UserRole = "CUSTOMER"
	RoleSeller   UserRole = "SELLER"
	RoleAdmin    UserRole = "ADMIN"
)

// ─── User ────────────────────────────────────────────────────────────────────

type User struct {
	ID           primitive.ObjectID `bson:"_id,omitempty"      json:"id"`
	Email        string             `bson:"email"              json:"email"`
	PasswordHash string             `bson:"password_hash"      json:"-"`
	Name         string             `bson:"name"               json:"name"`
	Roles        []UserRole         `bson:"roles"              json:"roles"`
	Address      *Address           `bson:"address,omitempty"  json:"address,omitempty"`
	IsActive     bool               `bson:"is_active"          json:"is_active"`
	CreatedAt    time.Time          `bson:"created_at"         json:"created_at"`
	UpdatedAt    time.Time          `bson:"updated_at"         json:"updated_at"`
}

type Address struct {
	Street       string `bson:"street"        json:"street"`
	Number       string `bson:"number"        json:"number"`
	Complement   string `bson:"complement"    json:"complement,omitempty"`
	Neighborhood string `bson:"neighborhood"  json:"neighborhood"`
	City         string `bson:"city"          json:"city"`
	State        string `bson:"state"         json:"state"`
	ZipCode      string `bson:"zip_code"      json:"zip_code"`
	Country      string `bson:"country"       json:"country"`
}

// ─── Product ─────────────────────────────────────────────────────────────────

type Product struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"        json:"id"`
	Name        string             `bson:"name"                 json:"name"`
	Description string             `bson:"description"          json:"description"`
	Category    ProductCategory    `bson:"category"             json:"category"`
	Price       float64            `bson:"price"                json:"price"`
	Stock       int                `bson:"stock"                json:"stock"`
	THCContent  *float64           `bson:"thc_content"          json:"thc_content,omitempty"`
	CBDContent  *float64           `bson:"cbd_content"          json:"cbd_content,omitempty"`
	Strain      *string            `bson:"strain"               json:"strain,omitempty"`
	Origin      *string            `bson:"origin"               json:"origin,omitempty"`
	ImageURLs   []string           `bson:"image_urls"           json:"image_urls"`
	SellerID    primitive.ObjectID `bson:"seller_id"            json:"seller_id"`
	IsActive    bool               `bson:"is_active"            json:"is_active"`
	CreatedAt   time.Time          `bson:"created_at"           json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at"           json:"updated_at"`
}

// ─── Order ───────────────────────────────────────────────────────────────────

type OrderItem struct {
	ProductID primitive.ObjectID `bson:"product_id"  json:"product_id"`
	Quantity  int                `bson:"quantity"    json:"quantity"`
	UnitPrice float64            `bson:"unit_price"  json:"unit_price"`
	Subtotal  float64            `bson:"subtotal"    json:"subtotal"`
}

type Order struct {
	ID                 primitive.ObjectID `bson:"_id,omitempty"            json:"id"`
	BuyerID            primitive.ObjectID `bson:"buyer_id"                 json:"buyer_id"`
	Items              []OrderItem        `bson:"items"                    json:"items"`
	TotalAmount        float64            `bson:"total_amount"             json:"total_amount"`
	Status             OrderStatus        `bson:"status"                   json:"status"`
	ShippingAddress    Address            `bson:"shipping_address"         json:"shipping_address"`
	TemporalWorkflowID string             `bson:"temporal_workflow_id"     json:"temporal_workflow_id"`
	PaymentIntentID    *string            `bson:"payment_intent_id"        json:"payment_intent_id,omitempty"`
	CreatedAt          time.Time          `bson:"created_at"               json:"created_at"`
	UpdatedAt          time.Time          `bson:"updated_at"               json:"updated_at"`
}

// ─── JWT Claims ──────────────────────────────────────────────────────────────

type Claims struct {
	UserID string       `json:"user_id"`
	Email  string       `json:"email"`
	Roles  []UserRole   `json:"roles"`
}
