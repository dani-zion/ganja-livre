package workflows

const (
	TaskQueueName = "ganja-livre-sales"

	SignalPaymentConfirmed = "payment-confirmed"
	SignalOrderCancelled   = "order-cancelled"
)

type OrderWorkflowInput struct {
	OrderID string
	BuyerID string
	Amount  float64
	Items   []OrderItemInput
}

type OrderItemInput struct {
	ProductID string
	Quantity  int
}
