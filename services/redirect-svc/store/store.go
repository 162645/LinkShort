package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"

	"github.com/go-systems-lab/go-url-shortener/services/url-shortener-svc/domain"
	"github.com/go-systems-lab/go-url-shortener/utils/bloom"
	"github.com/go-systems-lab/go-url-shortener/utils/metrics"
)

// RedirectStore handles URL resolution with cache-first strategy
type RedirectStore struct {
	db         *sqlx.DB
	redis      *redis.Client
	localCache *cache.Cache            // Local In-Memory Cache (L1)
	bloom      *bloom.RedisBloomFilter // Bloom Filter (Anti-Penetration)
	sf         singleflight.Group      // Singleflight (Anti-Breakdown)
}

// CacheEntry represents a cached URL mapping
type CacheEntry struct {
	ShortCode  string     `json:"short_code"`
	LongURL    string     `json:"long_url"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	ClickCount int64      `json:"click_count"`
	IsActive   bool       `json:"is_active"`
}

// dbURLResult represents the database query result
type dbURLResult struct {
	ID           int64      `db:"id"`
	ShortCode    string     `db:"short_code"`
	LongURL      string     `db:"long_url"`
	UserID       string     `db:"user_id"`
	CreatedAt    time.Time  `db:"created_at"`
	ExpiresAt    *time.Time `db:"expires_at"`
	ClickCount   int64      `db:"click_count"`
	LastAccessed *time.Time `db:"last_accessed"`
	IsActive     bool       `db:"is_active"`
}

// NewRedirectStore creates a new redirect store
func NewRedirectStore(db *sqlx.DB, redis *redis.Client) *RedirectStore {
	// Initialize Bloom Filter matched with url-shortener-svc configuration
	bf := bloom.NewRedisBloomFilter(redis, "url:bloom", 100*1000*1000, 5)

	// Initialize Local Cache (e.g., 10 min default expiration, 20 min cleanup)
	lc := cache.New(10*time.Minute, 20*time.Minute)

	return &RedirectStore{
		db:         db,
		redis:      redis,
		localCache: lc,
		bloom:      bf,
	}
}

// ResolveURL resolves a short code to long URL using L1(Local) -> Bloom -> L2(Redis) -> DB
func (s *RedirectStore) ResolveURL(ctx context.Context, shortCode string) (*domain.URL, error) {
	// 1. Check Local Cache (L1) - Fastest, microsecond latency
	if x, found := s.localCache.Get(shortCode); found {
		if entry, ok := x.(CacheEntry); ok {
			// Check expiration
			if entry.ExpiresAt != nil && time.Now().After(*entry.ExpiresAt) {
				s.localCache.Delete(shortCode) // Passive expiration
				// Record L1 miss due to expiration
				metrics.RecordCacheHit("redirect-service", "l1", "miss_expired")
			} else {
				// Record L1 hit
				metrics.RecordCacheHit("redirect-service", "l1", "hit")
				return s.entryToDomain(&entry), nil
			}
		}
	} else {
		// Record L1 miss
		metrics.RecordCacheHit("redirect-service", "l1", "miss")
	}

	// 2. Check Bloom Filter - Prevent Cache Penetration
	// If it doesn't exist in Bloom Filter, it definitely doesn't exist in DB.
	exists, err := s.bloom.Exists(ctx, shortCode)
	if err == nil && !exists {
		// Record Bloom Filter miss (interception)
		metrics.RecordCacheHit("redirect-service", "bloom", "miss")
		// Optimization: Return Not Found immediately without checking Redis/DB
		return nil, fmt.Errorf("short URL not found (bloom)")
	}
	if err == nil && exists {
		// Record Bloom Filter hit (possible existence)
		metrics.RecordCacheHit("redirect-service", "bloom", "hit")
	}

	// 3. Use Singleflight to prevent Cache Breakdown (Thundering Herd)
	// Combine concurrent requests for the same shortCode into one execution
	result, err, _ := s.sf.Do(shortCode, func() (interface{}, error) {
		return s.resolveFromRemote(ctx, shortCode)
	})

	if err != nil {
		return nil, err
	}

	return result.(*domain.URL), nil
}

// resolveFromRemote handles the Redis L2 and Database fallback logic
func (s *RedirectStore) resolveFromRemote(ctx context.Context, shortCode string) (*domain.URL, error) {
	// 3.1 Check Redis Cache (L2)
	// Key format must match utils/cache/redis.go which url-shortener-svc uses
	// "urlshortener:url:<shortCode>"
	cacheKey := fmt.Sprintf("urlshortener:url:%s", shortCode)
	cached, err := s.redis.Get(ctx, cacheKey).Result()

	if err == nil {
		var entry CacheEntry
		if err := json.Unmarshal([]byte(cached), &entry); err == nil {
			// Check expiration
			if entry.ExpiresAt != nil && time.Now().After(*entry.ExpiresAt) {
				s.redis.Del(ctx, cacheKey)
				// Record L2 miss due to expiration
				metrics.RecordCacheHit("redirect-service", "l2", "miss_expired")
				return nil, fmt.Errorf("URL expired")
			}

			// Record L2 hit
			metrics.RecordCacheHit("redirect-service", "l2", "hit")

			// Populate Local Cache (L1) for future hot access
			s.localCache.Set(shortCode, entry, cache.DefaultExpiration)

			return s.entryToDomain(&entry), nil
		}
		// Record L2 miss if unmarshal fails (data format mismatch)
		metrics.RecordCacheHit("redirect-service", "l2", "miss")
	} else {
		// Record L2 miss (e.g. redis.Nil)
		metrics.RecordCacheHit("redirect-service", "l2", "miss")
	}

	// 3.2 Cache Miss - Query Database
	var res dbURLResult

	query := `
		SELECT id, short_code, long_url, user_id, created_at, expires_at, 
		       click_count, last_accessed, is_active
		FROM url_mappings 
		WHERE short_code = $1 AND is_active = true
	`

	err = s.db.GetContext(ctx, &res, query, shortCode)
	if err != nil {
		return nil, fmt.Errorf("short URL not found")
	}

	// Convert to Domain URL
	urlObj := s.dbToDomain(&res)

	// Check expiry
	if urlObj.ExpiresAt != nil && time.Now().After(*urlObj.ExpiresAt) {
		return nil, fmt.Errorf("URL expired")
	}

	// 3.3 Write-Back to Caches (L2 & L1)
	cacheEntry := CacheEntry{
		ShortCode:  urlObj.ShortCode,
		LongURL:    urlObj.LongURL,
		CreatedAt:  urlObj.CreatedAt,
		ExpiresAt:  urlObj.ExpiresAt,
		ClickCount: urlObj.ClickCount,
		IsActive:   urlObj.IsActive,
	}

	// Update Redis (L2)
	if entryJSON, err := json.Marshal(cacheEntry); err == nil {
		s.redis.Set(ctx, cacheKey, entryJSON, 24*time.Hour)
	}

	// Update Local Cache (L1)
	s.localCache.Set(shortCode, cacheEntry, cache.DefaultExpiration)

	return urlObj, nil
}

// Helpers
func (s *RedirectStore) entryToDomain(entry *CacheEntry) *domain.URL {
	return &domain.URL{
		ShortCode:  entry.ShortCode,
		LongURL:    entry.LongURL,
		CreatedAt:  entry.CreatedAt,
		ExpiresAt:  entry.ExpiresAt,
		ClickCount: entry.ClickCount,
		IsActive:   entry.IsActive,
		Metadata:   make(map[string]string),
	}
}

func (s *RedirectStore) dbToDomain(res *dbURLResult) *domain.URL {
	return &domain.URL{
		ID:           res.ID,
		ShortCode:    res.ShortCode,
		LongURL:      res.LongURL,
		UserID:       res.UserID,
		CreatedAt:    res.CreatedAt,
		ExpiresAt:    res.ExpiresAt,
		ClickCount:   res.ClickCount,
		LastAccessed: res.LastAccessed,
		IsActive:     res.IsActive,
		Metadata:     make(map[string]string),
	}
}

// IncrementClickCount atomically increments the click count
func (s *RedirectStore) IncrementClickCount(ctx context.Context, shortCode string) error {
	// 【性能优化】：在高 QPS 短码跳转期间，禁用 PostgreSQL 的同步 UPDATE
	// 多个并发请求同时 UPDATE 同一行数据会引发严重的行锁 (Row Lock) 竞争，导致连接池耗尽和上百毫秒的延迟。
	// 这里我们只更新 Redis，持久化和分析交由 NATS 发送给 Clickhouse。

	// 2. Increment in cache (for fast access)
	cacheKey := fmt.Sprintf("urlshortener:url:%s", shortCode)

	// Get current cache entry
	cached, err := s.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		var entry CacheEntry
		if json.Unmarshal([]byte(cached), &entry) == nil {
			// Increment count and update cache
			entry.ClickCount++
			if entryJSON, err := json.Marshal(entry); err == nil {
				s.redis.Set(ctx, cacheKey, entryJSON, 24*time.Hour)
			}
		}
	}

	// 3. Track click count in Redis counter (for analytics)
	counterKey := fmt.Sprintf("clicks:counter:%s", shortCode)
	s.redis.Incr(ctx, counterKey)
	s.redis.Expire(ctx, counterKey, 30*24*time.Hour) // 30 days retention

	return nil
}

// GetClickCount gets the current click count from cache or database
func (s *RedirectStore) GetClickCount(ctx context.Context, shortCode string) (int64, error) {
	// Try cache first
	cacheKey := fmt.Sprintf("urlshortener:url:%s", shortCode)
	cached, err := s.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		var entry CacheEntry
		if json.Unmarshal([]byte(cached), &entry) == nil {
			return entry.ClickCount, nil
		}
	}

	// Fallback to database
	var clickCount int64
	query := `SELECT click_count FROM url_mappings WHERE short_code = $1`
	err = s.db.GetContext(ctx, &clickCount, query, shortCode)
	if err != nil {
		return 0, fmt.Errorf("short URL not found")
	}

	return clickCount, nil
}

// PrewarmCache preloads popular URLs into cache
func (s *RedirectStore) PrewarmCache(ctx context.Context, shortCodes []string) error {
	if len(shortCodes) == 0 {
		return nil
	}

	// Batch fetch from database
	query := `
		SELECT short_code, long_url, created_at, expires_at, click_count, is_active
		FROM url_mappings 
		WHERE short_code = ANY($1) AND is_active = true
	`

	rows, err := s.db.QueryContext(ctx, query, shortCodes)
	if err != nil {
		return fmt.Errorf("failed to fetch URLs for prewarming: %w", err)
	}
	defer rows.Close()

	// Batch update cache
	pipe := s.redis.Pipeline()
	for rows.Next() {
		var entry CacheEntry
		err := rows.Scan(&entry.ShortCode, &entry.LongURL, &entry.CreatedAt,
			&entry.ExpiresAt, &entry.ClickCount, &entry.IsActive)
		if err == nil {
			if entryJSON, err := json.Marshal(entry); err == nil {
				cacheKey := fmt.Sprintf("urlshortener:url:%s", entry.ShortCode)
				pipe.Set(ctx, cacheKey, entryJSON, 24*time.Hour)
			}
		}
	}

	_, err = pipe.Exec(ctx)
	return err
}

// InvalidateCache removes a URL from cache (useful for updates/deletions)
func (s *RedirectStore) InvalidateCache(ctx context.Context, shortCode string) error {
	cacheKey := fmt.Sprintf("urlshortener:url:%s", shortCode)
	// Also remove from local cache
	s.localCache.Delete(shortCode)
	return s.redis.Del(ctx, cacheKey).Err()
}

// GetCacheStats returns cache performance metrics
func (s *RedirectStore) GetCacheStats(ctx context.Context) (map[string]interface{}, error) {
	info := s.redis.Info(ctx, "stats").Val()

	stats := map[string]interface{}{
		"cache_info": info,
		"timestamp":  time.Now(),
	}

	return stats, nil
}
