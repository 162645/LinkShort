package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-systems-lab/go-url-shortener/utils/tracing"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"go-micro.dev/v5"

	analyticspb "github.com/go-systems-lab/go-url-shortener/proto/analytics"
	redirectpb "github.com/go-systems-lab/go-url-shortener/proto/redirect"
	pb "github.com/go-systems-lab/go-url-shortener/proto/url"
)

// URLHandler 结构体：持有多个微服务的客户端句柄，用于发起 RPC 调用
type URLHandler struct {
	client          pb.URLShortenerService       // URL 缩短服务客户端
	redirectClient  redirectpb.RedirectService   // 重定向服务客户端
	analyticsClient analyticspb.AnalyticsService // 统计分析服务客户端
	log             *logrus.Logger               // 日志记录器
}

// NewURLHandler 构造函数：初始化并连接各个微服务
func NewURLHandler(service micro.Service) *URLHandler {
	// 绑定具体的微服务名称（需与 docker-compose 中的服务名对应）
	client := pb.NewURLShortenerService("url.shortener.service", service.Client())
	redirectClient := redirectpb.NewRedirectService("redirect.service", service.Client())
	analyticsClient := analyticspb.NewAnalyticsService("url.shortener.analytics", service.Client())
	return &URLHandler{
		client:          client,
		redirectClient:  redirectClient,
		analyticsClient: analyticsClient,
		log:             logrus.New(),
	}
}

// ShortenURLRequest 定义了用户创建短链接时需要提交的 JSON 字段
type ShortenURLRequest struct {
	LongURL        string            `json:"long_url" binding:"required" example:"https://www.google.com"`
	CustomAlias    string            `json:"custom_alias,omitempty" example:"google"`
	ExpirationTime *int64            `json:"expiration_time,omitempty" example:"1735689600"`
	UserID         string            `json:"user_id" binding:"required" example:"user123"`
	Metadata       map[string]string `json:"metadata,omitempty" example:"campaign:social,source:twitter"`
}

// ShortenURLResponse 定义了创建成功后返回给用户的 JSON 数据
type ShortenURLResponse struct {
	ShortCode string `json:"short_code" example:"abc123"`
	ShortURL  string `json:"short_url" example:"https://short.ly/abc123"`
	LongURL   string `json:"long_url" example:"https://www.google.com"`
	CreatedAt int64  `json:"created_at" example:"1672531200"`
	ExpiresAt *int64 `json:"expires_at,omitempty" example:"1735689600"`
	UserID    string `json:"user_id" example:"user123"`
}

// ErrorResponse 统一错误返回格式
type ErrorResponse struct {
	Error string `json:"error" example:"Invalid request body"`
}

// ShortenURL 处理 POST /api/v1/shorten 接口
//
//	 @Summary      创建短链接
//	 @Description   根据长链接生成短链接，支持自定义别名和过期时间设置
//	 @Tags        链接管理
//	 @Accept          json
//	 @Produce      json
//	 @Param       request    body      ShortenURLRequest  true   "短链接生成请求体"
//		@Success		201		{object}	ShortenURLResponse	"Successfully created short URL"
//		@Failure		400		{object}	ErrorResponse		"Invalid request body"
//		@Failure		500		{object}	ErrorResponse		"Internal server error"
//		@Router			/shorten [post]
//
// ShortenURL 处理 POST /api/v1/shorten
func (h *URLHandler) ShortenURL(c *gin.Context) {
	var req ShortenURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.WithError(err).Error("请求体格式错误")
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}

	rpcReq := &pb.ShortenRequest{
		LongUrl:     req.LongURL,
		CustomAlias: req.CustomAlias,
		UserId:      req.UserID,
		Metadata:    req.Metadata,
	}

	if req.ExpirationTime != nil {
		rpcReq.ExpirationTime = *req.ExpirationTime
	}

	// 获取带有 Trace 信息的 Context 并设置超时
	rpcCtx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	rsp, err := h.client.ShortenURL(rpcCtx, rpcReq)
	if err != nil {
		h.log.WithError(err).Error("调用后台 RPC 服务失败")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to shorten URL"})
		return
	}

	c.JSON(http.StatusCreated, ShortenURLResponse{
		ShortCode: rsp.ShortCode,
		ShortURL:  rsp.ShortUrl,
		LongURL:   rsp.LongUrl,
		CreatedAt: rsp.CreatedAt,
		UserID:    rsp.UserId,
		ExpiresAt: func() *int64 {
			if rsp.ExpiresAt > 0 {
				return &rsp.ExpiresAt
			}
			return nil
		}(),
	})
}

