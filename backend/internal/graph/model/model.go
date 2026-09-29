package model

import "time"

// ─── Enums ────────────────────────────────────

type ProductCategory string

const (
	ProductCategoryFlower     ProductCategory = "FLOWER"
	ProductCategoryOil        ProductCategory = "OIL"
	ProductCategoryEdible     ProductCategory = "EDIBLE"
	ProductCategoryExtraction ProductCategory = "EXTRACTION"
)

func (e ProductCategory) IsValid() bool {
	switch e {
	case ProductCategoryFlower, ProductCategoryOil, ProductCategoryEdible, ProductCategoryExtraction:
		return true
	}
	return false
}

func (e ProductCategory) String() string { return string(e) }

type OrderStatus string

const (
	OrderStatusPending           OrderStatus = "PENDING"
	OrderStatusPaymentProcessing OrderStatus = "PAYMENT_PROCESSING"
	OrderStatusPaymentConfirmed  OrderStatus = "PAYMENT_CONFIRMED"
	OrderStatusPreparing         OrderStatus = "PREPARING"
	OrderStatusShipped           OrderStatus = "SHIPPED"
	OrderStatusDelivered         OrderStatus = "DELIVERED"
	OrderStatusCancelled         OrderStatus = "CANCELLED"
	OrderStatusRefunded          OrderStatus = "REFUNDED"
)

func (e OrderStatus) IsValid() bool {
	switch e {
	case OrderStatusPending, OrderStatusPaymentProcessing, OrderStatusPaymentConfirmed,
		OrderStatusPreparing, OrderStatusShipped, OrderStatusDelivered,
		OrderStatusCancelled, OrderStatusRefunded:
		return true
	}
	return false
}

func (e OrderStatus) String() string { return string(e) }

type UserRole string

const (
	UserRoleCustomer UserRole = "CUSTOMER"
	UserRoleSeller   UserRole = "SELLER"
	UserRoleAdmin    UserRole = "ADMIN"
)

func (e UserRole) IsValid() bool {
	switch e {
	case UserRoleCustomer, UserRoleSeller, UserRoleAdmin:
		return true
	}
	return false
}

func (e UserRole) String() string { return string(e) }

// ─── Types ────────────────────────────────────

type User struct {
	ID        string     `json:"id" bson:"_id,omitempty"`
	Email     string     `json:"email" bson:"email"`
	Name      string     `json:"name" bson:"name"`
	Roles     []UserRole `json:"roles" bson:"roles"`
	Address   *Address   `json:"address,omitempty" bson:"address,omitempty"`
	CreatedAt time.Time  `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt" bson:"updatedAt"`
}

type Address struct {
	Street       string  `json:"street" bson:"street"`
	Number       string  `json:"number" bson:"number"`
	Complement   *string `json:"complement,omitempty" bson:"complement,omitempty"`
	Neighborhood string  `json:"neighborhood" bson:"neighborhood"`
	City         string  `json:"city" bson:"city"`
	State        string  `json:"state" bson:"state"`
	ZipCode      string  `json:"zipCode" bson:"zipCode"`
	Country      string  `json:"country" bson:"country"`
}

type Product struct {
	ID          string          `json:"id" bson:"_id,omitempty"`
	Name        string          `json:"name" bson:"name"`
	Description string          `json:"description" bson:"description"`
	Category    ProductCategory `json:"category" bson:"category"`
	Price       float64         `json:"price" bson:"price"`
	Stock       int             `json:"stock" bson:"stock"`
	ThcContent  *float64        `json:"thcContent,omitempty" bson:"thcContent,omitempty"`
	CbdContent  *float64        `json:"cbdContent,omitempty" bson:"cbdContent,omitempty"`
	Strain      *string         `json:"strain,omitempty" bson:"strain,omitempty"`
	Origin      *string         `json:"origin,omitempty" bson:"origin,omitempty"`
	ImageURLs   []string        `json:"imageURLs" bson:"imageURLs"`
	SellerID    string          `json:"-" bson:"sellerId"`
	Seller      *User           `json:"seller" bson:"-"`
	IsActive    bool            `json:"isActive" bson:"isActive"`
	CreatedAt   time.Time       `json:"createdAt" bson:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt" bson:"updatedAt"`
}

