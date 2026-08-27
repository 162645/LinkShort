package store

import (
	"context"
	"time"

	"github.com/go-systems-lab/go-url-shortener/services/url-shortener-svc/domain"
	"github.com/go-systems-lab/go-url-shortener/utils/cache"
	"github.com/go-systems-lab/go-url-shortener/utils/database"
)

// URLStore 为 URL 操作提供了一个干净的接口
type URLStore struct {
	service *domain.URLService
}

// NewURLStore 创建一个新的 URL store 实例
func NewURLStore(db *database.PostgreSQL, cache *cache.Redis) *URLStore {
	service := domain.NewURLService(db, cache)
	return &URLStore{
		service: service,
	}
}

// ShortenURLRequest 代表存储层的 URL 缩短请求
type ShortenURLRequest struct {
	LongURL        string            `json:"long_url"`
	CustomAlias    string            `json:"custom_alias,omitempty"`
	ExpirationTime *time.Time        `json:"expiration_time,omitempty"`
	UserID         string            `json:"user_id"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// URLResponse 代表存储层的 URL 操作响应
type URLResponse struct {
	ID           int64             `json:"id"`
	ShortCode    string            `json:"short_code"`
	LongURL      string            `json:"long_url"`
	UserID       string            `json:"user_id"`
	CreatedAt    time.Time         `json:"created_at"`
	ExpiresAt    *time.Time        `json:"expires_at"`
	ClickCount   int64             `json:"click_count"`
	LastAccessed *time.Time        `json:"last_accessed"`
	IsActive     bool              `json:"is_active"`
	Metadata     map[string]string `json:"metadata"`
}

// GetUserURLsRequest 代表用户 URL 的分页请求
type GetUserURLsRequest struct {
	UserID    string `json:"user_id"`
	Page      int32  `json:"page"`
	PageSize  int32  `json:"page_size"`
	SortBy    string `json:"sort_by"`
	SortOrder string `json:"sort_order"`
}

// GetUserURLsResponse 代表用户 URL 的分页响应
type GetUserURLsResponse struct {
	URLs       []URLResponse `json:"urls"`
	TotalCount int32         `json:"total_count"`
	Page       int32         `json:"page"`
	PageSize   int32         `json:"page_size"`
	HasNext    bool          `json:"has_next"`
}

// UpdateURLRequest 代表存储层的 URL 更新请求
type UpdateURLRequest struct {
	ShortCode         string            `json:"short_code"`
	UserID            string            `json:"user_id"`
	NewLongURL        string            `json:"new_long_url,omitempty"`
	NewExpirationTime *time.Time        `json:"new_expiration_time,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

// ShortenURL 创建一个新的短链接 (已增加 context 支持)
func (s *URLStore) ShortenURL(ctx context.Context, req *ShortenURLRequest) (*URLResponse, error) {
	domainReq := &domain.CreateURLRequest{
		LongURL:        req.LongURL,
		CustomAlias:    req.CustomAlias,
		ExpirationTime: req.ExpirationTime,
		UserID:         req.UserID,
		Metadata:       req.Metadata,
	}

	// 透传 ctx 到领域层
	url, err := s.service.ShortenURL(ctx, domainReq)
	if err != nil {
		return nil, err
	}

	return s.domainToStoreURL(url), nil
}

// GetURL 通过短码检索 URL 信息 (修复：增加 context 参数)
func (s *URLStore) GetURL(ctx context.Context, shortCode, userID string) (*URLResponse, error) {
	// 透传 ctx 到领域层
	url, err := s.service.GetURL(ctx, shortCode, userID)
	if err != nil {
		return nil, err
	}

	return s.domainToStoreURL(url), nil
}

// GetUserURLs 分页检索用户的 URL (修复：增加 context 参数)
func (s *URLStore) GetUserURLs(ctx context.Context, req *GetUserURLsRequest) (*GetUserURLsResponse, error) {
	domainReq := &domain.GetUserURLsRequest{
		UserID:    req.UserID,
		Page:      req.Page,
		PageSize:  req.PageSize,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}

	// 透传 ctx 到领域层
	response, err := s.service.GetUserURLs(ctx, domainReq)
	if err != nil {
		return nil, err
	}

	urls := make([]URLResponse, len(response.URLs))
	for i, url := range response.URLs {
		urls[i] = *s.domainToStoreURL(&url)
	}

	return &GetUserURLsResponse{
		URLs:       urls,
		TotalCount: response.TotalCount,
		Page:       response.Page,
		PageSize:   response.PageSize,
		HasNext:    response.HasNext,
	}, nil
}

// UpdateURL 更新现有的 URL (修复：增加 context 参数)
func (s *URLStore) UpdateURL(ctx context.Context, req *UpdateURLRequest) (*URLResponse, error) {
	domainReq := &domain.UpdateURLRequest{
		ShortCode:         req.ShortCode,
		UserID:            req.UserID,
		NewLongURL:        req.NewLongURL,
		NewExpirationTime: req.NewExpirationTime,
		Metadata:          req.Metadata,
	}

	// 透传 ctx 到领域层
	url, err := s.service.UpdateURL(ctx, domainReq)
	if err != nil {
		return nil, err
	}

	return s.domainToStoreURL(url), nil
}

// DeleteURL 软删除一个 URL (修复：增加 context 参数)
func (s *URLStore) DeleteURL(ctx context.Context, shortCode, userID string) error {
	// 透传 ctx 到领域层
	return s.service.DeleteURL(ctx, shortCode, userID)
}

// 辅助函数：将领域层 URL 转换为存储层 URL
func (s *URLStore) domainToStoreURL(url *domain.URL) *URLResponse {
	if url == nil {
		return nil
	}
	return &URLResponse{
		ID:           url.ID,
		ShortCode:    url.ShortCode,
		LongURL:      url.LongURL,
		UserID:       url.UserID,
		CreatedAt:    url.CreatedAt,
		ExpiresAt:    url.ExpiresAt,
		ClickCount:   url.ClickCount,
		LastAccessed: url.LastAccessed,
		IsActive:     url.IsActive,
		Metadata:     url.Metadata,
	}
}
