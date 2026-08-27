package main

import (
	"github.com/sirupsen/logrus"

	// Import NATS plugins

	"github.com/go-systems-lab/go-url-shortener/services/url-shortener-svc/microservice"
)

// Version may be changed during build via --ldflags parameter
var Version = "latest"

func main() {
	// 初始化日志实例
	logger := logrus.New()
	// 设置日志格式为 JSON，方便 ELK 等日志系统采集分析
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.Info("正在启动基于 NATS 的 URL 短链接 RPC 服务...")

	// 初始化微服务实例，传入版本号和日志句柄
	microService, err := microservice.Init(&microservice.ClientOptions{
		Version: Version,
		Log:     logger,
	})
	if err != nil {
		// 如果初始化失败，记录错误并强制退出程序
		logger.WithError(err).Fatal("微服务初始化失败")
	}

	// 运行微服务（这通常是一个阻塞调用，直到服务收到停止信号）
	if err := microService.Run(); err != nil {
		// 如果运行中出现异常，记录错误并退出
		logger.WithError(err).Fatal("微服务运行异常")
	}
}
