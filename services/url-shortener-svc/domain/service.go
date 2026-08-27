package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"time"

	"github.com/go-systems-lab/go-url-shortener/utils/bloom"
	"github.com/go-systems-lab/go-url-shortener/utils/cache"
	"github.com/go-systems-lab/go-url-shortener/utils/database"
	"github.com/go-systems-lab/go-url-shortener/utils/idgen"
	"github.com/go-systems-lab/go-url-shortener/utils/tracing"
)

// URLService 实现 HLD（高层设计）中的核心业务逻辑
type URLService struct {
	db    *database.PostgreSQL    // 数据库持久化层
	cache *cache.Redis            // Redis 缓存层
	idGen *idgen.Generator        // 分布式 ID 生成器
	bloom *bloom.RedisBloomFilter // 布隆过滤器
}

// NewURLService 创建一个新的服务实例
func NewURLService(db *database.PostgreSQL, redisCache *cache.Redis) *URLService {
	// Initialize ID Generator
	// Key: "url:id:counter" for global atomic counter
	gen := idgen.NewGenerator(redisCache.Client, "url:id:counter")

	// Seed the generator from Database to prevent collisions after restarts/flush
	// We get the current maximum ID from Postgres
	// NOTE: querying DB in constructor blocks startup, which is intended for safety.
	var maxID int64
	// Try to get max ID. If table is empty, this returns 0/NULL
	// Access the underlying sqlx.DB via the publicly exported DB field
	err := db.DB.Get(&maxID, "SELECT COALESCE(MAX(id), 0) FROM url_mappings")
	if err != nil {
		fmt.Printf("⚠️ Warning: Failed to get Max ID from DB: %v. Defaulting to 10,000,000,000.\n", err)
		maxID = 10000000000 // 10 Billion default start
	} else {
		fmt.Printf("✅ Found Max ID in DB: %d\n", maxID)
		if maxID < 10000000000 {
			maxID = 10000000000
		}
	}

	// Initialize counter if not exists. Add buffer (e.g. +1000) to be safe?
	// Redis INCR is atomic, so if we start from MaxID, next INCR gives MaxID+1.
	// We use SetNX inside, so it only sets if Redis key is missing.
	if err := gen.InitializeCounter(context.Background(), maxID); err != nil {
		fmt.Printf("❌ Failed to initialize ID counter: %v\n", err)
	}

	// Initialize Bloom Filter
	// Key: "url:bloom", Size: 100M bits (~12MB), Hashes: 5
	bf := bloom.NewRedisBloomFilter(redisCache.Client, "url:bloom", 100*1000*1000, 5)

	return &URLService{
		db:    db,
		cache: redisCache,
		idGen: gen,
		bloom: bf,
	}
}

