# LinkShort｜高并发短链接系统

LinkShort 是一个基于 Go-Micro 微服务架构的短链接平台，支持短链接生成、短码自定义、低延迟跳转、点击统计和可观测性。

## 产品截图

### 用户端首页

![LinkShort 用户端首页](images/linkshort-home.png)

### 短链接创建成功

![短链接创建成功](images/linkshort-created.png)

### 自定义短码

![自定义短码](images/linkshort-custom-alias.png)

## 解决的问题

- 长链接不便于分享、传播和统计；
- 高并发跳转场景下，需要稳定、低延迟的访问链路；
- 链接创建、跳转和点击分析需要解耦，避免统计写入影响主链路；
- 运营人员需要按用户、时间和链接查看真实点击数据。

## 核心功能

- 短链接生成与 Base62 短码编码；
- 自定义短码和链接有效期；
- 用户维度的链接查询与管理；
- 短链接跳转和点击事件采集；
- Redis 多级缓存、Bloom Filter 和 Singleflight 防穿透/击穿；
- ClickHouse 实时点击分析；
- Prometheus 指标、Grafana 看板和 Jaeger 分布式追踪；
- 管理员接口鉴权、健康检查和容器化部署。

## 系统架构

```text
客户端
  │
  ▼
REST API 网关 ──► URL 服务 ──► PostgreSQL
  │                  │
  │                  └────► Redis 缓存
  │
  └─跳转请求────► Redirect 服务 ──► NATS 事件流 ──► Analytics 服务 ──► ClickHouse
                                                        │
                                                        └─ Prometheus / Grafana / Jaeger
```

## 服务划分

| 服务 | 职责 |
| --- | --- |
| REST API 服务 | 提供 HTTP API、鉴权和接口文档 |
| URL Shortener 服务 | 创建短链接、校验短码和管理链接 |
| Redirect 服务 | 高性能短链接解析与跳转 |
| Analytics 服务 | 消费点击事件并写入 ClickHouse |
| Redis | 热点缓存、计数器和 Bloom Filter |
| PostgreSQL | 链接和用户信息的持久化存储 |
| NATS JetStream | 服务发现、RPC 传输和可靠事件流 |
| ClickHouse | 点击明细和聚合分析 |

## 技术栈

Go 1.23+、Go-Micro v5、Gin、Protocol Buffers、NATS JetStream、Redis、PostgreSQL、ClickHouse、Docker Compose、Prometheus、Grafana、Jaeger。

## 快速启动

### 环境要求

- Go 1.23 或更高版本；
- Docker 和 Docker Compose；
- Make；
- Protocol Buffers 编译器（生成代码时需要）。

### 启动服务

```bash
cp .env.example .env
make setup
make run-all
```

或者使用 Docker Compose：

```bash
docker compose up -d --build
```

## 常用地址

| 服务 | 本地地址 |
| --- | --- |
| REST API | http://localhost:8080 |
| 健康检查 | http://localhost:8080/health |
| Swagger 文档 | http://localhost:8080/docs/index.html |
| Prometheus | http://localhost:9090 |
| Grafana | http://localhost:3000 |
| Jaeger | http://localhost:16686 |

## 常用命令

```bash
make deps              # 安装依赖
make proto             # 生成 Protocol Buffers 代码
make build-all         # 构建全部服务
make dev-up            # 启动基础设施
make health            # 健康检查
make test-api          # API 测试
make start-monitoring  # 启动监控组件
```

## 目录结构

```text
├── services/           # URL、Redirect、Analytics、REST API 服务
├── proto/              # Protocol Buffers 定义
├── utils/              # 缓存、ID 生成、数据库、指标和链路追踪工具
├── database/           # 数据库迁移与初始化脚本
├── frontend/           # 用户端和管理端页面
├── infrastructure/     # Docker、监控和基础设施配置
├── scripts/            # 部署、运维和测试脚本
├── docs/               # 设计、部署、性能和运维文档
└── images/             # README 使用的产品截图
```

## 安全说明

请勿提交 `.env`、数据库密码、API Token、证书和生产配置。生产环境建议只开放网关端口，Redis、PostgreSQL、NATS 和 ClickHouse 通过 Docker 内网访问。

## 许可证

MIT
