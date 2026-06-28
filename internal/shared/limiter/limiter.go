// internal/shared/limiter/limiter.go
//
// WHY THIS EXISTS:
//   An API Gateway must protect downstreams from DDoS attacks, brute forcing,
//   and resource starvation. Implementing a Token Bucket rate limiter on the
//   gateway ensures that clients are throttled before they hit expensive
//   business logic in backend services.
//
// DESIGN:
//   We use golang.org/x/time/rate, which is the standard Go token bucket.
//   We track limiters in a thread-safe map per IP address. An active cleaner
//   routine occasionally purges stale limiters to prevent memory leaks from
//   short-lived client IPs.

package limiter

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/time/rate"

	"github.com/rohithsandiri/Summer_Project/internal/shared/logger"
	"github.com/rohithsandiri/Summer_Project/internal/shared/middleware"
	"github.com/rohithsandiri/Summer_Project/internal/shared/response"
)

var (
	// RateLimitRejectionsTotal counts every request rejected with HTTP 429.
	RateLimitRejectionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_rate_limit_rejections_total",
			Help: "Total number of requests rejected by the rate limiter.",
		},
		[]string{"service", "path"},
	)
)

// IPClient represents a single client rate limiter state.
type IPClient struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter manages rate limiters for individual client IPs.
type RateLimiter struct {
	mu      sync.Mutex
	clients map[string]*IPClient
	r       rate.Limit
	b       int
	service string
}

// NewRateLimiter creates a RateLimiter that allows r requests/sec with a burst of b.
// Automatically starts a background cleanup goroutine to evict stale IPs.
func NewRateLimiter(service string, r float64, b int) *RateLimiter {
	rl := &RateLimiter{
		clients: make(map[string]*IPClient),
		r:       rate.Limit(r),
		b:       b,
		service: service,
	}

	// Clean up idle client limiters every 5 minutes to prevent memory bloat
	go rl.cleanupLoop(5 * time.Minute)

	return rl
}

func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	client, exists := rl.clients[ip]
	if !exists {
		limiter := rate.NewLimiter(rl.r, rl.b)
		rl.clients[ip] = &IPClient{
			limiter:  limiter,
			lastSeen: time.Now(),
		}
		return limiter
	}

	client.lastSeen = time.Now()
	return client.limiter
}

func (rl *RateLimiter) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, client := range rl.clients {
			// Evict limiters that haven't been seen in 1 hour
			if now.Sub(client.lastSeen) > 1*time.Hour {
				delete(rl.clients, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// ExtractIP extracts the client IP address, checking X-Forwarded-For first.
func ExtractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For: client, proxy1, proxy2
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// LimitHandler wraps http.Handler with rate limiting by client IP.
func (rl *RateLimiter) LimitHandler(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			traceID := middleware.TraceIDFromContext(r)
			ip := ExtractIP(r)

			limiter := rl.getLimiter(ip)
			if !limiter.Allow() {
				RateLimitRejectionsTotal.WithLabelValues(rl.service, r.URL.Path).Inc()

				log.Warn("rate_limit_exceeded",
					"trace_id", traceID,
					"remote_ip", ip,
					"path", r.URL.Path,
				)

				w.Header().Set("Retry-After", "1") // Ask client to retry after 1s
				response.Error(w, http.StatusTooManyRequests, traceID, "RATE_LIMIT_EXCEEDED", "too many requests from this IP, please back off")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
