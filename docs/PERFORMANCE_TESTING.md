# URL Shortener 性能测试指南

## 📊 简历级性能指标测试方案

本指南将帮助你测试并获取以下可写入简历的关键性能指标：

| 指标维度 | 衡量参数 | 优化目标 |
|---------|---------|---------|
| **响应速度** | TTFB (Time to First Byte) | < 20ms |
| **吞吐量** | QPS (Queries Per Second) | 单机 > 10,000 QPS |
| **存储效率** | Redis Hit Rate | 接近 100% |
| **稳定性** | P99 延迟 | < 50ms |
| **冲突率** | Collision Rate | < 0.1% |

---

## 🚀 快速开始

### 前置条件

1. **安装压测工具**

```bash
# Windows (使用 Chocolatey)
choco install wrk

# 或者使用 WSL/Linux
sudo apt-get install wrk

# 安装 Apache Bench (备选)
# Windows: 下载 Apache HTTP Server
# Linux: sudo apt-get install apache2-utils
```

2. **确保服务运行**

```bash
# 启动所有服务
make setup-full && make run-all

# 或使用 Docker
make docker-up-all

# 等待服务完全启动
sleep 15
```

---

## 📈 性能测试脚本

### 1. TTFB (Time to First Byte) 测试

**目标**: < 20ms

```bash
# 使用 curl 测试单次 TTFB
curl -o /dev/null -s -w "TTFB: %{time_starttransfer}s\nTotal: %{time_total}s\n" \
  http://localhost:8080/health

# 测试短链接重定向的 TTFB (这是最关键的指标)
# 先创建一个短链接
SHORT_CODE=$(curl -s -X POST http://localhost:8080/api/v1/shorten \
  -H "Content-Type: application/json" \
  -d '{"long_url":"https://google.com","user_id":"perf_test"}' | jq -r '.short_code')

# 测试重定向 TTFB (多次测试取平均值)
for i in {1..100}; do
  curl -o /dev/null -s -w "%{time_starttransfer}\n" \
    http://localhost:8080/$SHORT_CODE
done | awk '{sum+=$1; count++} END {print "Average TTFB:", sum/count*1000, "ms"}'
```

**预期结果**: 
- 冷启动: 10-30ms
- 热缓存 (Redis): 5-15ms

---

### 2. QPS (Queries Per Second) 测试

**目标**: 单机 > 10,000 QPS

#### 方法 A: 使用 wrk (推荐)

```bash
# 测试重定向性能 (最常用场景)
# -t: 线程数, -c: 并发连接数, -d: 持续时间
wrk -t12 -c400 -d30s http://localhost:8080/$SHORT_CODE

# 测试创建短链接性能
wrk -t12 -c400 -d30s -s scripts/perf/create_url.lua http://localhost:8080/api/v1/shorten

# 测试健康检查端点 (基准性能)
wrk -t12 -c400 -d30s http://localhost:8080/health
```

#### 方法 B: 使用 Apache Bench

```bash
# 测试重定向 (10万请求, 1000并发)
ab -n 100000 -c 1000 http://localhost:8080/$SHORT_CODE

# 测试 API 端点
ab -n 50000 -c 500 -p scripts/perf/create_url.json -T application/json \
  http://localhost:8080/api/v1/shorten
```

**预期结果**:
- 重定向 (有 Redis 缓存): 15,000-25,000 QPS
- 创建短链接 (写操作): 5,000-10,000 QPS
- 健康检查: 30,000+ QPS

---

### 3. Redis Hit Rate (缓存命中率)

**目标**: 接近 100%

```bash
# 方法 1: 通过 Redis CLI 查看
redis-cli -h localhost -p 6379 INFO stats | grep keyspace

# 方法 2: 计算命中率
redis-cli -h localhost -p 6379 INFO stats | grep -E "keyspace_hits|keyspace_misses"

# 计算命中率公式:
# Hit Rate = keyspace_hits / (keyspace_hits + keyspace_misses) * 100%
```

**测试脚本**:

```bash
# 先清空 Redis 缓存
redis-cli -h localhost -p 6379 FLUSHDB

# 创建测试数据
for i in {1..100}; do
  curl -s -X POST http://localhost:8080/api/v1/shorten \
    -H "Content-Type: application/json" \
    -d "{\"long_url\":\"https://test-$i.com\",\"user_id\":\"perf_test\"}" > /dev/null
done

# 第一次访问 (缓存未命中)
redis-cli -h localhost -p 6379 INFO stats | grep keyspace_hits

# 重复访问相同链接 (应该命中缓存)
for i in {1..1000}; do
  curl -s http://localhost:8080/test-1 > /dev/null
done

# 再次查看命中率
redis-cli -h localhost -p 6379 INFO stats | grep -E "keyspace_hits|keyspace_misses"
```

**预期结果**:
- 首次访问后的缓存命中率: > 95%
- 热点数据缓存命中率: > 99%

---

### 4. P99 延迟测试

**目标**: P99 < 50ms

使用 wrk 的详细统计功能:

```bash
# 运行压测并保存详细结果
wrk -t12 -c400 -d60s --latency http://localhost:8080/$SHORT_CODE

# 输出会包含:
# - 平均延迟 (Avg)
# - 标准差 (Stdev)
# - 最大延迟 (Max)
# - P50, P75, P90, P99, P99.9 百分位延迟
```

