package web

import (
	"math"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter applies a per-client token bucket.
//
// The client key is the caller's IP address, which is spoofable by a direct
// caller and shared by everyone behind one NAT. It is therefore a blunt
// instrument against accidental hammering and scraping, not a security
// control; authenticated limits belong on the identity, and are added when
// authentication exists.
type RateLimiter struct {
	limit rate.Limit
	burst int

	// idleTTL is how long an unused bucket is kept. Without eviction the map
	// grows once per distinct client address seen since boot, which is a slow
	// memory leak that a scanner can accelerate.
	idleTTL time.Duration

	mu      sync.Mutex
	clients map[string]*client
	stop    chan struct{}
	stopped sync.Once
}

type client struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRateLimiter starts a limiter allowing rps requests per second per client
// with the given burst. Call Close to stop its eviction goroutine.
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	const (
		evictEvery = time.Minute
		idleTTL    = 10 * time.Minute
	)

	rl := &RateLimiter{
		limit:   rate.Limit(rps),
		burst:   burst,
		idleTTL: idleTTL,
		clients: make(map[string]*client),
		stop:    make(chan struct{}),
	}

	go rl.evictLoop(evictEvery)

	return rl
}

// Middleware rejects requests from clients that are over their budget.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limiter := rl.limiterFor(clientIP(r))

		reservation := limiter.Reserve()
		if !reservation.OK() {
			// Only possible when burst is zero, which the config rejects.
			WriteError(w, r, TooManyRequests(1))
			return
		}

		if delay := reservation.Delay(); delay > 0 {
			// Do not make the client wait: tell it when to come back. Holding
			// the request open would let a hammering client consume our
			// goroutines, which is exactly what rate limiting is meant to stop.
			reservation.Cancel()
			WriteError(w, r, TooManyRequests(int(math.Ceil(delay.Seconds()))))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Close stops the eviction goroutine. It is safe to call more than once.
func (rl *RateLimiter) Close() {
	rl.stopped.Do(func() { close(rl.stop) })
}

func (rl *RateLimiter) limiterFor(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if existing, ok := rl.clients[key]; ok {
		existing.lastSeen = time.Now()
		return existing.limiter
	}

	limiter := rate.NewLimiter(rl.limit, rl.burst)
	rl.clients[key] = &client{limiter: limiter, lastSeen: time.Now()}
	return limiter
}

func (rl *RateLimiter) evictLoop(every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stop:
			return
		case <-ticker.C:
			rl.evictIdle()
		}
	}
}

func (rl *RateLimiter) evictIdle() {
	cutoff := time.Now().Add(-rl.idleTTL)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	for key, c := range rl.clients {
		if c.lastSeen.Before(cutoff) {
			delete(rl.clients, key)
		}
	}
}
