// internal/shared/apperrors/errors.go
//
// WHY THIS EXISTS:
//   Generic Go errors (errors.New, fmt.Errorf) carry only a string message.
//   In a distributed system with structured logging and Prometheus metrics,
//   we need errors that carry:
//     - A machine-readable code (for API clients and SLO dashboards)
//     - A human-readable message (for developers)
//     - An HTTP status code (so handlers don't need a giant switch)
//     - An error type category (for metrics labels: "not_found", "conflict", etc.)
//
//   This package is the single source of truth for every domain error in the
//   system. Every service imports it; no service defines its own ad-hoc errors.
//
// DESIGN — errors.As COMPATIBILITY:
//   AppError implements the error interface. It also embeds an optional Cause
//   for error chain traversal via errors.Is / errors.As, which is critical for
//   middleware that translates errors to HTTP responses.
//
// ERROR CATEGORIES (used as Prometheus label values):
//   "not_found"    → 404 — resource does not exist
//   "conflict"     → 409 — state conflict (duplicate, already processed)
//   "validation"   → 400 — client sent bad input
//   "unavailable"  → 503 — downstream service not reachable
//   "declined"     → 422 — business rejection (insufficient funds, no stock)
//   "internal"     → 500 — unexpected infrastructure error
//
// FUTURE PHASES:
//   - Add gRPC status code mapping for when gRPC transport is introduced
//   - Add error.Unwrap() chain support for OpenTelemetry span attributes

package apperrors

import (
	"fmt"
	"net/http"
)

// ErrorCategory is a Prometheus-safe label value categorising errors.
// Low-cardinality — never use dynamic strings here.
type ErrorCategory string

const (
	CategoryNotFound    ErrorCategory = "not_found"
	CategoryConflict    ErrorCategory = "conflict"
	CategoryValidation  ErrorCategory = "validation"
	CategoryUnavailable ErrorCategory = "unavailable"
	CategoryDeclined    ErrorCategory = "declined"
	CategoryInternal    ErrorCategory = "internal"
)

// AppError is the standard domain error type used across all services.
// It carries everything the system needs to respond correctly to failures.
type AppError struct {
	// Code is a SCREAMING_SNAKE_CASE constant for programmatic handling.
	// e.g. "ORDER_NOT_FOUND", "INVENTORY_UNAVAILABLE"
	Code string

	// Message is a human-readable description for developers / API consumers.
	Message string

	// HTTPStatus is the HTTP response code this error maps to.
	// Pre-computed here so handlers never need a switch statement.
	HTTPStatus int

	// Category is a low-cardinality label for Prometheus error metrics.
	Category ErrorCategory

	// Cause is the underlying error that triggered this AppError.
	// Populated when wrapping infrastructure errors (DB, HTTP client failures).
	Cause error
}

// Error implements the error interface.
// Returns a consistent string format: "[CODE] message (cause: ...)"
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause so errors.Is / errors.As work across
// wrapped error chains. Critical for middleware that inspects errors.
func (e *AppError) Unwrap() error {
	return e.Cause
}

