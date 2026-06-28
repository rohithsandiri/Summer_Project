// services/payment-service/models/payment.go
//
// WHY THIS EXISTS:
//   Payment models represent the financial transaction domain.
//   They are separated from inventory and order models because they belong to a
//   different bounded context with different consistency and compliance requirements.
//   (In a real system, payment data would be in a PCI-DSS compliant store.)
//
// FUTURE PHASES:
//   - Add currency/amount types with proper decimal handling (shopspring/decimal)
//   - Add idempotency key field for safe payment retries
//   - Add audit fields (created_by, ip_address) for compliance

package models

import "time"

// PaymentStatus represents the lifecycle state of a payment transaction.
type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "PENDING"
	PaymentStatusSucceeded PaymentStatus = "SUCCEEDED"
	PaymentStatusFailed    PaymentStatus = "FAILED"
	PaymentStatusRefunded  PaymentStatus = "REFUNDED"
)

// Payment represents a single payment transaction record.
type Payment struct {
	// PaymentID is a unique identifier for this transaction.
	PaymentID string `json:"payment_id"`

	// OrderID links this payment to the originating order.
	OrderID string `json:"order_id"`

	// Amount is the payment amount in the smallest currency unit (e.g. cents).
	Amount int64 `json:"amount"`

	// Currency is the ISO 4217 currency code (e.g. "USD", "EUR").
	Currency string `json:"currency"`

	// Status is the current state of this payment.
	Status PaymentStatus `json:"status"`

	// FailureReason is populated when Status == PaymentStatusFailed.
	FailureReason string `json:"failure_reason,omitempty"`

	// CreatedAt is when this payment record was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is the last time the status changed.
	UpdatedAt time.Time `json:"updated_at"`
}

// ProcessRequest is the payload for POST /payment/process.
type ProcessRequest struct {
	// OrderID links this payment attempt to an order.
	OrderID string `json:"order_id"`

	// Amount in smallest currency unit (cents, pence, etc.).
	Amount int64 `json:"amount"`

	// Currency is the ISO 4217 code.
	Currency string `json:"currency"`
}

// ProcessResponse is returned after a payment attempt (success or failure).
type ProcessResponse struct {
	PaymentID     string        `json:"payment_id"`
	OrderID       string        `json:"order_id"`
	Status        PaymentStatus `json:"status"`
	Amount        int64         `json:"amount"`
	Currency      string        `json:"currency"`
	FailureReason string        `json:"failure_reason,omitempty"`
	ProcessedAt   time.Time     `json:"processed_at"`
}

// RefundRequest is the payload for POST /payment/refund.
type RefundRequest struct {
	// PaymentID identifies the original successful payment to refund.
	PaymentID string `json:"payment_id"`

	// OrderID is used for cross-validation (prevents refunding someone else's payment).
	OrderID string `json:"order_id"`
}

// RefundResponse is returned after a successful refund.
type RefundResponse struct {
	PaymentID  string        `json:"payment_id"`
	OrderID    string        `json:"order_id"`
	Status     PaymentStatus `json:"status"`
	RefundedAt time.Time     `json:"refunded_at"`
}
