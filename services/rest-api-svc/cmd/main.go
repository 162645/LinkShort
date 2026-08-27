package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go-micro.dev/v5"
	"go-micro.dev/v5/client"
	"go-micro.dev/v5/registry"
	"go-micro.dev/v5/transport"

	// Import NATS plugins
	natsBroker "github.com/micro/plugins/v5/broker/nats"
	natsRegistry "github.com/micro/plugins/v5/registry/nats"
	natsTransport "github.com/micro/plugins/v5/transport/nats"

	_ "github.com/go-systems-lab/go-url-shortener/services/rest-api-svc/docs" // Import generated docs
	"github.com/go-systems-lab/go-url-shortener/services/rest-api-svc/handler"
	"github.com/go-systems-lab/go-url-shortener/utils/metrics"
	"github.com/go-systems-lab/go-url-shortener/utils/tracing"
	"github.com/micro/plugins/v5/wrapper/trace/opentelemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
)

// Version 可在构建时通过 --ldflags 参数修改（例如注入 Git Commit ID）
var Version = "latest"

// HealthResponse 定义了健康检查接口返回的 JSON 响应格式
type HealthResponse struct {
	Status    string `json:"status" example:"ok"`            // 运行状态
	Service   string `json:"service" example:"rest-api-svc"` // 服务名称
	Transport string `json:"transport" example:"NATS"`       // 通信传输方式
	Version   string `json:"version" example:"1.0"`          // 当前版本号
}