// URLInfoResponse 结构体：定义了向前端返回的短链接详细信息格式
type URLInfoResponse struct {
	ShortCode  string            `json:"short_code" example:"abc123"`
	ShortURL   string            `json:"short_url" example:"https://short.ly/abc123"`
	LongURL    string            `json:"long_url" example:"https://www.google.com"`
	UserID     string            `json:"user_id" example:"user123"`
	CreatedAt  int64             `json:"created_at" example:"1672531200"`
	ExpiresAt  *int64            `json:"expires_at,omitempty" example:"1735689600"`
	ClickCount int64             `json:"click_count" example:"42"`
	IsActive   bool              `json:"is_active" example:"true"`
	Metadata   map[string]string `json:"metadata,omitempty" example:"campaign:social,source:twitter"`
}

// GetURLInfo 处理 GET /api/v1/urls/:shortCode 接口，当管理员或用户想要查看某个短链接背后的具体信息（比如它指向哪里、什么时候过期、被点击了多少次）时，就会调用这个接口。
// 以下是 Swagger 文档注解，用于自动生成 API 文档
//
//	 @Summary      获取 URL 详情
//	 @Description   根据短码检索短链接的详细元数据
//	 @Tags        URL 管理
//	 @Accept          json
//	 @Produce      json
//	 @Param       shortCode  path      string          true   "短码标识符"    example(abc123)
//	 @Param       user_id       query     string          true   "用户 ID"           example(user123)
//		@Success		200			{object}	URLInfoResponse		"URL information retrieved successfully"
//		@Failure		400			{object}	ErrorResponse		"Missing user_id parameter"
//		@Failure		404			{object}	ErrorResponse		"URL not found"
//		@Router			/urls/{shortCode} [get]
//
// GetURLInfo 处理 GET /api/v1/urls/:shortCode
func (h *URLHandler) GetURLInfo(c *gin.Context) {
	shortCode := c.Param("shortCode")
	userID := c.Query("user_id")

	if userID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "user_id is required"})
		return
	}

	rpcCtx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Inject trace into go-micro metadata
	rpcCtx = tracing.InjectTraceToGoMicroContext(rpcCtx)

	rsp, err := h.client.GetURLInfo(rpcCtx, &pb.GetURLRequest{
		ShortCode: shortCode,
		UserId:    userID,
	})
	if err != nil {
		h.log.WithError(err).Error("调用 RPC 服务失败")
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "URL not found"})
		return
	}

	c.JSON(http.StatusOK, URLInfoResponse{
		ShortCode:  rsp.ShortCode,
		ShortURL:   rsp.ShortUrl,
		LongURL:    rsp.LongUrl,
		UserID:     rsp.UserId,
		CreatedAt:  rsp.CreatedAt,
		ClickCount: rsp.ClickCount,
		IsActive:   rsp.IsActive,
		Metadata:   rsp.Metadata,
		ExpiresAt: func() *int64 {
			if rsp.ExpiresAt > 0 {
				return &rsp.ExpiresAt
			}
			return nil
		}(),
	})
}

// UserURLsResponse represents the response for user URLs list
type UserURLsResponse struct {
	URLs       []URLInfoResponse `json:"urls"`
	TotalCount int32             `json:"total_count" example:"100"`
	Page       int32             `json:"page" example:"1"`
	PageSize   int32             `json:"page_size" example:"20"`
	HasNext    bool              `json:"has_next" example:"true"`
}

