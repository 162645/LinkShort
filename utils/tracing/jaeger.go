package tracing

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"go-micro.dev/v5/metadata"

	"go-micro.dev/v5/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TracingConfig 存储 Jaeger 链路追踪的配置信息
type TracingConfig struct {
	ServiceName    string  // 服务名称
	ServiceVersion string  // 服务版本
	Environment    string  // 部署环境 (development/production)
	JaegerEndpoint string  // Jaeger 收集器的地址 (gRPC 端口通常是 4317)
	SamplingRatio  float64 // 采样率 (0.0 到 1.0)
}

// DefaultTracingConfig 获取默认配置，优先从环境变量读取
func DefaultTracingConfig(serviceName string) *TracingConfig {
	return &TracingConfig{
		ServiceName:    serviceName,
		ServiceVersion: getEnv("SERVICE_VERSION", "1.0.0"),
		Environment:    getEnv("ENVIRONMENT", "development"),
		JaegerEndpoint: getEnv("JAEGER_ENDPOINT", "localhost:4317"),
		SamplingRatio:  0.01, // 压测环境下调整为 1% 采样，提升吞吐量
	}
}

// InitJaeger 使用 OTLP gRPC 导出器初始化服务的 Jaeger 链路追踪
func InitJaeger(config *TracingConfig) (*trace.TracerProvider, error) {
	// 使用超时上下文确保连接不会无限等待
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dialCancel()

	conn, err := grpc.DialContext(dialCtx, config.JaegerEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to dial Jaeger OTLP endpoint %s: %w", config.JaegerEndpoint, err)
	}

	// 给 exporter 也用一个短超时 ctx（避免在创建时 hang）
	expCtx, expCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer expCancel()

	exporter, err := otlptracegrpc.New(expCtx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP gRPC exporter: %w", err)
	}

	// Resource metadata
	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
			semconv.DeploymentEnvironment(config.Environment),
			attribute.String("service.type", "microservice"),
			attribute.String("service.framework", "go-micro"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// 使用配置的采样率（调试时可设为 1.0）
	var sampler trace.Sampler
	if config.SamplingRatio >= 1.0 {
		sampler = trace.AlwaysSample()
	} else if config.SamplingRatio <= 0.0 {
		sampler = trace.NeverSample()
	} else {
		sampler = trace.ParentBased(trace.TraceIDRatioBased(config.SamplingRatio))
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(res),
		trace.WithSampler(sampler),
	)

	// 设置全局 TracerProvider 与 Propagator（非常关键）
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	fmt.Printf("✅ Connected to Jaeger OTLP at %s (sampling=%v)\n", config.JaegerEndpoint, config.SamplingRatio)
	return tp, nil
}

// Tracer 结构体对 OpenTelemetry 的原生 Tracer 进行了二次封装
type Tracer struct {
	// tracer 是 OpenTelemetry 库中定义的原生追踪器接口
	tracer oteltrace.Tracer
}

// NewTracer 创建一个新的追踪器实例
func NewTracer(serviceName string) *Tracer {
	return &Tracer{
		tracer: otel.Tracer(serviceName),
	}
}

// StartSpan 开启一个具有指定名称和选项的新跨度 (Span)
func (t *Tracer) StartSpan(ctx context.Context, spanName string, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	return t.tracer.Start(ctx, spanName, opts...)
}

// StartHTTPSpan 为 HTTP 请求开启一个跨度
func (t *Tracer) StartHTTPSpan(ctx context.Context, method, endpoint string) (context.Context, oteltrace.Span) {
	spanName := fmt.Sprintf("HTTP %s %s", method, endpoint)
	ctx, span := t.tracer.Start(ctx, spanName)

	span.SetAttributes(
		attribute.String("http.method", method),
		attribute.String("http.route", endpoint),
		attribute.String("span.kind", "server"),
	)

	return ctx, span
}

// StartGRPCSpan 为 gRPC 请求开启一个跨度
func (t *Tracer) StartGRPCSpan(ctx context.Context, service, method string) (context.Context, oteltrace.Span) {
	spanName := fmt.Sprintf("gRPC %s/%s", service, method)
	ctx, span := t.tracer.Start(ctx, spanName)

	span.SetAttributes(
		attribute.String("rpc.system", "grpc"),
		attribute.String("rpc.service", service),
		attribute.String("rpc.method", method),
		attribute.String("span.kind", "server"),
	)

	return ctx, span
}

// StartDatabaseSpan 为数据库操作开启一个跨度
func (t *Tracer) StartDatabaseSpan(ctx context.Context, operation, table string) (context.Context, oteltrace.Span) {
	spanName := fmt.Sprintf("DB %s %s", operation, table)
	ctx, span := t.tracer.Start(ctx, spanName)

	span.SetAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", operation),
		attribute.String("db.sql.table", table),
		attribute.String("span.kind", "client"),
	)

	return ctx, span
}

// StartCacheSpan 为缓存操作开启一个跨度
func (t *Tracer) StartCacheSpan(ctx context.Context, operation, key string) (context.Context, oteltrace.Span) {
	spanName := fmt.Sprintf("Cache %s", operation)
	ctx, span := t.tracer.Start(ctx, spanName)

	span.SetAttributes(
		attribute.String("cache.system", "redis"),
		attribute.String("cache.operation", operation),
		attribute.String("cache.key", key),
		attribute.String("span.kind", "client"),
	)

	return ctx, span
}

