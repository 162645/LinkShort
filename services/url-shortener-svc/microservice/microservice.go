package microservice

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"go-micro.dev/v5"
	"go-micro.dev/v5/client"
	"go-micro.dev/v5/registry"
	"go-micro.dev/v5/transport"
	"go.opentelemetry.io/otel/sdk/trace"

	// 插件
	natsBroker "github.com/micro/plugins/v5/broker/nats"
	natsRegistry "github.com/micro/plugins/v5/registry/nats"
	natsTransport "github.com/micro/plugins/v5/transport/nats"

	// 【新增】OpenTelemetry 官方插件用于包装 Client
	"github.com/micro/plugins/v5/wrapper/trace/opentelemetry"

	pb "github.com/go-systems-lab/go-url-shortener/proto/url"
	"github.com/go-systems-lab/go-url-shortener/services/url-shortener-svc/handler"
	"github.com/go-systems-lab/go-url-shortener/utils/cache"
	"github.com/go-systems-lab/go-url-shortener/utils/database"
	"github.com/go-systems-lab/go-url-shortener/utils/metrics"
	"github.com/go-systems-lab/go-url-shortener/utils/tracing"
)

type ClientOptions struct {
	Version string
	Log     *logrus.Logger
}

type Microservice struct {
	service micro.Service
	log     *logrus.Logger
	tracer  *tracing.Tracer
	tp      *trace.TracerProvider
	db      *database.PostgreSQL // 提升到结构体以便 Close
	cache   *cache.Redis         // 提升到结构体以便 Close
}

func Init(opts *ClientOptions) (*Microservice, error) {
	// 1. 初始化追踪配置
	tracingConfig := tracing.DefaultTracingConfig("url.shortener.service")
	tp, err := tracing.InitJaeger(tracingConfig)
	if err != nil {
		opts.Log.WithError(err).Warn("Jaeger 初始化失败，将不带追踪运行")
	} else {
		opts.Log.Info("Jaeger 链路追踪初始化成功")
	}

	tracer := tracing.NewTracer("url.shortener.service")

	// 2. 初始化基础设施
	metricsRegistry := metrics.NewMetrics()
	db := database.NewPostgreSQL()
	redisCache := cache.NewRedis()

	// 3. 初始化处理器
	urlHandler := handler.NewURLHandler(db, redisCache)

	// 4. 获取 NATS 地址（增强环境适应性）
	natsAddr := os.Getenv("NATS_URL")
	if natsAddr == "" {
		natsAddr = "nats://localhost:4222"
	}

	// 5. 创建 Micro Service
	service := micro.NewService(
		micro.Name("url.shortener.service"),
		micro.Version(opts.Version),
		// 显式指定地址
		micro.Transport(natsTransport.NewTransport(transport.Addrs(natsAddr))),
		micro.Registry(natsRegistry.NewRegistry(registry.Addrs(natsAddr))),
		micro.Broker(natsBroker.NewBroker()),

		// 【关键修复】：必须添加 WrapClient
		// 即使是服务端，内部调用 idGen 或发送其他 RPC 时，没有这个包装器 Trace 就会断
		micro.WrapClient(opentelemetry.NewClientWrapper()),

		// 服务端拦截器
		micro.WrapHandler(tracing.TraceGoMicroMiddleware("url.shortener.service")),
		micro.WrapHandler(metrics.GoMicroMiddleware("url-shortener")),
	)

	service.Client().Init(client.PoolSize(200))

	service.Init()

	// 6. 注册 RPC Handler
	err = pb.RegisterURLShortenerHandler(service.Server(), urlHandler)
	if err != nil {
		return nil, err
	}

	// 7. 启动监控服务器
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.HandlerFor(metricsRegistry.Registry, promhttp.HandlerOpts{}))
		opts.Log.Info("监控指标服务器启动于 :8001/metrics")
		if err := http.ListenAndServe(":8001", mux); err != nil {
			opts.Log.WithError(err).Error("监控服务器启动失败")
		}
	}()

	// 8. 定时更新数据库指标
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			stats := db.GetStats()
			if total, ok := stats["total_conns"].(int32); ok {
				metrics.UpdateDatabaseConnections("url-shortener", float64(total))
			}
		}
	}()

	opts.Log.Info("URL 短链接 RPC 服务已配置完成")

	return &Microservice{
		service: service,
		log:     opts.Log,
		tracer:  tracer,
		tp:      tp,
		db:      db,
		cache:   redisCache,
	}, nil
}

// Close 提供给 main.go 调用，确保退出前数据上报和资源释放
func (m *Microservice) Close() {
	m.log.Info("正在关闭微服务，释放资源...")

	// 1. 关闭追踪提供者 (Flush 剩余 Span)
	if m.tp != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := m.tp.Shutdown(ctx); err != nil {
			m.log.WithError(err).Error("Jaeger Shutdown 失败")
		}
	}

	// 2. 关闭数据库连接
	if m.db != nil {
		m.db.Close()
	}

	// 3. 关闭缓存连接
	if m.cache != nil {
		m.cache.Close()
	}
}

func (m *Microservice) Run() error {
	m.log.Info("正在启动集成 NATS 与可观测性支持的 URL 短链接微服务...")
	return m.service.Run()
}
