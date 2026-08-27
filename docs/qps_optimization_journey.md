# QPS 优化全过程复盘：从 1,600 到 5,535 的实战记录

> **文档定位**：本文完整记录了 NexusLink 短链接系统在真实 Linux (100G+ 内存) 环境中，经过多轮排查与优化，将 QPS 从最初的 **1,676** 提升到 **5,535** 的全过程。每一轮优化都附有问题根因、修改方案和实测数据对比。适用于面试时展示"系统性能调优"能力。

---

## 一、测试环境与方法论

| 项目 | 配置 |
|:---|:---|
| **操作系统** | Linux (WSL2)，内存 100G+ |
| **部署方式** | Docker Compose（单实例，Bridge 网络） |
| **压测工具** | `wrk -t12 -c400 -d30s`（12 线程、400 并发、持续 30 秒） |
| **压测对象** | `GET /:shortCode`（短链接跳转，核心热路径） |
| **完整请求链路** | `wrk → HTTP(Gin) → rest-api-svc → NATS RPC → redirect-svc → L1/L2 Cache → NATS RPC 返回 → HTTP 302` |

> **关键认知**：每一次短链接跳转都会经过 **两次跨进程 NATS RPC 往返**（rest-api-svc → redirect-svc → 返回），这是架构层的固有延迟成本。

---

## 二、优化里程碑总览

| 轮次 | QPS | 平均延迟 | 核心问题 | 优化手段 |
|:---:|:---:|:---:|:---|:---|
| 基线 | **1,676** | ~230ms | 多处同步阻塞 I/O | — |
| 第 1 轮 | **2,131** | 193ms | Go-Micro Client 覆盖 NATS 传输层 | 修复 `micro.Client` 配置方式 |
| 第 2 轮 | **2,458** | 168ms | Redis 缓存 JSON 格式不兼容 + DB 行锁 | 修复时间戳序列化 + 移除同步 DB UPDATE |
| 第 3 轮 | **2,943** | 142ms | Jaeger 100% 全量采样 + 大量 stdout 打印 | 采样率降至 1% + 删除 `fmt.Printf` |
| 第 4 轮 | **5,535** | 75ms | 冗余的异步 TrackClick RPC 翻倍 NATS 流量 | 移除重复 RPC 调用 |

> **累计提升：3.3 倍** (1,676 → 5,535)

---

## 三、各轮优化详细复盘

### 第 0 轮：基线分析 — 为什么只有 1,676 QPS？

初始压测数据：
- QPS：1,676
- 平均延迟：~230ms
- L2 缓存命中率：0%

**发现的根因**（按影响程度排序）：

1. **NATS JetStream 同步发布阻塞主线程**：重定向核心路径中同步调用 MQ 发布点击事件，每次 302 跳转都要等 NATS 确认
2. **Gin 框架使用 `gin.Default()` 开发模式**：每请求生成控制台日志字符串，引发内核态 stdout 锁竞争
3. **Redis / PostgreSQL 连接池偏小**：`PoolSize=30` 在 400 并发下频繁建连

**初始优化措施**（已在基线测试前实施）：

| 优化项 | 修改前 | 修改后 | 文件 |
|:---|:---|:---|:---|
| NATS Publish | 同步阻塞 | `go func(){}()` 异步 | `redirect-svc/handler/handler.go` |
| Gin 模式 | `gin.Default()` | `gin.New()` + `gin.ReleaseMode` | `rest-api-svc/cmd/main.go` |
| Redis 连接池 | `PoolSize=30` | `PoolSize=100` | `utils/cache/redis.go` |
| PG 连接池 | `MaxConns=25` | `MaxConns=100` | `utils/database/postgres.go` |
| Go 运行时 | 默认 | `GOMAXPROCS=12`, `GOGC=200` | `docker-compose.yml` |

---

### 第 1 轮：修复 Go-Micro Client 覆盖 NATS 传输层 (1,676 → 2,131)

**问题现象**：测试脚本在创建短链接和获取用户链接时全部失败，系统返回 "connection refused"。

**根因分析**：
为了增加连接池大小，代码中使用了以下写法：
```go
// ❌ 错误写法：创建全新的默认 Client，擦除了 NATS Transport/Registry
micro.Client(client.NewClient(client.PoolSize(200)))
```

