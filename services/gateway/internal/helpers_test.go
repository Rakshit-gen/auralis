package internal

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisOutageStillLimits(t *testing.T) {
	// Nothing listens on this port.
	l := NewRedisLimiter(redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1}))
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if ok, _, err := l.Allow(ctx, "ip:1.2.3.4", 3, time.Minute); !ok || err == nil {
			t.Fatalf("request %d: ok=%v err=%v, want allowed with an error", i, ok, err)
		}
	}
	if ok, _, _ := l.Allow(ctx, "ip:1.2.3.4", 3, time.Minute); ok {
		t.Fatal("fourth request allowed while Redis is down")
	}
}
