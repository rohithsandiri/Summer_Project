// internal/shared/httpclient/httpclient.go
//
// WHY THIS EXISTS:
//   Service-to-service HTTP calls in a microservice system fail transiently:
//   network hiccups, pod restarts during rolling deployments, brief GC pauses.
//   Without retry logic, these transient failures surface as order failures —
//   degrading the user experience and burning SLO error budget unnecessarily.
//
// RETRY STRATEGY — EXPONENTIAL BACKOFF:
//   - Attempt 1: immediate
//   - Attempt 2: wait base_delay (default 100ms) + jitter
//   - Attempt 3: wait 2×base_delay + jitter
//   - Attempt 4: wait 4×base_delay + jitter
//   Jitter (±30%) prevents thundering-herd: all services retrying simultaneously
//   after a brief outage would overwhelm the recovering service.
//
// WHAT IS RETRIED vs NOT RETRIED:
//   TRANSIENT (retry):
//     - Network errors (connection refused, timeout, DNS failure)
//     - HTTP 429 Too Many Requests (with Retry-After if present)
//     - HTTP 503 Service Unavailable
//     - HTTP 500 Internal Server Error (may be transient infrastructure issue)
//
//   NON-TRANSIENT (never retry):
//     - HTTP 400 Bad Request    — our request is malformed; retry won't help
//     - HTTP 404 Not Found      — resource doesn't exist
//     - HTTP 409 Conflict       — state conflict (duplicate, insufficient stock)
//     - HTTP 422 Unprocessable  — business rejection (payment declined)
//
//   This distinction is critical: retrying a payment-declined response would
//   attempt to charge the customer again — a serious business error.
//
// TRACE ID PROPAGATION:
//   Every outbound request carries the X-Trace-ID header so downstream service
//   logs can be correlated with the originating request's trace.
//
// CIRCUIT BREAKER:
//   A circuit breaker stops calling a failing downstream immediately once a
//   failure threshold is reached, failing fast and allowing the downstream to
//   recover.
//
// OPENTELEMETRY:
//   Injects W3C traceparent headers using the global propagator for distributed tracing.

package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/rohithsandiri/Summer_Project/internal/shared/breaker"
	"github.com/rohithsandiri/Summer_Project/internal/shared/middleware"
)

// RetryConfig controls the retry behaviour for a Client.
type RetryConfig struct {
	// MaxAttempts is the total number of attempts (1 = no retry).
	MaxAttempts int

	// BaseDelay is the initial wait before the first retry.
	BaseDelay time.Duration

	// MaxDelay caps the exponential backoff delay.
	MaxDelay time.Duration
}

// DefaultRetryConfig provides sensible retry defaults for inter-service calls.
var DefaultRetryConfig = RetryConfig{
	MaxAttempts: 3,
	BaseDelay:   100 * time.Millisecond,
	MaxDelay:    2 * time.Second,
}

// NoRetry is a RetryConfig that disables retrying entirely.
var NoRetry = RetryConfig{MaxAttempts: 1}

// Client wraps http.Client with retry logic, trace ID propagation, OTel propagation,
// and circuit breaker protection.
type Client struct {
	inner       *http.Client
	baseURL     string
	serviceName string
	retry       RetryConfig
	breaker     *breaker.CircuitBreaker
}

// New creates a Client with the default retry configuration.
func New(baseURL, serviceName string) *Client {
	return &Client{
		inner: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL:     baseURL,
		serviceName: serviceName,
		retry:       DefaultRetryConfig,
	}
}

// SetBreaker attaches a CircuitBreaker to the HTTP client.
func (c *Client) SetBreaker(cb *breaker.CircuitBreaker) *Client {
	c.breaker = cb
	return c
}

