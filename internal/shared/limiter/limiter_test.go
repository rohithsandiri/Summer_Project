// internal/shared/limiter/limiter_test.go

package limiter

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rohithsandiri/Summer_Project/internal/shared/logger"
)

func TestExtractIP(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expected   string
	}{
		{
			name:       "remote addr with port",
			remoteAddr: "192.168.1.50:54321",
			expected:   "192.168.1.50",
		},
		{
			name:       "remote addr raw IP",
			remoteAddr: "127.0.0.1",
			expected:   "127.0.0.1",
		},
		{
			name: "x-forwarded-for single IP",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.195",
			},
			remoteAddr: "10.0.0.1:12345",
			expected:   "203.0.113.195",
		},
		{
			name: "x-forwarded-for multi IP list",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.195, 70.41.3.18, 150.172.238.178",
			},
			remoteAddr: "10.0.0.1:12345",
			expected:   "203.0.113.195",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/test", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			got := ExtractIP(req)
			if got != tt.expected {
				t.Errorf("expected IP %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestRateLimiter_LimitHandler(t *testing.T) {
	rl := NewRateLimiter("test-service", 2.0, 2)
	log := logger.New("test", "error")

	handler := rl.LimitHandler(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Attempt 1: Allow (within burst)
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected status OK, got %d", rr.Code)
	}

	// Attempt 2: Allow (within burst of 2)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Errorf("expected status OK, got %d", rr2.Code)
	}

	// Attempt 3: Reject (burst exceeded)
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req)
	if rr3.Code != http.StatusTooManyRequests {
		t.Errorf("expected status Too Many Requests (429), got %d", rr3.Code)
	}
}
