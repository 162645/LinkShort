package idgen

import (
	"context"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

// Generator handles short code generation using Redis INCR and Base62
type Generator struct {
	redisClient *redis.Client
	key         string
	// Shuffled alphabet to prevent easy sequential guessing
	alphabet string
	base     int64
}

// Default shuffled alphabet (can be changed via config in production)
// This permutation acts as a symmetric encryption key
const defaultAlphabet = "5J8M49p3N7bC2qXz6PvR1wAyZkGfLhEsTjDuQoWmBiVaKcxYnrtdeS0glUIOFH"

func NewGenerator(redisClient *redis.Client, key string) *Generator {
	return &Generator{
		redisClient: redisClient,
		key:         key,
		alphabet:    defaultAlphabet,
		base:        int64(len(defaultAlphabet)),
	}
}

// GenerateID create a new unique short ID
func (g *Generator) GenerateID(ctx context.Context) (string, error) {
	// 1. Get atomic ID from Redis
	// We start from 10,000,000,000 to ensure we have at least 6 chars
	// 62^5 = ~900 million. 62^6 = ~56 billion.
	// Starting at 20,000,000,000 ensures 6 chars firmly.
	id, err := g.redisClient.Incr(ctx, g.key).Result()
	if err != nil {
		return "", fmt.Errorf("redis incr failed: %w", err)
	}

	// 2. Base62 Encode
	return g.encode(id), nil
}

func (g *Generator) encode(num int64) string {
	if num == 0 {
		return string(g.alphabet[0])
	}

	var sb strings.Builder
	for num > 0 {
		rem := num % g.base
		num = num / g.base
		sb.WriteByte(g.alphabet[rem])
	}

	// Reverse string
	chars := []rune(sb.String())
	for i, j := 0, len(chars)-1; i < j; i, j = i+1, j-1 {
		chars[i], chars[j] = chars[j], chars[i]
	}

	return string(chars)
}

// InitializeCounter sets the initial value if not set.
// It uses SetNX to ensure we don't overwrite an existing counter in production.
func (g *Generator) InitializeCounter(ctx context.Context, start int64) error {
	// Only set if not exists to avoid resetting in production
	ok, err := g.redisClient.SetNX(ctx, g.key, start, 0).Result()
	if err != nil {
		return err
	}
	if ok {
		fmt.Printf("Initialized ID counter to %d\n", start)
	} else {
		// Optional: Check if current value is suspiciously low (< start) and bump it?
		// For now, trust Redis persistence.
		fmt.Printf("ID counter already exists, skipping initialization.\n")
	}
	return nil
}
