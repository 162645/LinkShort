package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

// PostgreSQL 连接管理器 - 包含 pgx 连接池和 sqlx 包装器
type PostgreSQL struct {
	Pool *pgxpool.Pool
	DB   *sqlx.DB
	ctx  context.Context
}

// URLMapping 代表数据库中的 url_mappings 表结构
type URLMapping struct {
	ID           int64        `db:"id" json:"id"`
	ShortCode    string       `db:"short_code" json:"short_code"`
	LongURL      string       `db:"long_url" json:"long_url"`
	UserID       string       `db:"user_id" json:"user_id"`
	CreatedAt    time.Time    `db:"created_at" json:"created_at"`
	ExpiresAt    sql.NullTime `db:"expires_at" json:"expires_at"`
	ClickCount   int64        `db:"click_count" json:"click_count"`
	LastAccessed sql.NullTime `db:"last_accessed" json:"last_accessed"`
	IsActive     bool         `db:"is_active" json:"is_active"`
	Metadata     string       `db:"metadata" json:"metadata"` // PostgreSQL JSONB 字段
}

// ClickEvent 代表数据库中的 click_events 分析表结构
type ClickEvent struct {
	ID          int64     `db:"id" json:"id"`
	ShortCode   string    `db:"short_code" json:"short_code"`
	Timestamp   time.Time `db:"timestamp" json:"timestamp"`
	IPAddress   string    `db:"ip_address" json:"ip_address"`
	UserAgent   string    `db:"user_agent" json:"user_agent"`
	Referrer    string    `db:"referrer" json:"referrer"`
	CountryCode string    `db:"country_code" json:"country_code"`
	CountryName string    `db:"country_name" json:"country_name"`
	City        string    `db:"city" json:"city"`
	DeviceType  string    `db:"device_type" json:"device_type"`
	Browser     string    `db:"browser" json:"browser"`
	OS          string    `db:"os" json:"os"`
	IsUnique    bool      `db:"is_unique" json:"is_unique"`
	SessionID   string    `db:"session_id" json:"session_id"`
}

// NewPostgreSQL 使用生产级配置初始化连接
func NewPostgreSQL() *PostgreSQL {
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:password@localhost:5432/url_shortener?sslmode=disable"
	}

	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Fatalf("无法解析数据库 URL: %v", err)
	}

	// 生产级连接池优化设置
	config.MaxConns = 100                      // 最大连接数
	config.MinConns = 10                       // 最小空闲连接数
	config.MaxConnLifetime = 5 * time.Minute   // 连接生命周期
	config.MaxConnIdleTime = 30 * time.Second  // 空闲连接超时
	config.HealthCheckPeriod = 1 * time.Minute // 健康检查周期

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		log.Fatalf("无法创建连接池: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("无法连接至 PostgreSQL: %v", err)
	}

	sqlxDB := sqlx.NewDb(stdlib.OpenDBFromPool(pool), "pgx")
	log.Println("✅ 成功通过 pgx 连接至 PostgreSQL")

	return &PostgreSQL{
		Pool: pool,
		DB:   sqlxDB,
		ctx:  ctx,
	}
}

// CreateTables 创建表结构
func (p *PostgreSQL) CreateTables() error {
	log.Println("🔧 正在创建数据库表...")

	urlMappingsSQL := `
    CREATE TABLE IF NOT EXISTS url_mappings (
       id BIGSERIAL PRIMARY KEY,
       short_code VARCHAR(10) UNIQUE NOT NULL,
       long_url TEXT NOT NULL,
       user_id VARCHAR(50),
       created_at TIMESTAMPTZ DEFAULT NOW(),
       expires_at TIMESTAMPTZ,
       click_count BIGINT DEFAULT 0,
       last_accessed TIMESTAMPTZ,
       is_active BOOLEAN DEFAULT true,
       metadata JSONB DEFAULT '{}'::jsonb
    );`

	if _, err := p.Pool.Exec(p.ctx, urlMappingsSQL); err != nil {
		return fmt.Errorf("创建 url_mappings 表失败: %v", err)
	}

	clickEventsSQL := `
    CREATE TABLE IF NOT EXISTS click_events (
       id BIGSERIAL PRIMARY KEY,
       short_code VARCHAR(10) NOT NULL,
       timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
       ip_address VARCHAR(45),
       user_agent TEXT,
       referrer TEXT,
       country_code VARCHAR(2),
       country_name VARCHAR(100),
       city VARCHAR(100),
       device_type VARCHAR(20),
       browser VARCHAR(50),
       os VARCHAR(50),
       is_unique BOOLEAN DEFAULT false,
       session_id VARCHAR(100)
    );`

	if _, err := p.Pool.Exec(p.ctx, clickEventsSQL); err != nil {
		return fmt.Errorf("创建 click_events 表失败: %v", err)
	}

	log.Println("✅ 数据库表创建成功")
	return nil
}

