package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

type rateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	limit    int
	window   time.Duration
}

type visitor struct {
	count    int
	resetAt  time.Time
}

var globalLimiter = &rateLimiter{
	visitors: make(map[string]*visitor),
	limit:    100,
	window:    time.Minute,
}

func init() {
	go func() {
		for {
			time.Sleep(time.Minute)
			globalLimiter.mu.Lock()
			globalLimiter.visitors = make(map[string]*visitor)
			globalLimiter.mu.Unlock()
		}
	}()
}

func RateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			ip = strings.Split(forwarded, ",")[0]
		}

		globalLimiter.mu.Lock()
		v, exists := globalLimiter.visitors[ip]
		if !exists || time.Now().After(v.resetAt) {
			v = &visitor{count: 0, resetAt: time.Now().Add(globalLimiter.window)}
			globalLimiter.visitors[ip] = v
		}

		v.count++
		if v.count > globalLimiter.limit {
			globalLimiter.mu.Unlock()
			http.Error(w, `{"status":"error","message":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		globalLimiter.mu.Unlock()

		next.ServeHTTP(w, r)
	})
}
