package ratelimit

import (
	"testing"
	"time"
)

func TestPlanLimit(t *testing.T) {
	tests := []struct {
		plan     string
		expected int
	}{
		{"Basic", 200},
		{"Pro", 1000},
		{"Enterprise", -1},
		{"Custom", -1},
		{"Unknown", 200},
	}

	for _, tt := range tests {
		got := PlanLimit(tt.plan)
		if got != tt.expected {
			t.Errorf("PlanLimit(%q) = %d; want %d", tt.plan, got, tt.expected)
		}
	}
}

func TestDailyLimiter(t *testing.T) {
	dl := NewDailyLimiter()

	// Basic plan allows 200 messages/day
	for i := 1; i <= 200; i++ {
		allowed, count, limit := dl.AllowWithPlan("client-basic-1", "Basic")
		if !allowed {
			t.Fatalf("expected message %d to be allowed", i)
		}
		if count != i {
			t.Errorf("expected count %d, got %d", i, count)
		}
		if limit != 200 {
			t.Errorf("expected limit 200, got %d", limit)
		}
	}

	// 201st request should be rejected
	allowed, count, limit := dl.AllowWithPlan("client-basic-1", "Basic")
	if allowed {
		t.Errorf("expected 201st request to be blocked")
	}
	if count != 200 {
		t.Errorf("expected count to stay at 200, got %d", count)
	}
	if limit != 200 {
		t.Errorf("expected limit to be 200, got %d", limit)
	}

	// Enterprise plan should be unlimited
	for i := 0; i < 500; i++ {
		allowed, _, limit := dl.AllowWithPlan("client-ent-1", "Enterprise")
		if !allowed || limit != -1 {
			t.Fatalf("Enterprise plan should be unlimited, failed on iteration %d", i)
		}
	}
}

func TestSlidingWindowLimiter(t *testing.T) {
	l := New(5, 500*time.Millisecond)

	// Send 5 requests — all should be allowed
	for i := 1; i <= 5; i++ {
		if !l.Allow("client-1") {
			t.Fatalf("request %d should be allowed", i)
		}
	}

	// 6th request should be blocked
	if l.Allow("client-1") {
		t.Errorf("6th request within window should be blocked")
	}

	// Different client should be allowed
	if !l.Allow("client-2") {
		t.Errorf("client-2 should not be affected by client-1's rate limit")
	}

	// Wait for window to expire
	time.Sleep(600 * time.Millisecond)

	// Now client-1 should be allowed again
	if !l.Allow("client-1") {
		t.Errorf("client-1 should be allowed after window expires")
	}
}