// Is reports whether target matches this error by comparing Code fields.
// This allows: errors.Is(err, ErrOrderNotFound) even when err is wrapped.
func (e *AppError) Is(target error) bool {
	t, ok := target.(*AppError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// Wrap creates a new AppError with a cause, preserving the original error
// for log chain inspection while providing a domain-specific code.
func (e *AppError) Wrap(cause error) *AppError {
	return &AppError{
		Code:       e.Code,
		Message:    e.Message,
		HTTPStatus: e.HTTPStatus,
		Category:   e.Category,
		Cause:      cause,
	}
}

// WithMessage creates a copy of this error with a different message.
// Use this to add context without changing the error code.
func (e *AppError) WithMessage(msg string) *AppError {
	return &AppError{
		Code:       e.Code,
		Message:    msg,
		HTTPStatus: e.HTTPStatus,
		Category:   e.Category,
		Cause:      e.Cause,
	}
}

// ─── Order Domain Errors ──────────────────────────────────────────────────────

// ErrOrderNotFound is returned when an order ID does not exist in the store.
var ErrOrderNotFound = &AppError{
	Code:       "ORDER_NOT_FOUND",
	Message:    "order not found",
	HTTPStatus: http.StatusNotFound,
	Category:   CategoryNotFound,
}

// ErrDuplicateOrder is returned when an order with the same idempotency key
// already exists. Prevents double-charging on network retries.
var ErrDuplicateOrder = &AppError{
	Code:       "DUPLICATE_ORDER",
	Message:    "an order with this idempotency key already exists",
	HTTPStatus: http.StatusConflict,
	Category:   CategoryConflict,
}

// ErrOrderValidation is returned when the create-order request fails validation.
var ErrOrderValidation = &AppError{
	Code:       "ORDER_VALIDATION_FAILED",
	Message:    "order request validation failed",
	HTTPStatus: http.StatusBadRequest,
	Category:   CategoryValidation,
}

// ErrOrderFailed is returned when the saga cannot complete — e.g. inventory
// unavailable or payment declined. The message is augmented per call site.
var ErrOrderFailed = &AppError{
	Code:       "ORDER_FAILED",
	Message:    "order could not be completed",
	HTTPStatus: http.StatusUnprocessableEntity,
	Category:   CategoryDeclined,
}

// ─── Inventory Domain Errors ──────────────────────────────────────────────────

// ErrInventoryItemNotFound is returned when the requested SKU does not exist.
var ErrInventoryItemNotFound = &AppError{
	Code:       "INVENTORY_ITEM_NOT_FOUND",
	Message:    "inventory item not found",
	HTTPStatus: http.StatusNotFound,
	Category:   CategoryNotFound,
}

// ErrInventoryUnavailable is returned when stock is insufficient to fulfil
// the reservation request.
var ErrInventoryUnavailable = &AppError{
	Code:       "INVENTORY_UNAVAILABLE",
	Message:    "insufficient stock to fulfil reservation",
	HTTPStatus: http.StatusConflict,
	Category:   CategoryDeclined,
}

// ErrInvalidInventory is returned when inventory state is internally inconsistent.
var ErrInvalidInventory = &AppError{
	Code:       "INVALID_INVENTORY_STATE",
	Message:    "inventory item is in an invalid state",
	HTTPStatus: http.StatusInternalServerError,
	Category:   CategoryInternal,
}

// ErrInventoryValidation is returned when reserve/release request params are invalid.
var ErrInventoryValidation = &AppError{
	Code:       "INVENTORY_VALIDATION_FAILED",
	Message:    "inventory request validation failed",
	HTTPStatus: http.StatusBadRequest,
	Category:   CategoryValidation,
}

// ─── Payment Domain Errors ────────────────────────────────────────────────────

// ErrPaymentNotFound is returned when a payment ID does not exist.
var ErrPaymentNotFound = &AppError{
	Code:       "PAYMENT_NOT_FOUND",
	Message:    "payment record not found",
	HTTPStatus: http.StatusNotFound,
	Category:   CategoryNotFound,
}

// ErrPaymentDeclined is returned when the payment gateway declines the charge.
// This is a business outcome — NOT an infrastructure error. HTTP 422.
var ErrPaymentDeclined = &AppError{
	Code:       "PAYMENT_DECLINED",
	Message:    "payment was declined by the payment gateway",
	HTTPStatus: http.StatusUnprocessableEntity,
	Category:   CategoryDeclined,
}

// ErrPaymentAlreadyRefunded is returned on double-refund attempts.
var ErrPaymentAlreadyRefunded = &AppError{
	Code:       "PAYMENT_ALREADY_REFUNDED",
	Message:    "payment has already been refunded",
	HTTPStatus: http.StatusConflict,
	Category:   CategoryConflict,
}

// ErrPaymentNotRefundable is returned when trying to refund a non-succeeded payment.
var ErrPaymentNotRefundable = &AppError{
	Code:       "PAYMENT_NOT_REFUNDABLE",
	Message:    "only succeeded payments can be refunded",
	HTTPStatus: http.StatusConflict,
	Category:   CategoryConflict,
}

// ErrPaymentValidation is returned when the process/refund request is malformed.
var ErrPaymentValidation = &AppError{
	Code:       "PAYMENT_VALIDATION_FAILED",
	Message:    "payment request validation failed",
	HTTPStatus: http.StatusBadRequest,
	Category:   CategoryValidation,
}

// ─── Infrastructure Errors ────────────────────────────────────────────────────

// ErrDownstreamUnavailable is returned when an inter-service HTTP call fails
// due to a network error (connection refused, timeout). This is TRANSIENT —
// the retry mechanism should attempt the call again.
var ErrDownstreamUnavailable = &AppError{
	Code:       "DOWNSTREAM_UNAVAILABLE",
	Message:    "a downstream service is temporarily unavailable",
	HTTPStatus: http.StatusServiceUnavailable,
	Category:   CategoryUnavailable,
}

// ErrInternal is a catch-all for unexpected infrastructure errors.
var ErrInternal = &AppError{
	Code:       "INTERNAL_ERROR",
	Message:    "an unexpected internal error occurred",
	HTTPStatus: http.StatusInternalServerError,
	Category:   CategoryInternal,
}

// ─── Helper Functions ─────────────────────────────────────────────────────────

// IsAppError reports whether err is an *AppError.
func IsAppError(err error) bool {
	var ae *AppError
	return err != nil && fmt.Sprintf("%T", err) == fmt.Sprintf("%T", ae) || asAppError(err) != nil
}

// AsAppError extracts the *AppError from an error chain, or returns nil.
func AsAppError(err error) *AppError {
	return asAppError(err)
}

func asAppError(err error) *AppError {
	if err == nil {
		return nil
	}
	if ae, ok := err.(*AppError); ok {
		return ae
	}
	return nil
}
