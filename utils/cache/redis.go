package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9" // 必须引入
	"github.com/redis/go-redis/v9"
)

// Redis 缓存管理器 - 生产级配置，目标实现 95% 的缓存命中率
type Redis struct {
	Client *redis.Client
	ctx    context.Context
}

// CacheItem 代表带有元数据的缓存项
type CacheItem struct {
	Value     string                 `json:"value"`
	CreatedAt time.Time              `json:"created_at"`
	TTL       int64                  `json:"ttl"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// NewRedis 创建并初始化生产环境优化的 Redis 客户端
func NewRedis() *Redis {
	// 从环境变量获取 Redis URL
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		// 默认开发环境配置
		redisURL = "redis://:redispassword@localhost:6379/0"
	}

	// 解析 Redis URL
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("解析 Redis URL 失败: %v", err)
	}

	// 生产环境优化的 Redis 配置参数
	opt.MaxRetries = 3
	opt.MinRetryBackoff = 8 * time.Millisecond
	opt.MaxRetryBackoff = 512 * time.Millisecond
	opt.PoolSize = 100    // 高并发连接池大小
	opt.MinIdleConns = 25 // 最小空闲连接数
	opt.PoolTimeout = 30 * time.Second

	// 创建客户端
	client := redis.NewClient(opt)

	// 【关键新增】：开启 Redis 的自动追踪插件
	if err := redisotel.InstrumentTracing(client); err != nil {
		log.Printf("⚠️ 开启 Redis Tracing 失败: %v", err)
	}

	// 测试连接
	ctx := context.Background()
	_, err = client.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("连接 Redis 失败: %v", err)
	}

	log.Println("✅ 成功连接至 Redis")

	return &Redis{
		Client: client,
		ctx:    ctx,
	}
}

// --- 基础操作函数（保持原签名以兼容旧代码，内部调用 WithCtx 版本） ---

func (r *Redis) Set(key string, value interface{}, ttl time.Duration) error {
	return r.SetWithCtx(r.ctx, key, value, ttl)
}

func (r *Redis) Get(key string) (string, bool) {
	return r.GetWithCtx(r.ctx, key)
}

func (r *Redis) GetJSON(key string, dest interface{}) (bool, error) {
	return r.GetJSONWithCtx(r.ctx, key, dest)
}

func (r *Redis) SetJSON(key string, value interface{}, ttl time.Duration) error {
	return r.SetJSONWithCtx(r.ctx, key, value, ttl)
}

func (r *Redis) Delete(key string) error {
	return r.DeleteWithCtx(r.ctx, key)
}

func (r *Redis) Exists(key string) (bool, error) {
	return r.ExistsWithCtx(r.ctx, key)
}

func (r *Redis) Increment(key string) (int64, error) {
	return r.IncrementWithCtx(r.ctx, key)
}

func (r *Redis) IncrementBy(key string, value int64) (int64, error) {
	return r.IncrementByWithCtx(r.ctx, key, value)
}

// --- 核心修改：新增支持 Tracing 的 WithCtx 版本 ---

// SetWithCtx 将值存入 Redis 并支持上下文追踪
func (r *Redis) SetWithCtx(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("序列化失败: %v", err)
	}

	// 使用传入的 ctx 传导追踪信息
	err = r.Client.Set(ctx, key, data, ttl).Err()
	if err != nil {
		return fmt.Errorf("设置缓存失败: %v", err)
	}

	return nil
}

// GetWithCtx 从 Redis 获取值并支持上下文追踪
func (r *Redis) GetWithCtx(ctx context.Context, key string) (string, bool) {
	result, err := r.Client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", false // 键不存在
	} else if err != nil {
		log.Printf("Redis GET 错误 (Key: %s): %v", key, err)
		return "", false
	}

	return result, true
}

// GetJSONWithCtx 获取并反序列化 JSON 数据，支持上下文追踪
func (r *Redis) GetJSONWithCtx(ctx context.Context, key string, dest interface{}) (bool, error) {
	result, err := r.Client.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("redis 错误: %v", err)
	}

	err = json.Unmarshal([]byte(result), dest)
	if err != nil {
		return false, fmt.Errorf("反序列化 JSON 失败: %v", err)
	}

	return true, nil
}

// SetJSONWithCtx 存储 JSON 数据，支持上下文追踪
func (r *Redis) SetJSONWithCtx(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("序列化 JSON 失败: %v", err)
	}

	return r.Client.Set(ctx, key, data, ttl).Err()
}

// DeleteWithCtx 删除键，支持上下文追踪
func (r *Redis) DeleteWithCtx(ctx context.Context, key string) error {
	return r.Client.Del(ctx, key).Err()
}

// ExistsWithCtx 检查键是否存在，支持上下文追踪
func (r *Redis) ExistsWithCtx(ctx context.Context, key string) (bool, error) {
	result, err := r.Client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

// IncrementWithCtx 计数器递增，支持上下文追踪
func (r *Redis) IncrementWithCtx(ctx context.Context, key string) (int64, error) {
	return r.Client.Incr(ctx, key).Result()
}

// IncrementByWithCtx 指定数值递增，支持上下文追踪
func (r *Redis) IncrementByWithCtx(ctx context.Context, key string, value int64) (int64, error) {
	return r.Client.IncrBy(ctx, key, value).Result()
}

// SetTTL 设置过期时间，支持追踪
func (r *Redis) SetTTL(key string, ttl time.Duration) error {
	return r.Client.Expire(r.ctx, key, ttl).Err()
}

// GetTTL 获取过期时间
func (r *Redis) GetTTL(key string) (time.Duration, error) {
	return r.Client.TTL(r.ctx, key).Result()
}

// MGet 批量获取
func (r *Redis) MGet(keys ...string) ([]interface{}, error) {
	return r.Client.MGet(r.ctx, keys...).Result()
}

// MSet 批量设置
func (r *Redis) MSet(pairs ...interface{}) error {
	return r.Client.MSet(r.ctx, pairs...).Err()
}

// Pipeline 创建管道
func (r *Redis) Pipeline() redis.Pipeliner {
	return r.Client.Pipeline()
}

// PrewarmCache 预热缓存
func (r *Redis) PrewarmCache(urlMappings map[string]string, ttl time.Duration) error {
	pipe := r.Client.Pipeline()

	for shortCode, longURL := range urlMappings {
		pipe.Set(r.ctx, shortCode, longURL, ttl)
	}

	_, err := pipe.Exec(r.ctx)
	if err != nil {
		return fmt.Errorf("预热缓存失败: %v", err)
	}

	log.Printf("✅ 成功预热 %d 条映射数据", len(urlMappings))
	return nil
}

// GetCacheStats 获取 Redis 运行统计状态
func (r *Redis) GetCacheStats() map[string]interface{} {
	info, err := r.Client.Info(r.ctx, "stats", "memory").Result()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	poolStats := r.Client.PoolStats()

	return map[string]interface{}{
		"redis_info":  info,
		"hits":        poolStats.Hits,
		"misses":      poolStats.Misses,
		"timeouts":    poolStats.Timeouts,
		"total_conns": poolStats.TotalConns,
		"idle_conns":  poolStats.IdleConns,
		"stale_conns": poolStats.StaleConns,
	}
}

// HealthCheck 健康检查
func (r *Redis) HealthCheck() error {
	_, err := r.Client.Ping(r.ctx).Result()
	return err
}

// FlushDB 清空数据库（非生产环境可用）
func (r *Redis) FlushDB() error {
	if os.Getenv("GO_ENV") == "production" {
		return fmt.Errorf("禁止在生产环境执行 FlushDB")
	}
	return r.Client.FlushDB(r.ctx).Err()
}

// Close 关闭 Redis 连接
func (r *Redis) Close() error {
	return r.Client.Close()
}

// --- 键生成工具函数 ---

func CacheKey(prefix, identifier string) string {
	return fmt.Sprintf("urlshortener:%s:%s", prefix, identifier)
}

func URLCacheKey(shortCode string) string {
	return CacheKey("url", shortCode)
}

func AnalyticsCacheKey(shortCode, metric string) string {
	return CacheKey("analytics", fmt.Sprintf("%s:%s", shortCode, metric))
}

func UserCacheKey(userID, dataType string) string {
	return CacheKey("user", fmt.Sprintf("%s:%s", userID, dataType))
}