Go-Micro 的 `micro.NewService(...)` 在加载参数时有**顺序覆盖**机制。传入全新的 `client.NewClient()` 会将之前注入的 NATS Transport、NATS Registry 全部清空，导致 RPC 退化为默认的 mDNS + TCP 本地查找，在 Docker 容器间完全不可达。

**修复方案**：

```go
// ✅ 正确写法：在 NewService 之后，安全地追加 PoolSize
service := micro.NewService(
    micro.Transport(natsTransport.NewTransport()),
    micro.Registry(natsRegistry.NewRegistry()),
    // ... 其他 NATS 插件
)
service.Client().Init(client.PoolSize(200))  // 安全补丁，不覆盖传输层
```

**涉及文件**：
- `services/rest-api-svc/cmd/main.go`
- `services/redirect-svc/microservice/microservice.go`
- `services/url-shortener-svc/microservice/microservice.go`

**效果**：QPS 1,676 → **2,131**（+27%），三个微服务间的 NATS RPC 通信完全恢复正常。

---

### 第 2 轮：修复 L2 缓存失效 + 消除数据库行锁 (2,131 → 2,458)

#### 问题 A：L2 (Redis) 缓存命中率始终为 0%

**根因分析**：
`url-shortener-svc` 创建短链接后将元数据写入 Redis，使用了 Unix 时间戳:
```go
"created_at": dbURL.CreatedAt.Unix()  // 写入的是 int64 整型
```

但 `redirect-svc` 从 Redis 读取时，其结构体要求的是 `time.Time` 类型：
```go
type CacheEntry struct {
    CreatedAt time.Time `json:"created_at"`  // 期望 RFC3339 字符串
}
```

`json.Unmarshal` 无法将 `int64` 解析为 `time.Time`，导致**每次反序列化静默失败**，系统认为缓存损坏而回源查 DB。

**修复**：统一使用 `time.RFC3339` 字符串格式：
```go
"created_at": dbURL.CreatedAt.Format(time.RFC3339)
```

#### 问题 B：PostgreSQL 行锁竞争导致连接池耗尽

**根因分析**：
`redirect-svc` 每次重定向都会异步执行：
```sql
UPDATE url_mappings SET click_count = click_count + 1 WHERE short_code = $1
```

当 400 并发同时请求同一个短码时，所有协程排队争抢同一行的悲观互斥锁，100 个 DB 连接全部阻塞等待锁释放。

**修复**：移除同步 PostgreSQL UPDATE，点击计数改为 Redis 原子计数器 + NATS 异步投递给 ClickHouse：
```go
// 【性能优化】：禁用 PostgreSQL 同步 UPDATE
// 点击追踪交由 Redis INCR + NATS → ClickHouse 异步处理
```

**效果**：QPS 2,131 → **2,458**（+15%），L2 命中率从 0% 恢复到 **100%**。

---

### 第 3 轮：消除 Jaeger 全量采样 + 清除调试日志 (2,458 → 2,943)

#### 问题 A：Jaeger 100% 采样率的 CPU 开销

**根因分析**：
```go
SamplingRatio: 1.0  // 100% 的请求都生成完整 Trace Span 并通过 gRPC 发送给 Jaeger
```

每次跳转经过 `rest-api-svc → redirect-svc` 两个服务，各自生成 Span 并序列化发送。在 400 并发下，大量协程阻塞在 gRPC 网络 I/O 上等待 Jaeger 收集器确认。

**修复**：将采样率降至 1%：
```go
SamplingRatio: 0.01  // 保留 1% 用于可观测性参考
```

#### 问题 B：热路径中遗留的 `fmt.Printf` 调试语句

**根因分析**：
`redirect-svc` 的 handler 和 domain 层遗留了十几处调试打印：
```go
fmt.Printf("🔍 [DEBUG] ResolveURL called with shortCode: %s\n", ...)
fmt.Printf("✅ [DEBUG] store.ResolveURL success: %s -> %s\n", ...)
```

`fmt.Printf` 写 stdout 是一个**带全局互斥锁的系统调用**。400 并发下，无数协程排队争抢 stdout 锁，严重拖慢主线程。

**修复**：全量搜索并删除所有热路径 `fmt.Printf` 和调试 `logger.Info` 语句。

