package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	DefaultTTL = 24 * time.Hour
	KeyPrefix  = "idempotency:"
)

// Store handles idempotency keys
type Store struct {
	redis *redis.Client
}

// NewStore creates a new idempotency store
func NewStore(redis *redis.Client) *Store {
	return &Store{
		redis: redis,
	}
}

// Result represents a cached result
type Result struct {
	StatusCode int             `json:"status_code"`
	Body       json.RawMessage `json:"body"`
	CreatedAt  time.Time       `json:"created_at"`
}

// GenerateKey generates an idempotency key from components
func GenerateKey(userID, operation, idempotencyKey string) string {
	data := fmt.Sprintf("%s:%s:%s", userID, operation, idempotencyKey)
	hash := sha256.Sum256([]byte(data))
	return KeyPrefix + hex.EncodeToString(hash[:])
}

// Get retrieves a cached result
func (s *Store) Get(ctx context.Context, key string) (*Result, bool, error) {
	data, err := s.redis.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to get idempotency key: %w", err)
	}

	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return &result, true, nil
}

// Set stores a result with an idempotency key
func (s *Store) Set(ctx context.Context, key string, statusCode int, body interface{}) error {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal body: %w", err)
	}

	result := Result{
		StatusCode: statusCode,
		Body:       bodyBytes,
		CreatedAt:  time.Now(),
	}

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	if err := s.redis.Set(ctx, key, data, DefaultTTL).Err(); err != nil {
		return fmt.Errorf("failed to set idempotency key: %w", err)
	}

	return nil
}

// Delete removes an idempotency key
func (s *Store) Delete(ctx context.Context, key string) error {
	if err := s.redis.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to delete idempotency key: %w", err)
	}
	return nil
}

// SetNX sets a key only if it doesn't exist (for lock-based idempotency)
func (s *Store) SetNX(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	ok, err := s.redis.SetNX(ctx, key, "locked", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("failed to setnx: %w", err)
	}
	return ok, nil
}
