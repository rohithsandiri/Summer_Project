// internal/shared/response/response.go
//
// WHY THIS EXISTS:
//   All services return the same JSON envelope shape. Consistent structure
//   means clients (including the API Gateway and Kubernetes Operator) can parse
//   any service response without special-casing per service.
//
// ENVELOPE DESIGN:
//   Success:  { "data": {...}, "trace_id": "...", "timestamp": "..." }
//   Error:    { "error": { "code": "...", "message": "...", "category": "..." },
//               "trace_id": "...", "timestamp": "..." }
//
// APPERROR INTEGRATION:
//   AppError (from apperrors package) carries HTTPStatus, Code, Category and
//   Message. The WriteAppError helper extracts all of these automatically,
//   so handlers never need a switch statement to determine the status code.

package response

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/shared/apperrors"
)

// SuccessEnvelope is the standard shape for all successful API responses.
type SuccessEnvelope struct {
	Data      any    `json:"data"`
	TraceID   string `json:"trace_id"`
	Timestamp string `json:"timestamp"`
}

// ErrorDetail carries machine-readable error information.
type ErrorDetail struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Category string `json:"category,omitempty"`
}

// ErrorEnvelope is the standard shape for all error API responses.
type ErrorEnvelope struct {
	Error     ErrorDetail `json:"error"`
	TraceID   string      `json:"trace_id"`
	Timestamp string      `json:"timestamp"`
}

// JSON writes any value as a JSON response with the given HTTP status.
func JSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// Success writes a JSON success envelope.
func Success(w http.ResponseWriter, status int, traceID string, data any) {
	JSON(w, status, SuccessEnvelope{
		Data:      data,
		TraceID:   traceID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// Error writes a JSON error envelope with explicit code, category, and message.
func Error(w http.ResponseWriter, status int, traceID, code, message string) {
	JSON(w, status, ErrorEnvelope{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
		TraceID:   traceID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// WriteAppError writes the correct HTTP response for an *AppError.
// It extracts HTTPStatus, Code, Category, and Message automatically.
// This is the primary error-writing function — handlers should prefer this
// over calling Error() with manually specified status codes.
func WriteAppError(w http.ResponseWriter, traceID string, ae *apperrors.AppError) {
	JSON(w, ae.HTTPStatus, ErrorEnvelope{
		Error: ErrorDetail{
			Code:     ae.Code,
			Message:  ae.Message,
			Category: string(ae.Category),
		},
		TraceID:   traceID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}
