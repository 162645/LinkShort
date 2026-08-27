# URL Shortener - 性能测试使用说明

## 📖 概述

本项目提供了完整的性能测试工具,帮助你获取可写入简历的关键性能指标。

## 🚀 快速开始

### 1. 启动服务

```bash
# 方式 1: 使用 Docker (推荐)
make docker-up-all

# 方式 2: 手动启动
make setup-full && make run-all

# 等待服务完全启动
sleep 15
```

### 2. 运行性能测试

```bash
# 完整测试套件 (5-10 分钟)
bash scripts/perf/run_all_tests.sh

# 或者使用 Makefile (如果已添加性能测试目标)
make performance-test

# 快速测试 (1-2 分钟)
bash scripts/perf/test_ttfb.sh
bash scripts/perf/test_cache_hit_rate.sh
```

### 3. 查看测试报告

```bash
# 查看最新报告
cat performance_results/performance_report_*.md | tail -n 100

# 或者使用 less 查看
less performance_results/performance_report_*.md
```

## 📊 测试指标说明

### 1. TTFB (Time to First Byte)

**目标**: < 20ms

**测试命令**:
```bash
bash scripts/perf/test_ttfb.sh
```

**简历示例**:
> 通过 Redis 缓存优化,将短链接重定向的 TTFB 控制在 8ms

### 2. QPS (Queries Per Second)

**目标**: > 10,000 QPS

**前置条件**: 需要安装 `wrk` 或 `ab`

**测试命令**:
```bash
# 使用 wrk (推荐)
wrk -t12 -c400 -d30s --latency http://localhost:8080/test-link

# 使用 Apache Bench
ab -n 100000 -c 1000 http://localhost:8080/test-link
```

**简历示例**:
> 单机 QPS 达到 18,500,支持高并发访问场景

### 3. Redis 缓存命中率

**目标**: > 95%

**测试命令**:
```bash
bash scripts/perf/test_cache_hit_rate.sh
```

**简历示例**:
> 实现 Redis 缓存层,热点数据缓存命中率达到 98.7%

### 4. P99 延迟

**目标**: < 50ms

**测试命令**:
```bash
wrk -t12 -c400 -d60s --latency http://localhost:8080/test-link
```

**简历示例**:
> P99 延迟控制在 32ms 以内,确保 99% 用户获得极速响应

### 5. 短码冲突率

**目标**: < 0.1%

**测试说明**: 包含在完整测试套件中

**简历示例**:
> 优化短码生成算法,在 1000 万次生成中冲突率低于 0.05%

## 🛠️ 工具安装

### Windows

```powershell
# 使用 Chocolatey 安装 wrk
choco install wrk

# 或者使用 WSL
wsl --install
# 然后在 WSL 中: sudo apt-get install wrk
```

### Linux

```bash
# Ubuntu/Debian
sudo apt-get install wrk apache2-utils

# CentOS/RHEL
sudo yum install wrk httpd-tools
```

### macOS

```bash
brew install wrk
```

## 📈 从 Prometheus 获取指标

如果 Prometheus 正在运行 (http://localhost:9090):

```bash
# 平均响应时间
curl -s 'http://localhost:9090/api/v1/query?query=rate(http_request_duration_seconds_sum[5m])/rate(http_request_duration_seconds_count[5m])'

# QPS
curl -s 'http://localhost:9090/api/v1/query?query=rate(http_requests_total[1m])'

# P99 延迟
curl -s 'http://localhost:9090/api/v1/query?query=histogram_quantile(0.99,rate(http_request_duration_seconds_bucket[5m]))'
```

## 📝 完整简历示例

### 示例 1: 性能优化成果

```
URL 短链接服务性能优化 | Go, Redis, PostgreSQL
• 优化 URL 短链接服务性能,实现 Redis 缓存层,将热点链接的 TTFB 从 45ms 降低至 8ms (提升 82%)
• 通过 Go 微服务架构和并发优化,单机 QPS 达到 18,500,P99 延迟控制在 32ms 以内
• 实现智能短码生成算法,在 1000 万次生成中冲突率低于 0.05%
• Redis 缓存命中率达到 98.7%,显著降低数据库负载
```

### 示例 2: 系统架构设计

```
分布式短链接系统 | Go Micro, NATS, ClickHouse
• 设计并实现基于 Go Micro 的分布式短链接系统,支持 4 个微服务协同工作
• 集成 Prometheus + Grafana + Jaeger 完整可观测性栈,实现实时性能监控
• 通过压测验证系统在 400 并发下稳定运行,QPS 达 15,000+,P99 延迟 < 50ms
• 使用 PostgreSQL + Redis + ClickHouse 三层存储架构,优化读写性能
```

## 🔧 性能优化建议

如果测试结果不理想,可以尝试:

### 提升 QPS
- 增加 Redis 连接池大小
- 优化数据库索引
- 使用连接复用
- 增加 Go 的 GOMAXPROCS

### 降低延迟
- 启用 Redis 缓存预热
- 优化数据库查询
- 减少网络往返次数
- 使用本地缓存

### 提高缓存命中率
- 增加 Redis 内存
- 优化缓存过期策略
- 实现缓存预热机制

### 降低冲突率
- 增加短码长度
- 使用更好的哈希算法
- 实现冲突检测和重试机制

## 📚 相关文档

- [完整性能测试指南](../docs/PERFORMANCE_TESTING.md)
- [wrk 文档](https://github.com/wg/wrk)
- [Apache Bench 文档](https://httpd.apache.org/docs/2.4/programs/ab.html)
- [Prometheus 查询语法](https://prometheus.io/docs/prometheus/latest/querying/basics/)

## ❓ 常见问题

### Q: wrk 测试时出现连接错误?
A: 检查服务是否正常运行,尝试减少并发数 (-c 参数)

### Q: Redis 缓存命中率很低?
A: 确保访问的是相同的短链接,检查 Redis 是否正常运行

### Q: 如何解释测试结果?
A: 查看生成的报告文件,其中包含详细的指标说明和简历建议

### Q: 测试会影响生产数据吗?
A: 测试使用独立的测试用户 ID,不会影响真实数据

## 💡 提示

1. 首次测试前先预热系统 (创建一些测试数据)
2. 多次运行测试取平均值,结果更准确
3. 在相同环境下测试,便于对比优化效果
4. 保存测试报告,记录优化历程
