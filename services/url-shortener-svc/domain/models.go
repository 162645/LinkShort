package domain

import (
	"errors"
	"time"
)

// URL 表示系统核心的 URL 实体（对应设计文档中的核心数据结构）
type URL struct {
	ID           int64             `json:"id" db:"id"`                       // 数据库自增主键
	ShortCode    string            `json:"short_code" db:"short_code"`       // 短码（如 abc123）
	LongURL      string            `json:"long_url" db:"long_url"`           // 原始长链接
	UserID       string            `json:"user_id" db:"user_id"`             // 创建者用户 ID
	CreatedAt    time.Time         `json:"created_at" db:"created_at"`       // 创建时间
	ExpiresAt    *time.Time        `json:"expires_at" db:"expires_at"`       // 过期时间（可为空，代表永久有效）
	ClickCount   int64             `json:"click_count" db:"click_count"`     // 点击量统计
	LastAccessed *time.Time        `json:"last_accessed" db:"last_accessed"` // 最后一次访问时间
	IsActive     bool              `json:"is_active" db:"is_active"`         // 是否激活状态（逻辑删除标识）
	Metadata     map[string]string `json:"metadata" db:"metadata"`           // 扩展元数据（如标签、分类等）
}

// CreateURLRequest 表示创建短链接时的业务逻辑请求参数
type CreateURLRequest struct {
	LongURL        string            `json:"long_url"`                  // 必填：目标长链接
	CustomAlias    string            `json:"custom_alias,omitempty"`    // 可选：自定义短码别名 说明：在字段后的反引号内容里，omitempty 是区分可选字段的关键标识：
	ExpirationTime *time.Time        `json:"expiration_time,omitempty"` // 可选：设置过期时间
	UserID         string            `json:"user_id"`                   // 必填：用户 ID
	Metadata       map[string]string `json:"metadata,omitempty"`        // 可选：附加信息
}

// UpdateURLRequest 表示更新 URL 时的业务逻辑请求参数
type UpdateURLRequest struct {
	ShortCode         string            `json:"short_code"`                    // 必填：要修改的短码
	UserID            string            `json:"user_id"`                       // 必填：操作人 ID（用于权限校验）
	NewLongURL        string            `json:"new_long_url,omitempty"`        // 可选：修改目标链接
	NewExpirationTime *time.Time        `json:"new_expiration_time,omitempty"` // 可选：修改过期时间
	Metadata          map[string]string `json:"metadata,omitempty"`            // 可选：更新元数据
}

// GetUserURLsRequest 表示用户查询自己链接列表时的分页与过滤参数
type GetUserURLsRequest struct {
	UserID    string `json:"user_id"`    // 用户 ID
	Page      int32  `json:"page"`       // 当前页码
	PageSize  int32  `json:"page_size"`  // 每页条数
	SortBy    string `json:"sort_by"`    // 排序字段（如 clicks, created_at）
	SortOrder string `json:"sort_order"` // 排序顺序（asc/desc）
}

// GetUserURLsResponse 表示带分页的用户链接列表响应数据
type GetUserURLsResponse struct {
	URLs       []URL `json:"urls"`        // 链接实体列表
	TotalCount int32 `json:"total_count"` // 总记录数
	Page       int32 `json:"page"`        // 当前返回的页码
	PageSize   int32 `json:"page_size"`   // 每页条数
	HasNext    bool  `json:"has_next"`    // 是否有下一页
}

// 业务领域特定的错误定义（来源于 HLD 高层设计文档）
var (
	ErrInvalidURL       = errors.New("invalid URL format")          // URL 格式错误
	ErrURLNotFound      = errors.New("URL not found")               // 链接不存在
	ErrURLExpired       = errors.New("URL has expired")             // 链接已过期
	ErrUnauthorized     = errors.New("unauthorized access to URL")  // 无权访问该链接
	ErrCustomAliasUsed  = errors.New("custom alias already exists") // 自定义别名已被占用
	ErrInvalidShortCode = errors.New("invalid short code format")   // 短码格式不合法
)

// IsExpired 检查当前 URL 是否已经超过有效期
func (u *URL) IsExpired() bool {
	if u.ExpiresAt == nil { // 如果没有设置过期时间，则永远不过期
		return false
	}
	return time.Now().After(*u.ExpiresAt) // 当前时间晚于过期时间则返回 true
}

// CanAccess 检查指定用户是否有权管理此 URL（权限业务规则）
func (u *URL) CanAccess(userID string) bool {
	return u.UserID == userID // 只有创建者可以修改或删除
}

// IsValidForRedirect 检查 URL 当前状态是否允许跳转（核心跳转业务规则）
func (u *URL) IsValidForRedirect() error {
	if !u.IsActive {
		return ErrURLNotFound // 如果已被禁用，返回不存在错误
	}
	if u.IsExpired() {
		return ErrURLExpired // 如果已过期，返回过期错误
	}
	return nil // 校验通过
}
