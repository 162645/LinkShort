package bloom

import (
	"context"
	"hash/fnv"

	"github.com/redis/go-redis/v9"
)

// RedisBloomFilter implements a simple Bloom Filter backed by Redis Bitmaps
type RedisBloomFilter struct {
	client *redis.Client
	key    string
	size   uint64 // Size of the bitmap in bits
	hashes uint   // Number of hash functions
}

// NewRedisBloomFilter creates a new Redis-backed Bloom Filter
// size: Size of bit array (m). E.g., 100,000,000 for 100M items ~1% false positive
// hashes: Number of hash functions (k). Optimal k = (m/n) * ln(2)
func NewRedisBloomFilter(client *redis.Client, key string, size uint64, hashes uint) *RedisBloomFilter {
	return &RedisBloomFilter{
		client: client,
		key:    key,
		size:   size,
		hashes: hashes,
	}
}

// Add adds a string to the bloom filter
func (bf *RedisBloomFilter) Add(ctx context.Context, item string) error {
	pipe := bf.client.Pipeline()
	for _, offset := range bf.getOffsets(item) {
		pipe.SetBit(ctx, bf.key, int64(offset), 1)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// Exists checks if an item might exist in the bloom filter
func (bf *RedisBloomFilter) Exists(ctx context.Context, item string) (bool, error) {
	// We need to check if ALL bits are set
	// Note: We can optimize this with a single Redis BITFIELD command or Lua script,
	// but Pipeline GetBit is simple and effective for now.
	pipe := bf.client.Pipeline()
	results := make([]*redis.IntCmd, bf.hashes)

	for i, offset := range bf.getOffsets(item) {
		results[i] = pipe.GetBit(ctx, bf.key, int64(offset))
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	for _, cmd := range results {
		val, err := cmd.Result()
		if err != nil {
			return false, err
		}
		if val == 0 {
			return false, nil // Definitely does not exist
		}
	}

	return true, nil // Might exist
}

// getOffsets calculates the k bit positions for an item
// We utilize Double Hashing (Kirsch-Mitzenmacher optimization)
// hash(i) = (hash1 + i * hash2) % size
func (bf *RedisBloomFilter) getOffsets(item string) []uint64 {
	h1, h2 := hash(item)
	offsets := make([]uint64, bf.hashes)

	for i := uint(0); i < bf.hashes; i++ {
		// Calculate hash location: (h1 + i*h2) % size
		// Use uint128 arithmetic logic implicitly via uint64 wrapping?
		// Actually standard implementation usually just adds.
		// To be robust:
		offset := (h1 + uint64(i)*h2) % bf.size
		offsets[i] = offset
	}
	return offsets
}

func hash(s string) (uint64, uint64) {
	h := fnv.New64a()
	h.Write([]byte(s))
	hash1 := h.Sum64()

	// A simple variation for hash2
	h2 := fnv.New64()
	h2.Write([]byte(s))
	// Twist it a bit to ensure it's different
	h2.Write([]byte{1})
	hash2 := h2.Sum64()

	return hash1, hash2
}