**预期结果**:
```
Latency Distribution
  50%    8.00ms
  75%   12.00ms
  90%   18.00ms
  99%   35.00ms  ← 这是关键指标
```

---

### 5. 冲突率 (Collision Rate) 测试

**目标**: < 0.1%

```bash
# 批量创建短链接并检测冲突
TOTAL=10000
COLLISIONS=0

for i in $(seq 1 $TOTAL); do
  RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/shorten \
    -H "Content-Type: application/json" \
    -d "{\"long_url\":\"https://collision-test-$i.com\",\"user_id\":\"perf_test\"}")
  
  # 检查是否有错误或重试
  if echo "$RESPONSE" | grep -q "error\|retry\|conflict"; then
    ((COLLISIONS++))
  fi
  
  # 每1000次显示进度
  if [ $((i % 1000)) -eq 0 ]; then
    echo "Progress: $i/$TOTAL, Collisions: $COLLISIONS"
  fi
done

# 计算冲突率
COLLISION_RATE=$(echo "scale=4; $COLLISIONS / $TOTAL * 100" | bc)
echo "Collision Rate: $COLLISION_RATE%"
```

**预期结果**:
- 使用 6 字符短码 (62^6 = 56.8 billion): < 0.01%
- 使用 7 字符短码: < 0.001%

---

## 🎯 完整性能测试流程

### 一键运行所有测试

```bash
# 使用提供的自动化脚本
bash scripts/perf/run_all_tests.sh

# 或使用 Make 命令
make performance-test
```

### 手动完整测试流程

```bash
# 1. 启动服务
make restart-all
sleep 15

# 2. 预热系统 (创建测试数据)
bash scripts/perf/warmup.sh

# 3. 运行 TTFB 测试
bash scripts/perf/test_ttfb.sh

# 4. 运行 QPS 测试
bash scripts/perf/test_qps.sh

# 5. 检查 Redis 命中率
bash scripts/perf/test_cache_hit_rate.sh

# 6. 运行 P99 延迟测试
bash scripts/perf/test_latency.sh

# 7. 测试冲突率
bash scripts/perf/test_collision_rate.sh

# 8. 生成报告
bash scripts/perf/generate_report.sh
```

---

## 📊 从 Prometheus 获取指标

```bash
# 1. 平均响应时间
curl -s 'http://localhost:9090/api/v1/query?query=rate(http_request_duration_seconds_sum[5m])/rate(http_request_duration_seconds_count[5m])' | jq '.data.result[0].value[1]'

# 2. QPS
curl -s 'http://localhost:9090/api/v1/query?query=rate(http_requests_total[1m])' | jq '.data.result[0].value[1]'

# 3. P99 延迟
curl -s 'http://localhost:9090/api/v1/query?query=histogram_quantile(0.99,rate(http_request_duration_seconds_bucket[5m]))' | jq '.data.result[0].value[1]'

# 4. 错误率
curl -s 'http://localhost:9090/api/v1/query?query=rate(http_requests_total{status=~"5.."}[5m])/rate(http_requests_total[5m])' | jq '.data.result[0].value[1]'
```

---

## 📝 简历示例

基于测试结果,你可以这样写简历:

### 示例 1: 性能优化成果

```
• 优化 URL 短链接服务性能,实现 Redis 缓存层,将热点链接的 TTFB 从 45ms 降低至 8ms (提升 82%)
• 通过 Go 微服务架构和并发优化,单机 QPS 达到 18,500,P99 延迟控制在 32ms 以内
• 实现智能短码生成算法,在 1000 万次生成中冲突率低于 0.05%
• Redis 缓存命中率达到 98.7%,显著降低数据库负载
```

### 示例 2: 系统架构设计

```
• 设计并实现基于 Go Micro 的分布式短链接系统,支持 4 个微服务协同工作
• 集成 Prometheus + Grafana + Jaeger 完整可观测性栈,实现实时性能监控
• 通过压测验证系统在 400 并发下稳定运行,QPS 达 15,000+,P99 延迟 < 50ms
• 使用 PostgreSQL + Redis + ClickHouse 三层存储架构,优化读写性能
```

---

## 🔧 性能优化建议

如果测试结果不理想,可以尝试:

1. **提升 QPS**:
   - 增加 Redis 连接池大小
   - 优化数据库索引
   - 使用连接复用
   - 增加 Go 的 GOMAXPROCS

2. **降低延迟**:
   - 启用 Redis 缓存预热
   - 优化数据库查询
   - 减少网络往返次数
   - 使用本地缓存

3. **提高缓存命中率**:
   - 增加 Redis 内存
   - 优化缓存过期策略
   - 实现缓存预热机制

4. **降低冲突率**:
   - 增加短码长度
   - 使用更好的哈希算法
   - 实现冲突检测和重试机制

---

## 📚 相关资源

- [wrk 文档](https://github.com/wg/wrk)
- [Apache Bench 文档](https://httpd.apache.org/docs/2.4/programs/ab.html)
- [Prometheus 查询语法](https://prometheus.io/docs/prometheus/latest/querying/basics/)
- [Redis 性能优化](https://redis.io/docs/management/optimization/)