// GetUserURLs handles GET /api/v1/users/:userID/urls  ，，“获取指定用户名下的所有短链接列表”。它支持分页和排序，是用户后台管理界面的核心 API。
//
//	 @Summary      获取用户的 URL 列表
//	 @Description   检索属于特定用户的短链接分页列表
//	 @Tags        用户管理
//	 @Accept          json
//	 @Produce      json
//	 @Param       userID    path      string          true   "用户唯一标识 ID"   example(user123)
//	 @Param       page      query     int                false  "页码（默认 1）"     example(1)
//	 @Param       page_size  query     int                false  "每页显示条数"      example(20)
//	 @Param       sort_by       query     string          false  "排序字段（如 created_at, click_count）"  example(created_at)
//	 @Param       sort_order query     string          false  "排序顺序（asc 升序, desc 降序）"  example(desc)
//		@Success		200			{object}	UserURLsResponse	"User URLs retrieved successfully"
//		@Failure		500			{object}	ErrorResponse		"Failed to get user URLs"
//		@Router			/users/{userID}/urls [get]
func (h *URLHandler) GetUserURLs(c *gin.Context) {
	userID := c.Param("userID")

	page, _ := strconv.ParseInt(c.DefaultQuery("page", "1"), 10, 32)
	pageSize, _ := strconv.ParseInt(c.DefaultQuery("page_size", "20"), 10, 32)
	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")

	h.log.WithFields(logrus.Fields{
		"user_id":   userID,
		"page":      page,
		"page_size": pageSize,
	}).Info("Processing GetUserURLs REST request")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Inject trace
	ctx = tracing.InjectTraceToGoMicroContext(ctx)

	rsp, err := h.client.GetUserURLs(ctx, &pb.GetUserURLsRequest{
		UserId:    userID,
		Page:      int32(page),
		PageSize:  int32(pageSize),
		SortBy:    sortBy,
		SortOrder: sortOrder,
	})
	if err != nil {
		h.log.WithError(err).Error("Failed to call RPC service")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to get user URLs"})
		return
	}

	urls := make([]URLInfoResponse, len(rsp.Urls))
	for i, url := range rsp.Urls {
		urlData := URLInfoResponse{
			ShortCode:  url.ShortCode,
			ShortURL:   url.ShortUrl,
			LongURL:    url.LongUrl,
			UserID:     url.UserId,
			CreatedAt:  url.CreatedAt,
			ClickCount: url.ClickCount,
			IsActive:   url.IsActive,
			Metadata:   url.Metadata,
		}
		if url.ExpiresAt > 0 {
			urlData.ExpiresAt = &url.ExpiresAt
		}
		urls[i] = urlData
	}

	response := UserURLsResponse{
		URLs:       urls,
		TotalCount: rsp.TotalCount,
		Page:       rsp.Page,
		PageSize:   rsp.PageSize,
		HasNext:    rsp.HasNext,
	}

	c.JSON(http.StatusOK, response)
}

// DeleteResponse 表示删除操作的响应结果
type DeleteResponse struct {
	// 提示消息：说明操作的结果，例如 "URL 已成功删除" 或具体的失败原因
	Message string `json:"message" example:"URL deleted successfully"`
}

// DeleteURL 处理 DELETE /api/v1/urls/:shortCode 接口
//
//	 @Summary      删除短链接
//	 @Description   删除属于已验证用户的特定短链接
//	 @Tags        URL 管理
//	 @Accept          json
//	 @Produce      json
//	 @Param       shortCode  path      string       true   "短码标识符"    example(abc123)
//	 @Param       user_id       query     string       true   "用户 ID"           example(user123)
//		@Success		200			{object}	DeleteResponse	"URL deleted successfully"
//		@Failure		400			{object}	ErrorResponse	"Missing user_id parameter"
//		@Failure		500			{object}	ErrorResponse	"Failed to delete URL"
//		@Router			/urls/{shortCode} [delete]
func (h *URLHandler) DeleteURL(c *gin.Context) {
	shortCode := c.Param("shortCode")
	userID := c.Query("user_id")

	if userID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "user_id is required"})
		return
	}

	h.log.WithFields(logrus.Fields{
		"short_code": shortCode,
		"user_id":    userID,
	}).Info("Processing DeleteURL REST request")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Inject trace
	ctx = tracing.InjectTraceToGoMicroContext(ctx)

	rsp, err := h.client.DeleteURL(ctx, &pb.DeleteURLRequest{
		ShortCode: shortCode,
		UserId:    userID,
	})
	if err != nil {
		h.log.WithError(err).Error("Failed to call RPC service")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete URL"})
		return
	}

	if !rsp.Success {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: rsp.Message})
		return
	}

	c.JSON(http.StatusOK, DeleteResponse{Message: rsp.Message})
}