// StartNATSSpan starts a span for NATS messaging
func (t *Tracer) StartNATSSpan(ctx context.Context, operation, subject string) (context.Context, oteltrace.Span) {
	spanName := fmt.Sprintf("NATS %s %s", operation, subject)
	ctx, span := t.tracer.Start(ctx, spanName)

	span.SetAttributes(
		attribute.String("messaging.system", "nats"),
		attribute.String("messaging.operation", operation),
		attribute.String("messaging.destination", subject),
		attribute.String("span.kind", "producer"),
	)

	return ctx, span
}

// RecordError records an error in the span
func RecordError(span oteltrace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// RecordSuccess marks the span as successful
func RecordSuccess(span oteltrace.Span) {
	span.SetStatus(codes.Ok, "")
}

// AddAttributes adds attributes to the span
func AddAttributes(span oteltrace.Span, attrs ...attribute.KeyValue) {
	span.SetAttributes(attrs...)
}

// TraceHTTPMiddleware provides tracing for HTTP requests
func TraceHTTPMiddleware(serviceName string) func(next http.Handler) http.Handler {
	tracer := NewTracer(serviceName)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract trace context from incoming request
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			ctx, span := tracer.StartHTTPSpan(ctx, r.Method, r.URL.Path)
			defer span.End()

			// Add request attributes
			span.SetAttributes(
				attribute.String("http.url", r.URL.String()),
				attribute.String("http.user_agent", r.UserAgent()),
				attribute.String("http.remote_addr", r.RemoteAddr),
				attribute.String("service.name", serviceName),
			)

			// Create response writer wrapper to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}

			// Process request
			next.ServeHTTP(wrapped, r.WithContext(ctx))

			// Record response attributes
			span.SetAttributes(
				attribute.Int("http.status_code", wrapped.statusCode),
			)

			// Set span status based on HTTP status code
			if wrapped.statusCode >= 400 {
				span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", wrapped.statusCode))
			} else {
				span.SetStatus(codes.Ok, "")
			}
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// TraceGoMicroMiddleware provides tracing for go-micro server handlers
func TraceGoMicroMiddleware(serviceName string) server.HandlerWrapper {
	tracer := NewTracer(serviceName)
	propagator := propagation.TraceContext{}

	return func(fn server.HandlerFunc) server.HandlerFunc {
		return func(ctx context.Context, req server.Request, rsp interface{}) error {
			md, ok := metadata.FromContext(ctx)
			if !ok || md == nil {
				md = map[string]string{}
			}

			// --- 【修复瀑布图的核心：兼容性处理】 ---
			// 检查是否有首字母大写的 Traceparent，如果有且没有小写的，手动补全
			if val, exists := md["Traceparent"]; exists && md["traceparent"] == "" {
				md["traceparent"] = val
			}
			// --------------------------------------

			// 现在执行 Extract，它一定能找到小写的 traceparent
			ctx = propagator.Extract(ctx, propagation.MapCarrier(md))

			// 开启 Span
			ctx, span := tracer.tracer.Start(ctx, fmt.Sprintf("RPC %s.%s", req.Service(), req.Method()),
				oteltrace.WithSpanKind(oteltrace.SpanKindServer),
			)
			defer span.End()

			// 验证：提取 TraceID (注释掉高压测下会严重拉低性能的 stdout 打印)
			// sc := oteltrace.SpanContextFromContext(ctx)

			return fn(ctx, req, rsp)
		}
	}
}

// Business Logic Tracing Helpers
// TraceURLShortening traces URL shortening operations
func TraceURLShortening(ctx context.Context, tracer *Tracer, longURL, userID string) (context.Context, oteltrace.Span) {
	ctx, span := tracer.StartSpan(ctx, "url.shorten")
	span.SetAttributes(
		attribute.String("url.long", longURL),
		attribute.String("user.id", userID),
		attribute.String("operation", "shorten"),
	)
	return ctx, span
}

// TraceURLRedirection traces URL redirection operations
func TraceURLRedirection(ctx context.Context, tracer *Tracer, shortCode string) (context.Context, oteltrace.Span) {
	ctx, span := tracer.StartSpan(ctx, "url.redirect")
	span.SetAttributes(
		attribute.String("url.short_code", shortCode),
		attribute.String("operation", "redirect"),
	)
	return ctx, span
}

// TraceAnalyticsEvent traces analytics event processing
func TraceAnalyticsEvent(ctx context.Context, tracer *Tracer, eventType, shortCode string) (context.Context, oteltrace.Span) {
	ctx, span := tracer.StartSpan(ctx, "analytics.process")
	span.SetAttributes(
		attribute.String("analytics.event_type", eventType),
		attribute.String("url.short_code", shortCode),
		attribute.String("operation", "analytics"),
	)
	return ctx, span
}

// Utility functions

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// InjectTraceToGoMicroContext 将当前 ctx 中的 trace 信息注入到 go-micro 的 metadata 中并返回新的 context
func InjectTraceToGoMicroContext(ctx context.Context) context.Context {
	// 先读出已有的 go-micro metadata（如果有）
	existing, ok := metadata.FromContext(ctx)
	md := map[string]string{}
	if ok && existing != nil {
		// 复制一份已有 key，避免修改原 map（安全）
		for k, v := range existing {
			md[k] = v
		}
	}

	// 注入 trace 到这个 md（map）
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(md))

	// 将合并后的 md 放回 context
	return metadata.NewContext(ctx, md)
}
