package internal

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, _ := strings.Cut(r.RemoteAddr, ":")
	return host
}

func itoa(n int) string { return strconv.Itoa(n) }

// RedisLimiter is a fixed-window counter in Redis shared across gateway
// instances. A window is a key that expires after the window duration; the
// first request in a window sets the TTL.
type RedisLimiter struct{ rdb *redis.Client }

func NewRedisLimiter(rdb *redis.Client) *RedisLimiter { return &RedisLimiter{rdb: rdb} }

func (l *RedisLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error) {
	bucket := time.Now().Truncate(window).Unix()
	rk := "rl:" + key + ":" + strconv.FormatInt(bucket, 10)
	pipe := l.rdb.TxPipeline()
	incr := pipe.Incr(ctx, rk)
	pipe.Expire(ctx, rk, window+time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return true, limit, err // fail open
	}
	count := int(incr.Val())
	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}
	return count <= limit, remaining, nil
}

// MemoryLimiter is a per-instance fallback used when Redis is not configured.
type MemoryLimiter struct {
	mu      sync.Mutex
	windows map[string]*memWindow
}

type memWindow struct {
	count int
	reset time.Time
}

func NewMemoryLimiter() *MemoryLimiter {
	m := &MemoryLimiter{windows: map[string]*memWindow{}}
	go func() {
		for range time.Tick(time.Minute) {
			m.mu.Lock()
			for k, w := range m.windows {
				if time.Now().After(w.reset) {
					delete(m.windows, k)
				}
			}
			m.mu.Unlock()
		}
	}()
	return m
}

func (m *MemoryLimiter) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.windows[key]
	if !ok || time.Now().After(w.reset) {
		w = &memWindow{reset: time.Now().Add(window)}
		m.windows[key] = w
	}
	w.count++
	remaining := limit - w.count
	if remaining < 0 {
		remaining = 0
	}
	return w.count <= limit, remaining, nil
}
