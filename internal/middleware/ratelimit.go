package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimit returns a middleware that limits requests per client IP.
// r — requests per second allowed; burst — maximum burst size.
// Idle limiters are garbage-collected after ttl.
func RateLimit(r rate.Limit, burst int, ttl time.Duration) func(http.Handler) http.Handler {
	type entry struct {
		limiter *rate.Limiter
		lastSeen time.Time
	}

	var (
		mu       sync.Mutex
		visitors = map[string]*entry{}
	)

	go func() {
		ticker := time.NewTicker(ttl)
		defer ticker.Stop()
		for range ticker.C {
			mu.Lock()
			for ip, e := range visitors {
				if time.Since(e.lastSeen) > ttl {
					delete(visitors, ip)
				}
			}
			mu.Unlock()
		}
	}()

	get := func(ip string) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()
		if e, ok := visitors[ip]; ok {
			e.lastSeen = time.Now()
			return e.limiter
		}
		lim := rate.NewLimiter(r, burst)
		visitors[ip] = &entry{limiter: lim, lastSeen: time.Now()}
		return lim
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ip := clientIP(req)
			if !get(ip).Allow() {
				w.Header().Set("Retry-After", "1")
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := len(xff); i > 0 {
			for j := 0; j < i; j++ {
				if xff[j] == ',' {
					return xff[:j]
				}
			}
			return xff
		}
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return xr
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
