package api

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// clientBucket tracks token-bucket state for a single client IP.
type clientBucket struct {
	tokens     float64
	lastRefill time.Time
}

// IPRateLimiter provides in-memory token-bucket rate limiting by client IP address.
type IPRateLimiter struct {
	mu           sync.Mutex
	clients      map[string]*clientBucket
	rate         float64 // tokens replenished per second
	burst        float64 // maximum token burst capacity
	cleanupTimer *time.Ticker
	stopChan     chan struct{}
}

// NewIPRateLimiter creates a new IPRateLimiter.
// rate: tokens added per second (e.g., 10.0).
// burst: maximum capacity for burst requests (e.g., 20).
func NewIPRateLimiter(rate float64, burst int) *IPRateLimiter {
	limiter := &IPRateLimiter{
		clients:      make(map[string]*clientBucket),
		rate:         rate,
		burst:        float64(burst),
		cleanupTimer: time.NewTicker(2 * time.Minute),
		stopChan:     make(chan struct{}),
	}

	go limiter.cleanupLoop()
	return limiter
}

// Close gracefully stops the background cleanup goroutine.
func (l *IPRateLimiter) Close() {
	if l.cleanupTimer != nil {
		l.cleanupTimer.Stop()
	}
	select {
	case <-l.stopChan:
	default:
		close(l.stopChan)
	}
}

func (l *IPRateLimiter) cleanupLoop() {
	for {
		select {
		case <-l.stopChan:
			return
		case now := <-l.cleanupTimer.C:
			l.mu.Lock()
			for ip, b := range l.clients {
				// Evict clients idle for longer than 5 minutes to reclaim memory
				if now.Sub(b.lastRefill) > 5*time.Minute {
					delete(l.clients, ip)
				}
			}
			l.mu.Unlock()
		}
	}
}

// Allow checks if the given client IP has available tokens.
func (l *IPRateLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, exists := l.clients[ip]
	if !exists {
		l.clients[ip] = &clientBucket{
			tokens:     l.burst - 1,
			lastRefill: now,
		}
		return true
	}

	// Refill tokens based on elapsed duration
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.lastRefill = now

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}

	return false
}

// Middleware returns an HTTP handler wrapping the rate limiter.
func (l *IPRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractClientIP(r)
		if !l.Allow(ip) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "too many requests, please slow down",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return strings.TrimSpace(xrip)
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

