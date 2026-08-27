package handler

import (
	"context"
	"fmt"
	oteltrace "go.opentelemetry.io/otel/trace"
	"time"

	"github.com/sirupsen/logrus"

	pb "github.com/go-systems-lab/go-url-shortener/proto/url"
	"github.com/go-systems-lab/go-url-shortener/services/url-shortener-svc/store"
	"github.com/go-systems-lab/go-url-shortener/utils/cache"
	"github.com/go-systems-lab/go-url-shortener/utils/database"
	"github.com/go-systems-lab/go-url-shortener/utils/tracing"
)

type URLHandler struct {
	store *store.URLStore
	log   *logrus.Logger
}

func NewURLHandler(db *database.PostgreSQL, cache *cache.Redis) pb.URLShortenerHandler {
	urlStore := store.NewURLStore(db, cache)
	return &URLHandler{
		store: urlStore,
		log:   logrus.New(),
	}
}

func (h *URLHandler) ShortenURL(ctx context.Context, req *pb.ShortenRequest, rsp *pb.ShortenResponse) error {
	// ADDED DEBUG: 打印 handler 一开始拿到的 span context（trace id）
	sc := oteltrace.SpanFromContext(ctx).SpanContext()
	h.log.WithFields(logrus.Fields{
		"location": "url-shortener handler start",
		"trace_id": sc.TraceID().String(),
		"span_id":  sc.SpanID().String(),
		"valid":    sc.IsValid(),
	}).Info("ADDED DEBUG: handler received span context")

	tracer := tracing.NewTracer("url-shortener-svc")
	businessCtx, span := tracer.StartSpan(ctx, "Business.ShortenURL")
	defer span.End()

	h.log.WithFields(logrus.Fields{
		"long_url": req.LongUrl,
		"user_id":  req.UserId,
	}).Info("正在处理 ShortenURL 请求")

	storeReq := &store.ShortenURLRequest{
		LongURL:     req.LongUrl,
		CustomAlias: req.CustomAlias,
		UserID:      req.UserId,
		Metadata:    req.Metadata,
	}

	if req.ExpirationTime > 0 {
		expirationTime := time.Unix(req.ExpirationTime, 0)
		storeReq.ExpirationTime = &expirationTime
	}

	// 传递 businessCtx，解决 Unused variable 报错并接力 Trace
	urlResponse, err := h.store.ShortenURL(businessCtx, storeReq)
	if err != nil {
		tracing.RecordError(span, err)
		h.log.WithError(err).Error("缩短 URL 失败")
		return fmt.Errorf("failed to shorten URL: %w", err)
	}

	rsp.ShortCode = urlResponse.ShortCode
	rsp.ShortUrl = fmt.Sprintf("https://short.ly/%s", urlResponse.ShortCode)
	rsp.LongUrl = urlResponse.LongURL
	rsp.CreatedAt = urlResponse.CreatedAt.Unix()
	rsp.UserId = urlResponse.UserID

	if urlResponse.ExpiresAt != nil {
		rsp.ExpiresAt = urlResponse.ExpiresAt.Unix()
	}

	tracing.RecordSuccess(span)
	return nil
}

func (h *URLHandler) GetURLInfo(ctx context.Context, req *pb.GetURLRequest, rsp *pb.URLInfo) error {
	tracer := tracing.NewTracer("url-shortener-svc")
	businessCtx, span := tracer.StartSpan(ctx, "RPC.GetURLInfo")
	defer span.End()

	h.log.WithFields(logrus.Fields{
		"short_code": req.ShortCode,
		"user_id":    req.UserId,
	}).Info("正在处理 GetURLInfo 请求")

	// 传递 businessCtx
	urlResponse, err := h.store.GetURL(businessCtx, req.ShortCode, req.UserId)
	if err != nil {
		tracing.RecordError(span, err)
		h.log.WithError(err).Error("获取 URL 信息失败")
		return fmt.Errorf("failed to get URL info: %w", err)
	}

	rsp.ShortCode = urlResponse.ShortCode
	rsp.ShortUrl = fmt.Sprintf("https://short.ly/%s", urlResponse.ShortCode)
	rsp.LongUrl = urlResponse.LongURL
	rsp.UserId = urlResponse.UserID
	rsp.CreatedAt = urlResponse.CreatedAt.Unix()
	rsp.ClickCount = urlResponse.ClickCount
	rsp.IsActive = urlResponse.IsActive
	rsp.Metadata = urlResponse.Metadata

	if urlResponse.ExpiresAt != nil {
		rsp.ExpiresAt = urlResponse.ExpiresAt.Unix()
	}

	tracing.RecordSuccess(span)
	return nil
}