// ShortenURL 实现核心短链接生成算法
func (s *URLService) ShortenURL(ctx context.Context, req *CreateURLRequest) (*URL, error) {
	// 0. 初始化追踪器并开启总逻辑 Span
	tracer := tracing.NewTracer("url-shortener-svc")
	// logicCtx 包含了当前 Span 信息，必须传给后续所有子操作以维持链路
	logicCtx, span := tracer.StartSpan(ctx, "Domain.ShortenURL")
	defer span.End()

	// 1. 验证长链接合法性（必须包含 http/https）
	if err := s.validateURL(req.LongURL); err != nil {
		tracing.RecordError(span, err)
		return nil, fmt.Errorf("URL 验证失败: %w", err)
	}

	// 2. 处理短码生成：如果有自定义别名则校验并使用，否则自动生成
	var shortCode string
	if req.CustomAlias != "" {
		if err := s.validateShortCode(req.CustomAlias); err != nil {
			tracing.RecordError(span, err)
			return nil, fmt.Errorf("自定义别名无效: %w", err)
		}

		// 【链路修复】：检查别名是否已被占用，使用 WithCtx 版本
		dbCtx, dbSpan := tracer.StartDatabaseSpan(logicCtx, "SELECT", "url_mappings_check")
		existing, _ := s.db.GetURLByShortCodeWithCtx(dbCtx, req.CustomAlias)
		dbSpan.End()

		if existing != nil {
			return nil, ErrCustomAliasUsed
		}
		shortCode = req.CustomAlias
	} else {
		// 【链路修复】：显式开启子 Span 并使用新 Context
		idCtx, idSpan := tracer.StartSpan(logicCtx, "IDGenerator.Generate")
		var err error
		// 必须传入 idCtx 确保 ID 生成过程出现在瀑布图中
		shortCode, err = s.idGen.GenerateID(idCtx)
		idSpan.End()

		if err != nil {
			tracing.RecordError(span, err)
			return nil, fmt.Errorf("生成短码失败: %w", err)
		}
	}

	// 3. 元数据序列化
	metadataJSON := "{}"
	if req.Metadata != nil && len(req.Metadata) > 0 {
		metadataBytes, err := json.Marshal(req.Metadata)
		if err != nil {
			tracing.RecordError(span, err)
			return nil, fmt.Errorf("元数据序列化失败: %w", err)
		}
		metadataJSON = string(metadataBytes)
	}

	// 4. 准备存入数据库的对象
	dbURL := &database.URLMapping{
		ShortCode: shortCode,
		LongURL:   req.LongURL,
		UserID:    req.UserID,
		IsActive:  true,
		Metadata:  metadataJSON,
	}

	// 处理过期时间
	if req.ExpirationTime != nil {
		dbURL.ExpiresAt.Valid = true
		dbURL.ExpiresAt.Time = *req.ExpirationTime
	}

	// 5. 【链路修复】：保存到数据库，调用带 Ctx 的版本
	dbCtx, dbSpan := tracer.StartDatabaseSpan(logicCtx, "INSERT", "url_mappings")
	if err := s.db.CreateURLWithCtx(dbCtx, dbURL); err != nil {
		tracing.RecordError(dbSpan, err)
		dbSpan.End()
		return nil, fmt.Errorf("保存数据失败: %w", err)
	}
	dbSpan.End()

	// 6. 添加到布隆过滤器 (异步操作，使用 context.Background 确保不随主请求取消而中断)
	go func(code string) {
		if err := s.bloom.Add(context.Background(), code); err != nil {
			fmt.Printf("警告: 布隆过滤器添加失败: %v\n", err)
		}
	}(shortCode)

	// 7. 【链路修复】：写入缓存，调用带 Ctx 的版本
	cacheCtx, cacheSpan := tracer.StartCacheSpan(logicCtx, "SET", "url:cache")

	urlData := map[string]interface{}{
		"short_code": shortCode,
		"long_url":   req.LongURL,
		"user_id":    req.UserID,
		"created_at": dbURL.CreatedAt.Format(time.RFC3339), // 转为 RFC3339 字符串以兼容 CacheEntry time.Time 类型
		"is_active":  true,
	}

	if req.ExpirationTime != nil {
		urlData["expires_at"] = req.ExpirationTime.Format(time.RFC3339)
	}

	ttl := time.Hour * 24
	if req.ExpirationTime != nil && req.ExpirationTime.Before(time.Now().Add(ttl)) {
		ttl = time.Until(*req.ExpirationTime)
	}

	// 调用我们在 redis.go 中定义的 WithCtx 方法
	if err := s.cache.SetJSONWithCtx(cacheCtx, cache.URLCacheKey(shortCode), urlData, ttl); err != nil {
		tracing.RecordError(cacheSpan, err)
		fmt.Printf("警告: 缓存写入失败: %v\n", err)
	}
	cacheSpan.End()

	tracing.RecordSuccess(span)
	return s.dbToDomainURL(dbURL), nil
}

// GetURL 获取 URL 信息（带缓存优先策略）
func (s *URLService) GetURL(ctx context.Context, shortCode, userID string) (*URL, error) {
	tracer := tracing.NewTracer("url-shortener-svc")
	logicCtx, span := tracer.StartSpan(ctx, "Domain.GetURL")
	defer span.End()

	// 1. 尝试从 Redis 获取
	cacheCtx, cacheSpan := tracer.StartCacheSpan(logicCtx, "GET", "url:cache")
	cacheKey := cache.URLCacheKey(shortCode)
	var cachedData map[string]interface{}
	found, err := s.cache.GetJSONWithCtx(cacheCtx, cacheKey, &cachedData)
	cacheSpan.End()

	if err == nil && found {
		url := s.cacheToURL(shortCode, cachedData)
		if url != nil {
			if userID != "" && !url.CanAccess(userID) {
				return nil, ErrUnauthorized
			}
			return url, nil
		}
	}

	// 2. 缓存未命中，回退到数据库查询
	dbCtx, dbSpan := tracer.StartDatabaseSpan(logicCtx, "SELECT", "url_mappings")
	dbURL, err := s.db.GetURLByShortCodeWithCtx(dbCtx, shortCode)
	dbSpan.End()

	if err != nil {
		return nil, ErrURLNotFound
	}

	url := s.dbToDomainURL(dbURL)

	// 3. 权限检查
	if userID != "" && !url.CanAccess(userID) {
		return nil, ErrUnauthorized
	}

	// 4. 将查到的结果回填至缓存，方便下次访问
	s.cacheURLWithCtx(logicCtx, url)

	return url, nil
}

