//go:build ignore
// +build ignore

package workflows

import (
	"time"

	"github.com/dani-zion/ganja-livre/internal/graph/model"
	"github.com/dani-zion/ganja-livre/internal/temporal/activities"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	TaskQueueName = "ganja-livre-sales"

	// Signal names for external triggers
	SignalPaymentConfirmed = "payment-confirmed"
	SignalOrderCancelled   = "order-cancelled"
)

// OrderWorkflowInput is the input payload for the order workflow.
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

// OrderWorkflow orchestrates the full lifecycle of a sale.
// Stages: reserve stock → await payment → confirm → prepare → ship → deliver.
func OrderWorkflow(ctx workflow.Context, input OrderWorkflowInput) error {
	log := workflow.GetLogger(ctx)
	log.Info("OrderWorkflow started", "orderID", input.OrderID)

	// Activity options with exponential back-off — idempotent retries are safe.
	actOpts := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    5,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, actOpts)

	// ── 1. Reserve stock ─────────────────────────────────────────────────────
	if err := workflow.ExecuteActivity(ctx, activities.ReserveStock, input.OrderID, input.Items).Get(ctx, nil); err != nil {
		log.Error("failed to reserve stock", "error", err)
		_ = workflow.ExecuteActivity(ctx, activities.UpdateOrderStatus, input.OrderID, model.StatusCancelled).Get(ctx, nil)
		return err
	}

	if err := workflow.ExecuteActivity(ctx, activities.UpdateOrderStatus, input.OrderID, model.StatusPaymentProcessing).Get(ctx, nil); err != nil {
		return err
	}

	// ── 2. Await payment signal (max 30 min) ─────────────────────────────────
	paymentCh := workflow.GetSignalChannel(ctx, SignalPaymentConfirmed)
	cancelCh := workflow.GetSignalChannel(ctx, SignalOrderCancelled)

	var paymentIntentID string
	var cancelled bool

	paymentTimer := workflow.NewTimer(ctx, 30*time.Minute)

	workflow.NewSelector(ctx).
		AddReceive(paymentCh, func(c workflow.ReceiveChannel, more bool) {
			c.Receive(ctx, &paymentIntentID)
		}).
		AddReceive(cancelCh, func(c workflow.ReceiveChannel, more bool) {
			cancelled = true
		}).
		AddFuture(paymentTimer, func(f workflow.Future) {
			cancelled = true // payment timed out
		}).
		Select(ctx)

	if cancelled {
		log.Info("order cancelled or timed out", "orderID", input.OrderID)
		_ = workflow.ExecuteActivity(ctx, activities.ReleaseStock, input.OrderID, input.Items).Get(ctx, nil)
		_ = workflow.ExecuteActivity(ctx, activities.UpdateOrderStatus, input.OrderID, model.StatusCancelled).Get(ctx, nil)
		return nil
	}

	// ── 3. Record payment intent ─────────────────────────────────────────────
	if err := workflow.ExecuteActivity(ctx, activities.RecordPaymentIntent, input.OrderID, paymentIntentID).Get(ctx, nil); err != nil {
		return err
	}
	if err := workflow.ExecuteActivity(ctx, activities.UpdateOrderStatus, input.OrderID, model.StatusPaymentConfirmed).Get(ctx, nil); err != nil {
		return err
	}

	// ── 4. Notify seller / prepare ───────────────────────────────────────────
	if err := workflow.ExecuteActivity(ctx, activities.NotifySeller, input.OrderID).Get(ctx, nil); err != nil {
		log.Warn("seller notification failed (non-fatal)", "error", err)
	}
	if err := workflow.ExecuteActivity(ctx, activities.UpdateOrderStatus, input.OrderID, model.StatusPreparing).Get(ctx, nil); err != nil {
		return err
	}

	// ── 5. Ship ───────────────────────────────────────────────────────────────
	shipOpts := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
		},
	}
	if err := workflow.ExecuteActivity(workflow.WithActivityOptions(ctx, shipOpts), activities.ShipOrder, input.OrderID).Get(ctx, nil); err != nil {
		return err
	}
	if err := workflow.ExecuteActivity(ctx, activities.UpdateOrderStatus, input.OrderID, model.StatusShipped).Get(ctx, nil); err != nil {
		return err
	}

	// ── 6. Await delivery confirmation (up to 15 days) ───────────────────────
	deliveryTimer := workflow.NewTimer(ctx, 15*24*time.Hour)
	deliveryCh := workflow.GetSignalChannel(ctx, "delivery-confirmed")

	var delivered bool
	workflow.NewSelector(ctx).
		AddReceive(deliveryCh, func(c workflow.ReceiveChannel, more bool) {
			delivered = true
		}).
		AddFuture(deliveryTimer, func(f workflow.Future) {
			// Auto-confirm after 15 days if no dispute
			delivered = true
		}).
		Select(ctx)

	if delivered {
		_ = workflow.ExecuteActivity(ctx, activities.UpdateOrderStatus, input.OrderID, model.StatusDelivered).Get(ctx, nil)
	}

	log.Info("OrderWorkflow completed", "orderID", input.OrderID)
	return nil
}