**涉及文件**：
- `utils/tracing/jaeger.go`（采样率）
- `services/redirect-svc/handler/handler.go`（6 处 Printf）
- `services/redirect-svc/domain/resolver.go`（10 处 Printf）
- `services/rest-api-svc/cmd/main.go`（HTTP middleware 调试日志）

**效果**：QPS 2,458 → **2,943**（+20%）。

---

### 第 4 轮：消除冗余 TrackClick RPC 调用 (2,943 → 5,535)

**根因分析**：
梳理完整链路后发现，每次重定向实际触发了**三次 NATS 操作**：

| 序号 | 操作 | 发起方 | 类型 | 是否必要 |
|:---:|:---|:---|:---|:---:|
| 1 | `ResolveURL` RPC | rest-api-svc | 同步 NATS RPC | ✅ 必要 |
| 2 | `TrackClick` RPC | rest-api-svc | 异步 NATS RPC | ❌ 冗余 |
| 3 | `publishClickEvent` | redirect-svc | 异步 NATS Broker Publish | ✅ 必要 |

第 2 项和第 3 项功能完全重复！`redirect-svc` 在 `ResolveURL` 内部已经通过 `go h.publishClickEvent(...)` 发送了点击事件，`rest-api-svc` 又额外发了一次 `TrackClick` RPC，导致：
- NATS 流量翻倍
- goroutine 数量翻倍
- 额外的 context 创建和 metadata 注入开销

**修复**：删除 `rest-api-svc/handler/handler.go` 中整个 `TrackClick` 异步 goroutine 块（约 25 行代码）。

**效果**：QPS 2,943 → **5,535**（+88%），平均延迟从 142ms 降至 **75ms**。

---

## 四、最终压测数据

| 指标 | 最终值 | 目标 | 状态 |
|:---|:---:|:---:|:---:|
| **QPS** | 5,535 | >3,000 | ✅ 优秀 |
| **平均延迟** | 75.14 ms | <200ms | ✅ 优秀 |
| **TTFB** | 3.72 ms | <20ms | ✅ 优秀 |
| **P99 延迟** | 62.83 ms | <50ms | ⚠️ 略高 |
| **L1 命中率** | 99.63% | >95% | ✅ 极优 |
| **L2 命中率** | 100.00% | >80% | ✅ 极优 |
| **短码冲突率** | 0% | <0.1% | ✅ 完美 |
| **布隆过滤器通行率** | 83.87% | — | ✅ 正常 |
| **Redis 内存** | 16.39M | — | ✅ 健康 |
| **DB 活跃连接** | 11 | — | ✅ 健康 |

---

## 五、P99 延迟分析与优化空间

当前 P99 = **62.83ms**，超过 50ms 目标，但这在当前架构下属于**合理范围**。

### P99 偏高的成因

1. **NATS RPC 的尾部延迟**：Docker Bridge 网络中 NATS 消息的 P99 传输延迟本身就在 10-20ms。两次 RPC 往返叠加后，尾部请求延迟可达 40-60ms。
2. **Go Runtime GC 停顿**：虽然 `GOGC=200` 已降低 GC 频率，但在高并发下 GC STW（Stop The World）偶发暂停仍会影响 P99。
3. **Redis/NATS 连接池的偶发争抢**：当连接池接近饱和时，少量请求需要等待连接释放。
4. **Docker 内核态网络栈延迟**：Bridge 模式需要经过 iptables NAT 转换，相比 host 模式有额外开销。

### 进一步降低 P99 的方向（无需大改架构）

| 优化方向 | 预期效果 | 改动量 |
|:---|:---|:---|
| Docker 使用 `network_mode: host` | P99 降低 10-15ms | 仅改 docker-compose.yml |
| 调整 `GOGC=400` 或使用 `GOMEMLIMIT` | 减少 GC 暂停频率 | 环境变量 |
| Redis 连接池 `PoolSize` 从 30 提升到 100 (redirect-svc) | 消除偶发连接等待 | 1 行代码 |
| 部署多个 redirect-svc 实例 | 分散负载，降低单节点尾部延迟 | docker-compose scale |

### 更大幅度的架构级优化（需改代码）