// RedirectResponse 表示带有分析数据的重定向操作响应结果
type RedirectResponse struct {
	LongURL    string `json:"long_url" example:"https://www.google.com"`
	ShortCode  string `json:"short_code" example:"abc123"`
	SessionID  string `json:"session_id" example:"sess_1234567890abcdef"`
	ClickCount int64  `json:"click_count" example:"43"`
	Timestamp  int64  `json:"timestamp" example:"1672531200"`
}

// RedirectURL 处理 GET /:shortCode 接口，用于 URL 重定向，也就是当用户在浏览器输入短链接（如 http://t.cn/abc123）时，这个函数负责把用户送往真正的目的地。
//
//	 @Summary      重定向至原始 URL
//	 @Description   解析短码，跳转至原始长链接并记录点击追踪数据
//	 @Tags        重定向
//	 @Accept          json
//	 @Produce      json
//	 @Param       shortCode  path   string true   "短码标识符"    example(abc123)
//		@Success		302			"Redirect to original URL"
//		@Success		200			{object}	RedirectResponse	"Redirect information (for API testing)"
//		@Failure		404			{object}	ErrorResponse		"Short code not found"
//		@Failure		410			{object}	ErrorResponse		"URL has expired"
//		@Failure		500			{object}	ErrorResponse		"Internal server error"
//		@Router			/{shortCode} [get]
//
// RedirectURL 处理 GET /:shortCode (包含关键异步 Trace 修复)
func (h *URLHandler) RedirectURL(c *gin.Context) {
	shortCode := c.Param("shortCode")
	userAgent := c.GetHeader("User-Agent")
	ipAddress := c.ClientIP()
	referrer := c.GetHeader("Referer")

	// 1. 同步解析请求：传递 Context
	resolveCtx, resolveCancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer resolveCancel()

	// Inject trace into metadata for the redirect service call (synchronous)
	resolveCtx = tracing.InjectTraceToGoMicroContext(resolveCtx)

	rsp, err := h.redirectClient.ResolveURL(resolveCtx, &redirectpb.ResolveRequest{
		ShortCode: shortCode,
		ClientIp:  ipAddress,
		UserAgent: userAgent,
		Referrer:  referrer,
	})

	if err != nil || !rsp.Found {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Short URL not found"})
		return
	}

	if rsp.Expired {
		c.JSON(http.StatusGone, ErrorResponse{Error: "This short URL has expired"})
		return
	}

	// 【性能优化】：移除冗余的异步 TrackClick RPC 调用
	// redirect-svc 在 ResolveURL 内部已经通过 publishClickEvent 异步发送点击事件到 NATS
	// 再从 rest-api-svc 端多发一次 TrackClick RPC 纯属浪费 NATS 带宽和 goroutine 资源

	// 执行重定向
	if c.GetHeader("Accept") == "application/json" {
		c.JSON(http.StatusOK, RedirectResponse{
			LongURL:    rsp.LongUrl,
			ShortCode:  shortCode,
			ClickCount: rsp.ClickCount,
			Timestamp:  time.Now().Unix(),
		})
		return
	}
	c.Redirect(http.StatusFound, rsp.LongUrl)
}

// =============================================================================
// 分析统计接口
// =============================================================================