func (h *URLHandler) DeleteURL(ctx context.Context, req *pb.DeleteURLRequest, rsp *pb.DeleteResponse) error {
	tracer := tracing.NewTracer("url-shortener-svc")
	businessCtx, span := tracer.StartSpan(ctx, "RPC.DeleteURL")
	defer span.End()

	h.log.WithFields(logrus.Fields{
		"short_code": req.ShortCode,
		"user_id":    req.UserId,
	}).Info("正在处理 DeleteURL 请求")

	// 传递 businessCtx
	err := h.store.DeleteURL(businessCtx, req.ShortCode, req.UserId)
	if err != nil {
		tracing.RecordError(span, err)
		h.log.WithError(err).Error("删除 URL 失败")
		rsp.Success = false
		rsp.Message = fmt.Sprintf("Failed to delete URL: %v", err)
		return nil
	}

	rsp.Success = true
	rsp.Message = "URL deleted successfully"
	tracing.RecordSuccess(span)
	return nil
}

func (h *URLHandler) GetUserURLs(ctx context.Context, req *pb.GetUserURLsRequest, rsp *pb.GetUserURLsResponse) error {
	tracer := tracing.NewTracer("url-shortener-svc")
	businessCtx, span := tracer.StartSpan(ctx, "RPC.GetUserURLs")
	defer span.End()

	h.log.WithFields(logrus.Fields{
		"user_id":   req.UserId,
		"page":      req.Page,
		"page_size": req.PageSize,
	}).Info("正在处理 GetUserURLs 请求")

	storeReq := &store.GetUserURLsRequest{
		UserID:    req.UserId,
		Page:      req.Page,
		PageSize:  req.PageSize,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}

	// 传递 businessCtx
	storeResponse, err := h.store.GetUserURLs(businessCtx, storeReq)
	if err != nil {
		tracing.RecordError(span, err)
		h.log.WithError(err).Error("获取用户 URL 列表失败")
		return fmt.Errorf("failed to get user URLs: %w", err)
	}

	urls := make([]*pb.URLInfo, len(storeResponse.URLs))
	for i, storeURL := range storeResponse.URLs {
		urlInfo := &pb.URLInfo{
			ShortCode:  storeURL.ShortCode,
			ShortUrl:   fmt.Sprintf("https://short.ly/%s", storeURL.ShortCode),
			LongUrl:    storeURL.LongURL,
			UserId:     storeURL.UserID,
			CreatedAt:  storeURL.CreatedAt.Unix(),
			ClickCount: storeURL.ClickCount,
			IsActive:   storeURL.IsActive,
			Metadata:   storeURL.Metadata,
		}
		if storeURL.ExpiresAt != nil {
			urlInfo.ExpiresAt = storeURL.ExpiresAt.Unix()
		}
		urls[i] = urlInfo
	}

	rsp.Urls = urls
	rsp.TotalCount = storeResponse.TotalCount
	rsp.Page = storeResponse.Page
	rsp.PageSize = storeResponse.PageSize
	rsp.HasNext = storeResponse.HasNext

	tracing.RecordSuccess(span)
	return nil
}

func (h *URLHandler) UpdateURL(ctx context.Context, req *pb.UpdateURLRequest, rsp *pb.UpdateURLResponse) error {
	tracer := tracing.NewTracer("url-shortener-svc")
	businessCtx, span := tracer.StartSpan(ctx, "RPC.UpdateURL")
	defer span.End()

	h.log.WithFields(logrus.Fields{
		"short_code": req.ShortCode,
		"user_id":    req.UserId,
	}).Info("正在处理 UpdateURL 请求")

	storeReq := &store.UpdateURLRequest{
		ShortCode:  req.ShortCode,
		UserID:     req.UserId,
		NewLongURL: req.NewLongUrl,
		Metadata:   req.Metadata,
	}

	if req.NewExpirationTime > 0 {
		newExpirationTime := time.Unix(req.NewExpirationTime, 0)
		storeReq.NewExpirationTime = &newExpirationTime
	}

	// 传递 businessCtx
	urlResponse, err := h.store.UpdateURL(businessCtx, storeReq)
	if err != nil {
		tracing.RecordError(span, err)
		h.log.WithError(err).Error("更新 URL 失败")
		rsp.Success = false
		rsp.Message = fmt.Sprintf("Failed to update URL: %v", err)
		return nil
	}

	updatedURL := &pb.URLInfo{
		ShortCode:  urlResponse.ShortCode,
		ShortUrl:   fmt.Sprintf("https://short.ly/%s", urlResponse.ShortCode),
		LongUrl:    urlResponse.LongURL,
		UserId:     urlResponse.UserID,
		CreatedAt:  urlResponse.CreatedAt.Unix(),
		ClickCount: urlResponse.ClickCount,
		IsActive:   urlResponse.IsActive,
		Metadata:   urlResponse.Metadata,
	}

	if urlResponse.ExpiresAt != nil {
		updatedURL.ExpiresAt = urlResponse.ExpiresAt.Unix()
	}

	rsp.Success = true
	rsp.Message = "URL updated successfully"
	rsp.UpdatedUrl = updatedURL

	tracing.RecordSuccess(span)
	return nil
}