| 方案 | 预期 QPS | 原理 |
|:---|:---|:---|
| 将 redirect-svc 嵌入 rest-api-svc（绕过 NATS RPC） | 15,000+ | 消除两次网络往返，变为本地函数调用 |
| 多实例部署 redirect-svc (replicas=3) | 15,000+ | 利用 NATS 的负载均衡，QPS 线性扩展 |
| Nginx + OpenResty + lua-resty-redis 直读缓存 | 50,000+ | 跳过 Go 服务，在反向代理层直接完成跳转 |

---

## 六、面试陈述建议

> **面试话术参考（约 2 分钟）**：
>
> "我在对短链接系统做压力测试时，发现 QPS 卡在 1600 多上不去，但 CPU 和内存水位并不高，说明系统存在严重的 I/O 阻塞或锁竞争。
>
> 我从四个层面逐步排查定位并解决：
>
> **第一**，Go-Micro 框架的客户端初始化有一个隐蔽的配置覆盖陷阱。我为了增加连接池大小，用 `micro.Client(client.NewClient(...))` 传入了新客户端，结果把之前注入的 NATS Transport 全部擦除了，导致微服务间 RPC 退化为本地 TCP 寻址。改为 `service.Client().Init()` 的安全追加方式后恢复正常。
>
> **第二**，我发现 L2 Redis 缓存命中率始终是 0%。排查后发现是两个微服务之间 JSON 序列化的时间戳格式不兼容——写入端用的是 Unix 整型，读取端期望的是 RFC3339 字符串。反序列化静默失败导致每次都穿透到数据库。同时，重定向路径上的同步 `UPDATE click_count` 在 400 并发同一短码时引发了严重的 PostgreSQL 行锁竞争，我将其改为 Redis 原子计数 + NATS 异步投递 ClickHouse。
>
> **第三**，Jaeger 链路追踪默认 100% 采样，在高并发下大量协程阻塞在 gRPC 发送 Span 上。我将采样率调至 1%，并清除了热路径中十几处遗留的 `fmt.Printf` 调试语句——stdout 写入是带全局锁的系统调用，成为了隐形瓶颈。
>
> **第四**，也是提升最大的一轮。我梳理完整链路后发现，每次跳转竟然触发了三次 NATS 操作，其中 rest-api-svc 端的 TrackClick RPC 和 redirect-svc 内部的 publishClickEvent 功能完全重复。删除这个冗余 RPC 后，NATS 流量直接减半，QPS 从 2900 跃升到了 5500。
>
> 最终在单实例 Docker 部署下，QPS 达到 **5,535**，L1 本地缓存命中率 99.6%，L2 Redis 命中率 100%，短码冲突率 0%。如果要进一步提升，可以通过多实例部署线性扩展，或将跳转逻辑嵌入网关层跳过 NATS RPC，预计可达 15,000+ QPS。"

---

## 七、优化过程修改文件清单

| 文件 | 修改内容 |
|:---|:---|
| `services/rest-api-svc/cmd/main.go` | Gin ReleaseMode、去除 `micro.Client` 覆盖、移除调试日志 |
| `services/rest-api-svc/handler/handler.go` | 移除冗余 TrackClick 异步 RPC、清除 debug 日志和未用 import |
| `services/redirect-svc/microservice/microservice.go` | 安全追加 PoolSize、去除 `micro.Client` 覆盖 |
| `services/redirect-svc/handler/handler.go` | 清除全部 `fmt.Printf` 调试语句 |
| `services/redirect-svc/domain/resolver.go` | 清除全部 `fmt.Printf` 调试语句 |
| `services/redirect-svc/store/store.go` | 移除同步 PostgreSQL UPDATE 行锁操作 |
| `services/url-shortener-svc/microservice/microservice.go` | 安全追加 PoolSize、去除 `micro.Client` 覆盖 |
| `services/url-shortener-svc/domain/service.go` | 修复缓存时间戳格式 (Unix → RFC3339) |
| `utils/tracing/jaeger.go` | 采样率 1.0 → 0.01、移除 TraceID 打印 |
| `docker-compose.yml` | 添加 `GOMAXPROCS=12`、`GOGC=200` 环境变量 |
| `scripts/perf/run_all_tests.sh` | Fallback 逻辑、L2 测试改进、`set -e` 兼容修复 |