type ProductConnection struct {
	Edges      []*ProductEdge `json:"edges"`
	PageInfo   *PageInfo      `json:"pageInfo"`
	TotalCount int            `json:"totalCount"`
}

type ProductEdge struct {
	Node   *Product `json:"node"`
	Cursor string   `json:"cursor"`
}

type PageInfo struct {
	HasNextPage     bool    `json:"hasNextPage"`
	HasPreviousPage bool    `json:"hasPreviousPage"`
	StartCursor     *string `json:"startCursor,omitempty"`
	EndCursor       *string `json:"endCursor,omitempty"`
}

type Order struct {
	ID                 string       `json:"id" bson:"_id,omitempty"`
	BuyerID            string       `json:"-" bson:"buyerId"`
	Buyer              *User        `json:"buyer" bson:"-"`
	Items              []*OrderItem `json:"items" bson:"items"`
	TotalAmount        float64      `json:"totalAmount" bson:"totalAmount"`
	Status             OrderStatus  `json:"status" bson:"status"`
	ShippingAddress    Address      `json:"shippingAddress" bson:"shippingAddress"`
	TemporalWorkflowID string       `json:"temporalWorkflowID" bson:"temporalWorkflowId"`
	PaymentIntentID    *string      `json:"paymentIntentID,omitempty" bson:"paymentIntentId,omitempty"`
	CreatedAt          time.Time    `json:"createdAt" bson:"createdAt"`
	UpdatedAt          time.Time    `json:"updatedAt" bson:"updatedAt"`
}

type OrderItem struct {
	ProductID string   `json:"-" bson:"productId"`
	Product   *Product `json:"product" bson:"-"`
	Quantity  int      `json:"quantity" bson:"quantity"`
	UnitPrice float64  `json:"unitPrice" bson:"unitPrice"`
	Subtotal  float64  `json:"subtotal" bson:"subtotal"`
}

type AuthPayload struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	User         *User  `json:"user"`
}

// ─── Inputs ───────────────────────────────────

type RegisterInput struct {
	Email    string      `json:"email"`
	Password string      `json:"password"`
	Name     string      `json:"name"`
	Roles    []UserRole  `json:"roles,omitempty"`
}

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AddressInput struct {
	Street       string  `json:"street"`
	Number       string  `json:"number"`
	Complement   *string `json:"complement,omitempty"`
	Neighborhood string  `json:"neighborhood"`
	City         string  `json:"city"`
	State        string  `json:"state"`
	ZipCode      string  `json:"zipCode"`
	Country      string  `json:"country"`
}

type CreateProductInput struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Category    ProductCategory `json:"category"`
	Price       float64         `json:"price"`
	Stock       int             `json:"stock"`
	ThcContent  *float64        `json:"thcContent,omitempty"`
	CbdContent  *float64        `json:"cbdContent,omitempty"`
	Strain      *string         `json:"strain,omitempty"`
	Origin      *string         `json:"origin,omitempty"`
	ImageURLs   []string        `json:"imageURLs"`
}

type UpdateProductInput struct {
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	Price       *float64 `json:"price,omitempty"`
	Stock       *int     `json:"stock,omitempty"`
	ThcContent  *float64 `json:"thcContent,omitempty"`
	CbdContent  *float64 `json:"cbdContent,omitempty"`
	Strain      *string  `json:"strain,omitempty"`
	Origin      *string  `json:"origin,omitempty"`
	ImageURLs   []string `json:"imageURLs,omitempty"`
	IsActive    *bool    `json:"isActive,omitempty"`
}

type OrderItemInput struct {
	ProductID string `json:"productID"`
	Quantity  int    `json:"quantity"`
}

type PlaceOrderInput struct {
	Items           []*OrderItemInput `json:"items"`
	ShippingAddress AddressInput      `json:"shippingAddress"`
}

type ProductFilterInput struct {
	Category *ProductCategory `json:"category,omitempty"`
	MinPrice *float64         `json:"minPrice,omitempty"`
	MaxPrice *float64         `json:"maxPrice,omitempty"`
	MinThc   *float64         `json:"minThc,omitempty"`
	MaxThc   *float64         `json:"maxThc,omitempty"`
	Search   *string          `json:"search,omitempty"`
}