// URLStatsResponse 结构体：表示短链接的分析统计响应数据
type URLStatsResponse struct {
	ShortCode     string              `json:"short_code" example:"abc123"`
	TotalClicks   int64               `json:"total_clicks" example:"150"`
	UniqueClicks  int64               `json:"unique_clicks" example:"95"`
	TimeSeries    []TimeSeriesPoint   `json:"time_series"`
	CountryStats  []CountryStatsItem  `json:"country_stats"`
	DeviceStats   []DeviceStatsItem   `json:"device_stats"`
	BrowserStats  []BrowserStatsItem  `json:"browser_stats"`
	ReferrerStats []ReferrerStatsItem `json:"referrer_stats"`
}

// TimeSeriesPoint 结构体：表示时间序列数据中的一个数据点
type TimeSeriesPoint struct {
	Timestamp    int64 `json:"timestamp" example:"1672531200"`
	Clicks       int64 `json:"clicks" example:"25"`
	UniqueClicks int64 `json:"unique_clicks" example:"18"`
}

// CountryStatsItem 结构体：表示按国家/地区划分的分析数据项
type CountryStatsItem struct {
	// 国家代码：通常使用 ISO 标准缩写（如 "CN" 代表中国, "US" 代表美国）
	Country string `json:"country" example:"US"`
	// 点击量：来自该国家或地区的总访问次数
	Clicks int64 `json:"clicks" example:"75"`
	// 百分比：该地区的点击量占总点击量的百分比（0.0-100.0）
	Percentage float32 `json:"percentage" example:"50.0"`
}

// DeviceStatsItem 表示设备类型分析统计项
type DeviceStatsItem struct {
	DeviceType string  `json:"device_type" example:"Desktop"` // 设备类型（如 Desktop, Mobile, Tablet）
	Clicks     int64   `json:"clicks" example:"90"`           // 来自该类型设备的点击总数
	Percentage float32 `json:"percentage" example:"60.0"`     // 该设备点击量在总点击中的占比百分比
}

// BrowserStatsItem 表示浏览器分析统计项
type BrowserStatsItem struct {
	Browser    string  `json:"browser" example:"Chrome 120"` // 浏览器名称及版本号
	Clicks     int64   `json:"clicks" example:"80"`          // 该浏览器的累计点击数
	Percentage float32 `json:"percentage" example:"53.3"`    // 该浏览器占比百分比 (0-100)
}

// ReferrerStatsItem 表示来源页面（引荐来源）的分析统计项
type ReferrerStatsItem struct {
	Referrer   string  `json:"referrer" example:"https://google.com"` // 来源地址（如搜索引擎、社交媒体或特定网站）
	Clicks     int64   `json:"clicks" example:"45"`                   // 该来源贡献的累计点击数
	Percentage float32 `json:"percentage" example:"30.0"`             // 该来源占总流量的百分比 (0-100)
}

// TopURLsResponse 表示热门 URL 排行榜的分析响应
type TopURLsResponse struct {
	URLs []TopURLItem `json:"urls"` // 热门短链接信息列表
}

// TopURLItem 表示表现优异的单个短链接统计项
type TopURLItem struct {
	ShortCode    string `json:"short_code" example:"abc123"`       // 短码标识符
	TotalClicks  int64  `json:"total_clicks" example:"150"`        // 总点击次数
	UniqueClicks int64  `json:"unique_clicks" example:"95"`        // 独立访客点击数
	CreatedAt    int64  `json:"created_at" example:"1672531200"`   // 链接创建的时间戳
	LastClicked  int64  `json:"last_clicked" example:"1672617600"` // 最后一次被点击的时间戳
}

// DashboardResponse 表示仪表盘总览分析统计响应
type DashboardResponse struct {
	TotalURLs       int64              `json:"total_urls" example:"500"`     // 系统中总共创建的短链接数量
	TotalClicks     int64              `json:"total_clicks" example:"12500"` // 所有链接累计的总点击次数
	UniqueClicks    int64              `json:"unique_clicks" example:"8750"` // 所有链接累计的独立访客数 (UV)
	ActiveURLs      int64              `json:"active_urls" example:"425"`    // 当前处于激活状态（未过期且未禁用）的链接数
	ClickTimeline   []TimeSeriesPoint  `json:"click_timeline"`               // 点击量随时间变化的趋势数据
	TopCountries    []CountryStatsItem `json:"top_countries"`                // 流量最高的国家/地区分布排名
	DeviceBreakdown []DeviceStatsItem  `json:"device_breakdown"`             // 访问设备的分类占比明细
}

