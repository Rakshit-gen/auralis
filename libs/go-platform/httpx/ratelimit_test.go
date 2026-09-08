package httpx

import "testing"

func TestRateLimiterAllowsBurstThenBlocks(t *testing.T) {
	rl := NewRateLimiter(60, 3) // 1/sec refill, burst 3

	allowed := 0
	for i := 0; i < 10; i++ {
		if rl.Allow("client-a") {
			allowed++
		}
	}
	if allowed != 3 {
		t.Fatalf("expected 3 immediate allowances, got %d", allowed)
	}
	if rl.Allow("client-a") {
		t.Fatal("client-a should be blocked after burst")
	}
	if !rl.Allow("client-b") {
		t.Fatal("client-b must have its own bucket")
	}
}