// CreateIndexes 创建高性能索引
func (p *PostgreSQL) CreateIndexes() error {
	log.Println("🔧 正在创建性能索引...")

	indexes := []string{
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_url_mappings_short_code ON url_mappings(short_code);",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_url_mappings_user_id ON url_mappings(user_id);",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_url_mappings_created_at ON url_mappings(created_at DESC);",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_url_mappings_expires_at ON url_mappings(expires_at) WHERE expires_at IS NOT NULL;",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_url_mappings_user_created ON url_mappings(user_id, created_at DESC);",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_url_mappings_active ON url_mappings(is_active) WHERE is_active = true;",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_click_events_short_code ON click_events(short_code);",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_click_events_timestamp ON click_events(timestamp DESC);",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_click_events_country_code ON click_events(country_code);",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_click_events_device_type ON click_events(device_type);",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_click_events_shortcode_time ON click_events(short_code, timestamp DESC);",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_click_events_country_time ON click_events(country_code, timestamp DESC);",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_click_events_device_time ON click_events(device_type, timestamp DESC);",
	}

	for _, indexSQL := range indexes {
		if _, err := p.Pool.Exec(p.ctx, indexSQL); err != nil {
			log.Printf("警告: 索引创建失败: %v", err)
		}
	}

	log.Println("✅ 性能索引创建成功")
	return nil
}

// AutoMigrate 自动迁移
func (p *PostgreSQL) AutoMigrate() error {
	if err := p.CreateTables(); err != nil {
		return err
	}
	return p.CreateIndexes()
}

// CreateURL 插入新映射
func (p *PostgreSQL) CreateURL(url *URLMapping) error {
	return p.CreateURLWithCtx(p.ctx, url)
}

func (p *PostgreSQL) CreateURLWithCtx(ctx context.Context, url *URLMapping) error {
	query := `
       INSERT INTO url_mappings (short_code, long_url, user_id, expires_at, metadata)
       VALUES ($1, $2, $3, $4, $5)
       RETURNING id, created_at`

	// 明确地把 expiresAt 设为 nil 默认值，避免隐式零值歧义
	var expiresAt interface{} = nil
	if url.ExpiresAt.Valid {
		expiresAt = url.ExpiresAt.Time
	}

	// 使用 p.Pool.QueryRow(ctx, ...) 保证使用外部传入的 ctx（携带 Trace）
	return p.Pool.QueryRow(ctx, query,
		url.ShortCode, url.LongURL, url.UserID, expiresAt, url.Metadata,
	).Scan(&url.ID, &url.CreatedAt)
}

// UpdateURLWithCtx 【关键修复：此前缺失的接口】
func (p *PostgreSQL) UpdateURLWithCtx(ctx context.Context, url *URLMapping) error {
	query := `
		UPDATE url_mappings 
		SET long_url = $1, expires_at = $2, metadata = $3, is_active = $4 
		WHERE short_code = $5 AND user_id = $6`

	var expiresAt interface{}
	if url.ExpiresAt.Valid {
		expiresAt = url.ExpiresAt.Time
	}

	result, err := p.Pool.Exec(ctx, query,
		url.LongURL, expiresAt, url.Metadata, url.IsActive,
		url.ShortCode, url.UserID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("未找到记录或权限不足")
	}
	return nil
}

// GetURLByShortCode 获取映射
func (p *PostgreSQL) GetURLByShortCode(shortCode string) (*URLMapping, error) {
	return p.GetURLByShortCodeWithCtx(p.ctx, shortCode)
}

func (p *PostgreSQL) GetURLByShortCodeWithCtx(ctx context.Context, shortCode string) (*URLMapping, error) {
	var url URLMapping
	query := `
       SELECT id, short_code, long_url, user_id, created_at, expires_at,
              click_count, last_accessed, is_active, metadata
       FROM url_mappings 
       WHERE short_code = $1 AND is_active = true`

	err := p.DB.GetContext(ctx, &url, query, shortCode)
	if err != nil {
		return nil, err
	}
	return &url, nil
}