// GetURLStats 处理 GET /api/v1/analytics/urls/:shortCode 接口
//
//	 @Summary      获取 URL 分析统计数据
//	 @Description   检索特定短链接的详尽分析数据（包括点击量、地理位置、设备分布等）
//	 @Tags        分析统计
//	 @Accept          json
//	 @Produce      json
//	 @Param       shortCode  path      string          true   "短码标识符"    example(abc123)
//	 @Param       start_time query     int64           false  "开始时间 (Unix 时间戳)"  example(1672531200)
//	 @Param       end_time   query     int64           false  "结束时间 (Unix 时间戳)"    example(1672617600)
//	 @Param       granularity    query     string          false  "时间粒度 (hour/day/week/month)"   example(day)
//		@Success		200			{object}	URLStatsResponse	"URL analytics retrieved successfully"
//		@Failure		400			{object}	ErrorResponse		"Invalid parameters"
//		@Failure		404			{object}	ErrorResponse		"URL not found"
//		@Failure		500			{object}	ErrorResponse		"Internal server error"
//		@Router			/analytics/urls/{shortCode} [get]
func (h *URLHandler) GetURLStats(c *gin.Context) {
	shortCode := c.Param("shortCode") // 从路径中获取短码

	// 解析查询参数
	var startTime, endTime int64
	var err error

	if startTimeStr := c.Query("start_time"); startTimeStr != "" {
		startTime, err = strconv.ParseInt(startTimeStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid start_time format"})
			return
		}
	} else {
		startTime = time.Now().AddDate(0, 0, -30).Unix() // 默认获取过去 30 天的数据
	}

	if endTimeStr := c.Query("end_time"); endTimeStr != "" {
		endTime, err = strconv.ParseInt(endTimeStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid end_time format"})
			return
		}
	} else {
		endTime = time.Now().Unix()
	}

	granularity := c.Query("granularity")
	if granularity == "" {
		granularity = "day"
	}

	h.log.WithFields(logrus.Fields{
		"short_code":  shortCode,
		"start_time":  startTime,
		"end_time":    endTime,
		"granularity": granularity,
	}).Info("Processing GetURLStats analytics request")

	// 调用后端分析统计微服务
	// 【修复点】使用 c.Request.Context()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	rsp, err := h.analyticsClient.GetURLStats(ctx, &analyticspb.StatsRequest{
		ShortCode:   shortCode,
		StartTime:   startTime,
		EndTime:     endTime,
		Granularity: granularity,
	})
	if err != nil {
		h.log.WithError(err).Error("Failed to get URL analytics")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve analytics"})
		return
	}

	// 转换为 REST 响应格式
	response := URLStatsResponse{
		ShortCode:    rsp.ShortCode,
		TotalClicks:  rsp.TotalClicks,
		UniqueClicks: rsp.UniqueClicks,
	}

	// 将上游 RPC 返回的复杂维度数据映射为 REST 响应格式
	// 时间序列数据
	for _, ts := range rsp.TimeSeries {
		response.TimeSeries = append(response.TimeSeries, TimeSeriesPoint{
			Timestamp:    ts.Timestamp,
			Clicks:       ts.Clicks,
			UniqueClicks: ts.UniqueClicks,
		})
	}

	// 国家/地区统计
	for _, country := range rsp.CountryStats {
		response.CountryStats = append(response.CountryStats, CountryStatsItem{
			Country:    country.Country,
			Clicks:     country.Clicks,
			Percentage: country.Percentage,
		})
	}

	// 设备类型统计
	for _, device := range rsp.DeviceStats {
		response.DeviceStats = append(response.DeviceStats, DeviceStatsItem{
			DeviceType: device.DeviceType,
			Clicks:     device.Clicks,
			Percentage: device.Percentage,
		})
	}

	// 浏览器统计
	for _, browser := range rsp.BrowserStats {
		response.BrowserStats = append(response.BrowserStats, BrowserStatsItem{
			Browser:    browser.Browser,
			Clicks:     browser.Clicks,
			Percentage: browser.Percentage,
		})
	}

	// 来源统计
	for _, referrer := range rsp.ReferrerStats {
		response.ReferrerStats = append(response.ReferrerStats, ReferrerStatsItem{
			Referrer:   referrer.Referrer,
			Clicks:     referrer.Clicks,
			Percentage: referrer.Percentage,
		})
	}

	h.log.WithFields(logrus.Fields{
		"short_code":     shortCode,
		"total_clicks":   response.TotalClicks,
		"unique_clicks":  response.UniqueClicks,
		"time_series":    len(response.TimeSeries),
		"country_stats":  len(response.CountryStats),
		"device_stats":   len(response.DeviceStats),
		"browser_stats":  len(response.BrowserStats),
		"referrer_stats": len(response.ReferrerStats),
	}).Info("已成功获取 URL 分析数据（含完整维度明细）")

	c.JSON(http.StatusOK, response) // 返回最终 JSON
}

