package middleware

import (
	"net/http"
	"sync"
	"time"
)

type RateLimiter struct {
	mu      sync.Mutex
	windows map[string]*visitWindow
	limit   int
	window  time.Duration
}

type visitWindow struct {
	count   int
	startAt time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		windows: make(map[string]*visitWindow),
		limit:   limit,
		window:  window,
	}
}

func (rl *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		now := time.Now()

		rl.mu.Lock()
		win, ok := rl.windows[ip]
		if !ok || now.Sub(win.startAt) > rl.window {
			win = &visitWindow{count: 0, startAt: now}
			rl.windows[ip] = win
		}
		win.count++
		count := win.count
		rl.mu.Unlock()

		if count > rl.limit {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