// GetUserURLs 获取用户的 URL 列表并支持分页功能
func (s *URLService) GetUserURLs(ctx context.Context, req *GetUserURLsRequest) (*GetUserURLsResponse, error) {
	tracer := tracing.NewTracer("url-shortener-svc")
	logicCtx, span := tracer.StartSpan(ctx, "Domain.GetUserURLs")
	defer span.End()

	if req.PageSize <= 0 || req.PageSize > 100 {
		req.PageSize = 20
	}
	if req.Page <= 0 {
		req.Page = 1
	}

	offset := (req.Page - 1) * req.PageSize

	dbCtx, dbSpan := tracer.StartDatabaseSpan(logicCtx, "SELECT", "url_mappings_list")
	dbURLs, err := s.db.GetURLsByUserIDWithCtx(dbCtx, req.UserID, int(req.PageSize+1), int(offset))
	dbSpan.End()

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve user URLs: %w", err)
	}

	hasNext := len(dbURLs) > int(req.PageSize)
	if hasNext {
		dbURLs = dbURLs[:req.PageSize]
	}

	urls := make([]URL, len(dbURLs))
	for i, dbURL := range dbURLs {
		urls[i] = *s.dbToDomainURL(&dbURL)
	}

	return &GetUserURLsResponse{
		URLs:       urls,
		TotalCount: int32(len(urls)),
		Page:       req.Page,
		PageSize:   req.PageSize,
		HasNext:    hasNext,
	}, nil
}

// UpdateURL 更新现有的 URL，源自 HLD 高层设计
func (s *URLService) UpdateURL(ctx context.Context, req *UpdateURLRequest) (*URL, error) {
	tracer := tracing.NewTracer("url-shortener-svc")
	logicCtx, span := tracer.StartSpan(ctx, "Domain.UpdateURL")
	defer span.End()

	existingURL, err := s.GetURL(logicCtx, req.ShortCode, req.UserID)
	if err != nil {
		return nil, err
	}

	updated := false
	if req.NewLongURL != "" {
		if err := s.validateURL(req.NewLongURL); err != nil {
			return nil, fmt.Errorf("invalid new URL: %w", err)
		}
		existingURL.LongURL = req.NewLongURL
		updated = true
	}

	if req.NewExpirationTime != nil {
		existingURL.ExpiresAt = req.NewExpirationTime
		updated = true
	}

	if req.Metadata != nil {
		existingURL.Metadata = req.Metadata
		updated = true
	}

	if !updated {
		return existingURL, nil
	}

	// 【正式实现】：实现数据库更新操作
	dbCtx, dbSpan := tracer.StartDatabaseSpan(logicCtx, "UPDATE", "url_mappings")
	dbModel := &database.URLMapping{
		ShortCode: existingURL.ShortCode,
		LongURL:   existingURL.LongURL,
		UserID:    existingURL.UserID,
		IsActive:  existingURL.IsActive,
	}
	if existingURL.ExpiresAt != nil {
		dbModel.ExpiresAt.Valid = true
		dbModel.ExpiresAt.Time = *existingURL.ExpiresAt
	}
	metaBytes, _ := json.Marshal(existingURL.Metadata)
	dbModel.Metadata = string(metaBytes)

	if err := s.db.UpdateURLWithCtx(dbCtx, dbModel); err != nil {
		dbSpan.End()
		return nil, fmt.Errorf("failed to update database: %w", err)
	}
	dbSpan.End()

	// Invalidate cache
	cacheCtx, cacheSpan := tracer.StartCacheSpan(logicCtx, "DEL", "url:cache")
	cacheKey := cache.URLCacheKey(req.ShortCode)
	s.cache.DeleteWithCtx(cacheCtx, cacheKey)
	cacheSpan.End()

	return existingURL, nil
}

// DeleteURL 软删除 URL
func (s *URLService) DeleteURL(ctx context.Context, shortCode, userID string) error {
	tracer := tracing.NewTracer("url-shortener-svc")
	logicCtx, span := tracer.StartSpan(ctx, "Domain.DeleteURL")
	defer span.End()

	_, err := s.GetURL(logicCtx, shortCode, userID)
	if err != nil {
		return err
	}

	dbCtx, dbSpan := tracer.StartDatabaseSpan(logicCtx, "UPDATE", "url_mappings_delete")
	if err := s.db.DeleteURLWithCtx(dbCtx, shortCode, userID); err != nil {
		dbSpan.End()
		return fmt.Errorf("failed to delete URL: %w", err)
	}
	dbSpan.End()

	cacheCtx, cacheSpan := tracer.StartCacheSpan(logicCtx, "DEL", "url:cache")
	cacheKey := cache.URLCacheKey(shortCode)
	s.cache.DeleteWithCtx(cacheCtx, cacheKey)
	cacheSpan.End()

	return nil
}

