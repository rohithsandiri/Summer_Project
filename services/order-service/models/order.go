// services/order-service/models/order.go
//
// WHY THIS EXISTS:
//   Order models represent the aggregate state of the order workflow.
//   An order is the central entity that ties together inventory reservations
//   and payment transactions. Its state machine drives the orchestration logic.

package models

import "time"

// OrderStatus is the lifecycle state of an order.
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "PENDING"
	OrderStatusConfirmed OrderStatus = "CONFIRMED"
	OrderStatusFailed    OrderStatus = "FAILED"
	OrderStatusCancelled OrderStatus = "CANCELLED"
)

// OrderItem represents a single line item within an order.
type OrderItem struct {
	ItemID    string `json:"item_id"`
	Quantity  int    `json:"quantity"`
	UnitPrice int64  `json:"unit_price"`
}

// Order is the root aggregate for the order domain.
type Order struct {
	OrderID       string      `json:"order_id"`
	CustomerID    string      `json:"customer_id"`
	Items         []OrderItem `json:"items"`
	TotalAmount   int64       `json:"total_amount"`
	Currency      string      `json:"currency"`
	Status        OrderStatus `json:"status"`
	PaymentID     string      `json:"payment_id,omitempty"`
	FailureReason string      `json:"failure_reason,omitempty"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

// CreateOrderRequest is the payload for POST /orders.
type CreateOrderRequest struct {
	CustomerID     string      `json:"customer_id"`
	Items          []OrderItem `json:"items"`
	Currency       string      `json:"currency"`
	IdempotencyKey string      `json:"idempotency_key,omitempty"`
}

// CreateOrderResponse is returned by POST /orders.
type CreateOrderResponse struct {
	Order   *Order `json:"order"`
	Message string `json:"message"`
}
