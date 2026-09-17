// Package redis wraps go-redis for the platform's only sanctioned uses of
// Redis: caching, distributed locks, and short-lived job coordination (§22).
// Redis is never the system of record — PostgreSQL is authoritative.
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type Client struct {
	rdb *redis.Client
}

func New(addr string) *Client {
	return &Client{rdb: redis.NewClient(&redis.Options{Addr: addr})}
}

func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

func (c *Client) Close() error {
	return c.rdb.Close()
}

// SetCache stores value under key with a TTL.
func (c *Client) SetCache(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

// GetCache returns (value, true) if key exists and has not expired.
func (c *Client) GetCache(ctx context.Context, key string) ([]byte, bool, error) {
	v, err := c.rdb.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return v, true, nil
}

// Lock is a Redis-backed distributed lock (SET NX PX), released via Unlock.
// It is intended for short critical sections (e.g. a workflow step claim
// window), not as a substitute for the Postgres-level SELECT FOR UPDATE SKIP
// LOCKED claim used by the workflow engine itself (ADR-0005).
type Lock struct {
	client *Client
	key    string
	token  string
}

// AcquireLock attempts to acquire a lock named key for ttl. ok is false if
// another holder currently owns it.
func (c *Client) AcquireLock(ctx context.Context, key string, ttl time.Duration) (*Lock, bool, error) {
	token := uuid.NewString()
	ok, err := c.rdb.SetNX(ctx, lockKey(key), token, ttl).Result()
	if err != nil {
		return nil, false, fmt.Errorf("acquiring lock %s: %w", key, err)
	}
	if !ok {
		return nil, false, nil
	}
	return &Lock{client: c, key: key, token: token}, true, nil
}

// Unlock releases the lock only if it is still held by this token, so a
// caller can never release a lock it did not acquire (e.g. after its TTL
// already expired and someone else acquired it).
func (l *Lock) Unlock(ctx context.Context) error {
	const script = `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		end
		return 0
	`
	return l.client.rdb.Eval(ctx, script, []string{lockKey(l.key)}, l.token).Err()
}

func lockKey(key string) string { return "lock:" + key }