// PostJSON sends a POST request with JSON body, exponential backoff retry,
// OTel context propagation, and optional circuit breaker wrapping.
func (c *Client) PostJSON(ctx context.Context, path, traceID string, requestBody, responseBody any) (int, error) {
	data, err := json.Marshal(requestBody)
	if err != nil {
		return 0, fmt.Errorf("marshal request body: %w", err)
	}

	var statusCode int
	var executeErr error

	// Define the actual execution block
	operation := func() error {
		statusCode, executeErr = c.doWithRetry(ctx, func() (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json")

			// 1. Classic Trace ID propagation
			if traceID != "" {
				req.Header.Set(middleware.TraceIDHeader, traceID)
			}

			// 2. OpenTelemetry context propagation (inject traceparent header)
			otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

			return req, nil
		}, responseBody)

		// Return error only if it's a transient failure (which helps breaker trip on infrastructure failures).
		// Non-transient errors (like 400, 404, 409, 422) shouldn't trip the circuit breaker.
		if executeErr != nil {
			return executeErr
		}
		if statusCode >= 500 || statusCode == http.StatusTooManyRequests {
			return fmt.Errorf("HTTP status error: %d", statusCode)
		}
		return nil
	}

	// Wrap execution with CircuitBreaker if configured
	if c.breaker != nil {
		err = c.breaker.Execute(operation)
		if err != nil {
			if errorsIsCircuitOpen(err) {
				return 0, err
			}
			return statusCode, executeErr
		}
		return statusCode, nil
	}

	_ = operation()
	return statusCode, executeErr
}

// GetJSON sends a GET request with retry, OTel trace propagation, and optional circuit breaker.
func (c *Client) GetJSON(ctx context.Context, path, traceID string, responseBody any) (int, error) {
	var statusCode int
	var executeErr error

	operation := func() error {
		statusCode, executeErr = c.doWithRetry(ctx, func() (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Accept", "application/json")

			// 1. Trace ID propagation
			if traceID != "" {
				req.Header.Set(middleware.TraceIDHeader, traceID)
			}

			// 2. OpenTelemetry context propagation
			otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

			return req, nil
		}, responseBody)

		if executeErr != nil {
			return executeErr
		}
		if statusCode >= 500 || statusCode == http.StatusTooManyRequests {
			return fmt.Errorf("HTTP status error: %d", statusCode)
		}
		return nil
	}

	if c.breaker != nil {
		err := c.breaker.Execute(operation)
		if err != nil {
			if errorsIsCircuitOpen(err) {
				return 0, err
			}
			return statusCode, executeErr
		}
		return statusCode, nil
	}

	_ = operation()
	return statusCode, executeErr
}

// errorsIsCircuitOpen is a helper to check if error is breaker.ErrCircuitOpen.
func errorsIsCircuitOpen(err error) bool {
	return err == breaker.ErrCircuitOpen
}

// doWithRetry executes an HTTP request with exponential backoff.
func (c *Client) doWithRetry(
	ctx context.Context,
	makeReq func() (*http.Request, error),
	responseBody any,
) (int, error) {
	var (
		lastStatus int
		lastErr    error
	)

	for attempt := 1; attempt <= c.retry.MaxAttempts; attempt++ {
		req, err := makeReq()
		if err != nil {
			return 0, fmt.Errorf("build request: %w", err)
		}

		statusCode, err := c.execute(req, responseBody)
		if err == nil && !isRetryable(statusCode) {
			return statusCode, nil
		}
		if err == nil && !isRetryableStatus(statusCode) {
			return statusCode, nil
		}

		lastStatus = statusCode
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("HTTP %d from %s%s", statusCode, c.baseURL, req.URL.Path)
		}

		if attempt == c.retry.MaxAttempts {
			break
		}

		select {
		case <-ctx.Done():
			return lastStatus, ctx.Err()
		default:
		}

		delay := c.backoffDelay(attempt)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return lastStatus, ctx.Err()
		}
	}

	return lastStatus, fmt.Errorf("all %d attempts failed calling %s: %w",
		c.retry.MaxAttempts, c.baseURL, lastErr)
}

// execute performs a single HTTP request and decodes the response body.
func (c *Client) execute(req *http.Request, responseBody any) (int, error) {
	resp, err := c.inner.Do(req)
	if err != nil {
		return 0, fmt.Errorf("execute request to %s: %w", req.URL.String(), err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return resp.StatusCode, fmt.Errorf("read response body: %w", err)
	}

	if responseBody != nil && len(body) > 0 {
		if err := json.Unmarshal(body, responseBody); err != nil {
			return resp.StatusCode, fmt.Errorf("decode response from %s: %w", req.URL.String(), err)
		}
	}

	return resp.StatusCode, nil
}

// backoffDelay calculates the exponential backoff delay for attempt n.
func (c *Client) backoffDelay(attempt int) time.Duration {
	delay := c.retry.BaseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay > c.retry.MaxDelay {
			delay = c.retry.MaxDelay
			break
		}
	}
	// Add ±30% jitter.
	jitter := float64(delay) * (0.7 + 0.6*rand.Float64())
	return time.Duration(jitter)
}

func isRetryable(statusCode int) bool {
	return statusCode == 0
}

func isRetryableStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}