// GetTopURLs 处理 GET /api/v1/analytics/top-urls 接口
//
//	 @Summary      获取热门 URL 排行榜
//	 @Description   根据点击量检索表现最出色的顶级短链接列表
//	 @Tags        分析统计
//	 @Accept          json
//	 @Produce      json
//	 @Param       limit     query     int32           false  "返回的 URL 数量" example(10) // 参数：限制条数
//	 @Param       start_time query     int64           false  "开始时间 (Unix 时间戳)"  example(1672531200)
//	 @Param       end_time   query     int64           false  "结束时间 (Unix 时间戳)"    example(1672617600)
//	 @Param       sort_by       query     string          false  "排序维度 (clicks/unique_clicks/created_at)"    example(clicks)
//		@Success		200			{object}	TopURLsResponse		"Top URLs retrieved successfully"
//		@Failure		400			{object}	ErrorResponse		"Invalid parameters"
//		@Failure		500			{object}	ErrorResponse		"Internal server error"
//		@Router			/analytics/top-urls [get]
func (h *URLHandler) GetTopURLs(c *gin.Context) {
	// 解析查询参数
	var limit int32 = 10 // Default limit
	var startTime, endTime int64
	var err error

	if limitStr := c.Query("limit"); limitStr != "" {
		limitParsed, err := strconv.ParseInt(limitStr, 10, 32)
		if err != nil || limitParsed <= 0 {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid limit format"})
			return
		}
		limit = int32(limitParsed) //// 转换为 int32 供 RPC 调用
	}

	if startTimeStr := c.Query("start_time"); startTimeStr != "" {
		startTime, err = strconv.ParseInt(startTimeStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid start_time format"})
			return
		}
	} else {
		startTime = time.Now().AddDate(0, 0, -30).Unix() // Default to last 30 days
	}

	if endTimeStr := c.Query("end_time"); endTimeStr != "" {
		endTime, err = strconv.ParseInt(endTimeStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid end_time format"})
			return
		}
	} else {
		endTime = time.Now().Unix()
	}

	sortBy := c.Query("sort_by") // 获取排序字段（如 clicks）
	if sortBy == "" {
		sortBy = "clicks" // 默认按总点击量排序
	}

	h.log.WithFields(logrus.Fields{
		"limit":      limit,
		"start_time": startTime,
		"end_time":   endTime,
		"sort_by":    sortBy,
	}).Info("正在处理获取热门排行分析请求")

	// 调用后端分析统计微服务获取数据
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	rsp, err := h.analyticsClient.GetTopURLs(ctx, &analyticspb.TopURLsRequest{
		Limit:     limit,
		StartTime: startTime,
		EndTime:   endTime,
		SortBy:    sortBy,
	})
	if err != nil {
		h.log.WithError(err).Error("获取热门 URL 数据失败")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve top URLs"})
		return
	}

	// 将 RPC 返回的 Protobuf 数据转换为 REST API 的响应结构体
	response := TopURLsResponse{}
	for _, url := range rsp.Urls {
		response.URLs = append(response.URLs, TopURLItem{
			ShortCode:    url.ShortCode,
			TotalClicks:  url.TotalClicks,
			UniqueClicks: url.UniqueClicks,
			CreatedAt:    url.CreatedAt,
			LastClicked:  url.LastClicked,
		})
	}

	h.log.WithField("count", len(response.URLs)).Info("成功获取热门 URL 排行榜数据")
	c.JSON(http.StatusOK, response) // 返回 200 OK 及数据
}