// validateURL 验证长链接格式，源自 HLD 的业务规则
func (s *URLService) validateURL(longURL string) error {
	if longURL == "" {
		return ErrInvalidURL
	}
	parsedURL, err := url.Parse(longURL)
	if err != nil {
		return ErrInvalidURL
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return ErrInvalidURL
	}
	if parsedURL.Host == "" {
		return ErrInvalidURL
	}
	return nil
}

// validateShortCode 验证自定义短码格式
func (s *URLService) validateShortCode(shortCode string) error {
	if len(shortCode) < 3 || len(shortCode) > 10 {
		return ErrInvalidShortCode
	}
	matched, _ := regexp.MatchString("^[a-zA-Z0-9]+$", shortCode)
	if !matched {
		return ErrInvalidShortCode
	}
	return nil
}

// Helper functions for data conversion

func (s *URLService) dbToDomainURL(dbURL *database.URLMapping) *URL {
	var metadata map[string]string
	if dbURL.Metadata != "" && dbURL.Metadata != "{}" {
		json.Unmarshal([]byte(dbURL.Metadata), &metadata)
	}

	var expiresAt *time.Time
	if dbURL.ExpiresAt.Valid {
		expiresAt = &dbURL.ExpiresAt.Time
	}

	var lastAccessed *time.Time
	if dbURL.LastAccessed.Valid {
		lastAccessed = &dbURL.LastAccessed.Time
	}

	return &URL{
		ID:           dbURL.ID,
		ShortCode:    dbURL.ShortCode,
		LongURL:      dbURL.LongURL,
		UserID:       dbURL.UserID,
		CreatedAt:    dbURL.CreatedAt,
		ExpiresAt:    expiresAt,
		ClickCount:   dbURL.ClickCount,
		LastAccessed: lastAccessed,
		IsActive:     dbURL.IsActive,
		Metadata:     metadata,
	}
}

func (s *URLService) cacheToURL(shortCode string, data map[string]interface{}) *URL {
	longURL, ok := data["long_url"].(string)
	if !ok {
		return nil
	}

	userID, _ := data["user_id"].(string)
	isActive, _ := data["is_active"].(bool)

	url := &URL{
		ShortCode: shortCode,
		LongURL:   longURL,
		UserID:    userID,
		IsActive:  isActive,
	}

	// 兼容处理：尝试从 Unix 时间戳或 RFC3339 字符串恢复时间
	if createdAtUnix, ok := data["created_at"].(float64); ok {
		url.CreatedAt = time.Unix(int64(createdAtUnix), 0)
	} else if createdAtStr, ok := data["created_at"].(string); ok {
		t, _ := time.Parse(time.RFC3339, createdAtStr)
		url.CreatedAt = t
	}

	if expiresAtUnix, ok := data["expires_at"].(float64); ok {
		expiresAt := time.Unix(int64(expiresAtUnix), 0)
		url.ExpiresAt = &expiresAt
	} else if expiresAtStr, ok := data["expires_at"].(string); ok {
		t, _ := time.Parse(time.RFC3339, expiresAtStr)
		url.ExpiresAt = &t
	}

	return url
}

func (s *URLService) cacheURL(url *URL) {
	s.cacheURLWithCtx(context.Background(), url)
}

func (s *URLService) cacheURLWithCtx(ctx context.Context, url *URL) {
	cacheKey := cache.URLCacheKey(url.ShortCode)
	urlData := map[string]interface{}{
		"short_code": url.ShortCode,
		"long_url":   url.LongURL,
		"user_id":    url.UserID,
		"created_at": url.CreatedAt.Format(time.RFC3339),
		"is_active":  url.IsActive,
	}

	ttl := time.Hour * 24
	if url.ExpiresAt != nil {
		urlData["expires_at"] = url.ExpiresAt.Format(time.RFC3339)
		if url.ExpiresAt.Before(time.Now().Add(ttl)) {
			ttl = time.Until(*url.ExpiresAt)
		}
	}

	s.cache.SetJSONWithCtx(ctx, cacheKey, urlData, ttl)
}