// GetURLsByUserID 分页获取
func (p *PostgreSQL) GetURLsByUserID(userID string, limit, offset int) ([]URLMapping, error) {
	return p.GetURLsByUserIDWithCtx(p.ctx, userID, limit, offset)
}

func (p *PostgreSQL) GetURLsByUserIDWithCtx(ctx context.Context, userID string, limit, offset int) ([]URLMapping, error) {
	var urls []URLMapping
	query := `
       SELECT id, short_code, long_url, user_id, created_at, expires_at,
              click_count, last_accessed, is_active, metadata
       FROM url_mappings 
       WHERE user_id = $1 AND is_active = true
       ORDER BY created_at DESC
       LIMIT $2 OFFSET $3`

	err := p.DB.SelectContext(ctx, &urls, query, userID, limit, offset)
	return urls, err
}

// UpdateClickCount 更新点击数
func (p *PostgreSQL) UpdateClickCount(shortCode string) error {
	return p.UpdateClickCountWithCtx(p.ctx, shortCode)
}

func (p *PostgreSQL) UpdateClickCountWithCtx(ctx context.Context, shortCode string) error {
	query := `
       UPDATE url_mappings 
       SET click_count = click_count + 1, last_accessed = NOW()
       WHERE short_code = $1`

	_, err := p.Pool.Exec(ctx, query, shortCode)
	return err
}

// DeleteURL 软删除
func (p *PostgreSQL) DeleteURL(shortCode, userID string) error {
	return p.DeleteURLWithCtx(p.ctx, shortCode, userID)
}

func (p *PostgreSQL) DeleteURLWithCtx(ctx context.Context, shortCode, userID string) error {
	query := `
       UPDATE url_mappings 
       SET is_active = false 
       WHERE short_code = $1 AND user_id = $2`

	result, err := p.Pool.Exec(ctx, query, shortCode, userID)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("未找到 URL 或权限不足")
	}
	return nil
}

// CreateClickEvent 记录点击事件
func (p *PostgreSQL) CreateClickEvent(event *ClickEvent) error {
	return p.CreateClickEventWithCtx(p.ctx, event)
}

func (p *PostgreSQL) CreateClickEventWithCtx(ctx context.Context, event *ClickEvent) error {
	query := `
       INSERT INTO click_events (
          short_code, timestamp, ip_address, user_agent, referrer,
          country_code, country_name, city, device_type, browser,
          os, is_unique, session_id
       ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
       RETURNING id`

	return p.Pool.QueryRow(ctx, query,
		event.ShortCode, event.Timestamp, event.IPAddress, event.UserAgent, event.Referrer,
		event.CountryCode, event.CountryName, event.City, event.DeviceType, event.Browser,
		event.OS, event.IsUnique, event.SessionID,
	).Scan(&event.ID)
}

// GetClickEventsByShortCode 获取点击记录
func (p *PostgreSQL) GetClickEventsByShortCode(shortCode string, limit int) ([]ClickEvent, error) {
	return p.GetClickEventsByShortCodeWithCtx(p.ctx, shortCode, limit)
}

func (p *PostgreSQL) GetClickEventsByShortCodeWithCtx(ctx context.Context, shortCode string, limit int) ([]ClickEvent, error) {
	var events []ClickEvent
	query := `
       SELECT id, short_code, timestamp, ip_address, user_agent, referrer,
              country_code, country_name, city, device_type, browser,
              os, is_unique, session_id
       FROM click_events 
       WHERE short_code = $1
       ORDER BY timestamp DESC
       LIMIT $2`

	err := p.DB.SelectContext(ctx, &events, query, shortCode, limit)
	return events, err
}

// HealthCheck 健康检查
func (p *PostgreSQL) HealthCheck() error {
	return p.Pool.Ping(p.ctx)
}

// GetStats 获取连接池统计信息
func (p *PostgreSQL) GetStats() map[string]interface{} {
	stats := p.Pool.Stat()
	return map[string]interface{}{
		"total_conns":        stats.TotalConns(),
		"acquired_conns":     stats.AcquiredConns(),
		"idle_conns":         stats.IdleConns(),
		"max_conns":          stats.MaxConns(),
		"constructing_conns": stats.ConstructingConns(),
	}
}

// Close 关闭连接
func (p *PostgreSQL) Close() {
	if p.Pool != nil {
		p.Pool.Close()
	}
	if p.DB != nil {
		p.DB.Close()
	}
}
