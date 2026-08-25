package ratelimit

import (
	"sync"
	"time"
)

// windowEntry tracks request timestamps for a single client ID.
type windowEntry struct {
	mu       sync.Mutex
	requests []time.Time
}

// Limiter is a per-client in-memory sliding window rate limiter.
// It is safe for concurrent use by multiple goroutines.
type Limiter struct {
	mu          sync.RWMutex
	clients     map[string]*windowEntry
	maxRequests int
	window      time.Duration
}

// New creates a Limiter allowing maxRequests within the given window per client.
// Example: New(60, time.Minute) → 60 requests per minute per client.
func New(maxRequests int, window time.Duration) *Limiter {
	l := &Limiter{
		clients:     make(map[string]*windowEntry),
		maxRequests: maxRequests,
		window:      window,
	}
	go l.cleanup()
	return l
}

// Allow returns true if the clientID has not exceeded the rate limit.
// It is non-blocking and safe for concurrent calls.
func (l *Limiter) Allow(clientID string) bool {
	l.mu.RLock()
	entry, ok := l.clients[clientID]
	l.mu.RUnlock()

	if !ok {
		l.mu.Lock()
		// Double-check after write lock to avoid race.
		if entry, ok = l.clients[clientID]; !ok {
			entry = &windowEntry{}
			l.clients[clientID] = entry
		}
		l.mu.Unlock()
	}

	now := time.Now()
	cutoff := now.Add(-l.window)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	// Evict requests that fall outside the sliding window.
	valid := entry.requests[:0]
	for _, t := range entry.requests {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	entry.requests = valid

	if len(entry.requests) >= l.maxRequests {
		return false // rate limit exceeded
	}

	entry.requests = append(entry.requests, now)
	return true
}

// cleanup removes idle client entries every 5 minutes to prevent unbounded memory growth.
func (l *Limiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		l.mu.Lock()
		for id, entry := range l.clients {
			entry.mu.Lock()
			if len(entry.requests) == 0 {
				delete(l.clients, id)
			}
			entry.mu.Unlock()
		}
		l.mu.Unlock()
	}
}

// dailyCounter tracks daily message usage for a client.
type dailyCounter struct {
	count int
	date  string // YYYY-MM-DD
}

// DailyLimiter enforces per-client daily message limits based on their subscription tier.
type DailyLimiter struct {
	mu     sync.Mutex
	counts map[string]*dailyCounter
}

// PlanLimit returns the daily message quota for a given billing plan tier.
// Returns -1 for unlimited plans.
func PlanLimit(plan string) int {
	switch plan {
	case "Basic":
		return 200
	case "Pro":
		return 1000
	case "Enterprise":
		return -1 // unlimited
	case "Custom":
		return -1 // unlimited
	default:
		return 200
	}
}

// NewDailyLimiter creates a new plan-based daily rate limiter.
func NewDailyLimiter() *DailyLimiter {
	d := &DailyLimiter{
		counts: make(map[string]*dailyCounter),
	}
	go d.cleanup()
	return d
}

// AllowWithPlan checks if clientID with the given plan can send a message today.
// Returns (allowed bool, currentCount int, limit int).
func (d *DailyLimiter) AllowWithPlan(clientID, plan string) (bool, int, int) {
	limit := PlanLimit(plan)
	if limit == -1 {
		return true, 0, -1 // unlimited
	}

	today := time.Now().Format("2006-01-02")

	d.mu.Lock()
	defer d.mu.Unlock()

	counter, exists := d.counts[clientID]
	if !exists || counter.date != today {
		counter = &dailyCounter{count: 0, date: today}
		d.counts[clientID] = counter
	}

	if counter.count >= limit {
		return false, counter.count, limit
	}

	counter.count++
	return true, counter.count, limit
}

// cleanup removes past dates entries from memory every hour.
func (d *DailyLimiter) cleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		today := time.Now().Format("2006-01-02")
		d.mu.Lock()
		for id, counter := range d.counts {
			if counter.date != today {
				delete(d.counts, id)
			}
		}
		d.mu.Unlock()
	}
}