// @title         URL 短链接 REST API
// @version    1.0
// @description 基于 Go Micro v5, NATS, PostgreSQL 和 Redis 构建的生产级短链接微服务。
// @description 此 REST API 为终端用户提供 HTTP 端点，用于与短链接服务进行交互。
//
// @contact.name    URL Shortener API 支持
// @contact.email   support@urlshortener.com
//
// @license.name    MIT
// @license.url https://opensource.org/licenses/MIT
//
// @host       192.168.0.51:8080
// @BasePath    /api/v1
func main() {
	// 1. 初始化日志记录器 (Logrus)
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.Info("正在启动 REST API 服务（集成 NATS、Swagger 文档及可观测性）...")

	// 2. 初始化分布式追踪 (Jaeger)
	tracingConfig := tracing.DefaultTracingConfig("rest.api.service")
	tp, err := tracing.InitJaeger(tracingConfig)
	if err != nil {
		logger.WithError(err).Warn("Jaeger 追踪初始化失败，服务将继续运行但无追踪功能")
	} else {
		logger.Info("Jaeger 追踪系统初始化成功")
		defer func() {
			if err := tp.Shutdown(context.Background()); err != nil {
				logger.WithError(err).Error("关闭追踪提供者失败")
			}
		}()
	}

	// 3. 初始化监控指标 (Prometheus)
	metricsRegistry := metrics.NewMetrics()

	// 4. 获取运行端口（环境变量 PORT，默认为 8080）
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// 5. 配置基于 NATS 插件的 Go Micro 客户端服务
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	// 5. 修改 service 初始化部分
	service := micro.NewService(
		micro.Name("rest.api.service"),
		micro.Version(Version),
		micro.Transport(natsTransport.NewTransport(transport.Addrs(natsURL))),
		micro.Registry(natsRegistry.NewRegistry(registry.Addrs(natsURL))),
		micro.Broker(natsBroker.NewBroker()),

		// 【必须添加这一行】
		// 它负责拦截 client 的每一次 RPC 调用，把 ctx 里的 Trace 信息注入到 NATS 消息中
		micro.WrapClient(opentelemetry.NewClientWrapper()),
	)

	// 【关键修复】：在此处增加 PoolSize，不要在 NewService 内部使用 micro.Client 覆盖 NATS 原生客户端
	service.Client().Init(client.PoolSize(200))

	// 6. 初始化并启动微服务后台协程（用于服务发现）
	service.Init()
	go func() {
		if err := service.Run(); err != nil {
			logger.WithError(err).Error("Go Micro 服务运行失败")
		}
	}()

	logger.Info("REST API 服务已配置 NATS 传输、注册中心及代理")

	// 7. 创建业务逻辑处理器
	urlHandler := handler.NewURLHandler(service)

	// 8. 设置 Gin 路由器 (使用 New 而不是 Default 来避免不必要的同步日志)
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	// 注册监控中间件
	router.Use(metrics.GinMiddleware("rest-api"))

	// 注册分布式追踪中间件（如果 Jaeger 可用）
	if tp != nil {
		logger.Info("🔍 正在注册分布式追踪中间件")
		router.Use(func(c *gin.Context) {
			// 从 HTTP Header 中提取追踪上下文
			ctx := otel.GetTextMapPropagator().Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))

			// 开启一个新的 HTTP Span
			tracer := tracing.NewTracer("rest.api.service")
			ctx, span := tracer.StartHTTPSpan(ctx, c.Request.Method, c.Request.URL.Path)

			defer span.End()

			// 添加 HTTP 标签属性
			tracing.AddAttributes(span,
				attribute.String("http.url", c.Request.URL.String()),
				attribute.String("http.user_agent", c.Request.UserAgent()),
				attribute.String("http.remote_addr", c.ClientIP()),
				attribute.String("service.name", "rest.api.service"),
			)

			// 将带有追踪信息的上下文传给后续 Handler
			c.Request = c.Request.WithContext(ctx)
			c.Next()

			// 记录响应状态并标记错误
			statusCode := c.Writer.Status()
			tracing.AddAttributes(span, attribute.Int("http.status_code", statusCode))

			if statusCode >= 400 {
				tracing.RecordError(span, fmt.Errorf("HTTP %d", statusCode))
			} else {
				tracing.RecordSuccess(span)
			}
		})
	} else {
		logger.Warn("🔍 追踪提供者为空，跳过追踪中间件配置")
	}

	// 9. 配置跨域 (CORS) 中间件
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// 10. 前端集成：提供现代化静态页面及资源服务
	router.StaticFile("/", "./frontend/index.html")
	router.StaticFile("/styles.css", "./frontend/styles.css")
	router.StaticFile("/app.js", "./frontend/app.js")

	// 监控及 Swagger 文档
	router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(metricsRegistry.Registry, promhttp.HandlerOpts{})))
	router.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// 13. 健康检查接口（附带 Swagger 注解以便文档生成）
	// @Summary      服务健康检查
	// @Description   检查 REST API 服务的运行健康状况
	// @Tags        Health
	// @Accept          json
	// @Produce      json
	// @Success      200    {object}   HealthResponse "服务运行正常"
	// @Router          /health [get]
	// 使用 Match 同时支持 GET 和 HEAD 方法
	router.Match([]string{"GET", "HEAD"}, "/health", func(c *gin.Context) {
		c.JSON(200, HealthResponse{
			Status:    "ok",
			Service:   "rest-api-svc",
			Transport: "NATS",
			Version:   Version,
		})
	})

	// 11. 注册业务路由分组 (v1)
	api := router.Group("/api/v1")
	{
		// URL 管理
		api.POST("/shorten", urlHandler.ShortenURL)
		api.GET("/urls/:shortCode", urlHandler.GetURLInfo)
		api.DELETE("/urls/:shortCode", urlHandler.DeleteURL)
		api.GET("/users/:userID/urls", urlHandler.GetUserURLs)

		// 数据分析
		api.GET("/analytics/urls/:shortCode", urlHandler.GetURLStats)
		api.GET("/analytics/top-urls", urlHandler.GetTopURLs)
		api.GET("/analytics/dashboard", urlHandler.GetDashboard)
	}

	// 12. 注册重定向路由（必须放在最后，以免干扰上述 API）
	router.GET("/:shortCode", urlHandler.RedirectURL)

	// 14. 记录启动成功信息并运行
	logger.WithFields(logrus.Fields{
		"port":    port,
		"swagger": "http://localhost:" + port + "/docs/index.html",
		"docs":    "http://localhost:" + port + "/",
	}).Info("REST API 服务及 Swagger 文档就绪")

	if err := router.Run(":" + port); err != nil {
		logger.WithError(err).Fatal("REST API 服务启动失败")
	}
}