// GetDashboard 返回综合分析仪表盘数据
func (h *URLHandler) GetDashboard(c *gin.Context) {
	h.log.Info("GetDashboard 接口被调用") // 记录函数进入日志

	// 解析查询参数
	startTimeParam := c.Query("start_time") // 获取开始时间参数
	endTimeParam := c.Query("end_time")     // 获取结束时间参数

	// 创建请求对象
	req := &analyticspb.DashboardRequest{} // 初始化 RPC 请求结构体

	// 如果提供了时间参数，则进行解析
	if startTimeParam != "" {
		if startTime, err := time.Parse(time.RFC3339, startTimeParam); err == nil {
			req.StartTime = startTime.Unix()
		}
	}
	if endTimeParam != "" {
		if endTime, err := time.Parse(time.RFC3339, endTimeParam); err == nil {
			req.EndTime = endTime.Unix()
		}
	}

	h.log.WithFields(logrus.Fields{
		"start_time": req.StartTime,
		"end_time":   req.EndTime,
	}).Info("正在调用分析服务 GetDashboard")

	// 调用后端分析服务
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	rsp, err := h.analyticsClient.GetDashboard(ctx, req)
	if err != nil {
		h.log.WithFields(logrus.Fields{
			"error":      err.Error(),
			"error_type": fmt.Sprintf("%T", err),
		}).Error("调用分析服务 GetDashboard 失败")

		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to retrieve dashboard analytics",
			"details": err.Error(),
		})
		return
	}

	h.log.WithFields(logrus.Fields{
		"total_urls":    rsp.TotalUrls,
		"total_clicks":  rsp.TotalClicks,
		"unique_clicks": rsp.UniqueClicks,
		"active_urls":   rsp.ActiveUrls,
	}).Info("分析服务成功返回仪表盘数据")

	// 将 Protobuf 响应转换为 REST API 使用的 JSON 友好格式
	dashboard := DashboardResponse{
		TotalURLs:    rsp.TotalUrls,
		TotalClicks:  rsp.TotalClicks,
		UniqueClicks: rsp.UniqueClicks,
		ActiveURLs:   rsp.ActiveUrls,
	}

	// 转换点击量时间轴数据
	for _, ts := range rsp.ClickTimeline {
		dashboard.ClickTimeline = append(dashboard.ClickTimeline, TimeSeriesPoint{
			Timestamp:    ts.Timestamp,
			Clicks:       ts.Clicks,
			UniqueClicks: ts.UniqueClicks,
		})
	}

	// 转换热门国家数据
	for _, country := range rsp.TopCountries {
		dashboard.TopCountries = append(dashboard.TopCountries, CountryStatsItem{
			Country:    country.Country,
			Clicks:     country.Clicks,
			Percentage: country.Percentage,
		})
	}

	// 转换设备分布数据
	for _, device := range rsp.DeviceBreakdown {
		dashboard.DeviceBreakdown = append(dashboard.DeviceBreakdown, DeviceStatsItem{
			DeviceType: device.DeviceType,
			Clicks:     device.Clicks,
			Percentage: device.Percentage,
		})
	}

	h.log.Info("仪表盘指标成功转换为 JSON")

	c.JSON(http.StatusOK, dashboard) // 返回最终统计结果
}
