#!/bin/bash

# URL Shortener Performance Testing Suite
# 完整的性能测试自动化脚本

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 配置
API_BASE="http://localhost:8080"
REDIS_HOST="localhost"
REDIS_PORT="6379"
REDIS_PASSWORD="${REDIS_PASSWORD:-redispassword}"  # 默认密码
PROMETHEUS_URL="http://localhost:9090"
TEST_USER="perf_test_user"
RESULTS_DIR="./performance_results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# 创建结果目录
mkdir -p "$RESULTS_DIR"
REPORT_FILE="$RESULTS_DIR/performance_report_$TIMESTAMP.md"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  URL Shortener Performance Test Suite${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# 检查服务是否运行
check_services() {
    echo -e "${YELLOW}[1/7] 检查服务状态...${NC}"
    
    if ! curl -s "$API_BASE/health" > /dev/null 2>&1; then
        echo -e "${RED}❌ REST API 服务未运行!${NC}"
        echo "请先启动服务: make run-all 或 make docker-up-all"
        exit 1
    fi
    
    # 检查 Redis - 尝试多种方式
    REDIS_OK=false
    
    # 方式 1: 使用 redis-cli ping (带密码)
    if redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -a "$REDIS_PASSWORD" ping 2>/dev/null | grep -q "PONG"; then
        REDIS_OK=true
    # 方式 2: 使用 Docker exec
    elif docker exec url-shortener-redis redis-cli -a "$REDIS_PASSWORD" ping 2>/dev/null | grep -q "PONG"; then
        REDIS_OK=true
    # 方式 3: 使用 telnet 测试端口
    elif timeout 2 bash -c "echo -e '\n' | nc -w 1 $REDIS_HOST $REDIS_PORT" > /dev/null 2>&1; then
        REDIS_OK=true
    # 方式 4: 检查 Docker 容器
    elif docker ps 2>/dev/null | grep -q redis; then
        REDIS_OK=true
    fi
    
    if [ "$REDIS_OK" = false ]; then
        echo -e "${RED}❌ Redis 服务未运行!${NC}"
        echo "请先启动 Redis: docker-compose up -d redis"
        exit 1
    fi
    
    echo -e "${GREEN}✅ 所有服务正常运行${NC}"
    echo ""
}

# 预热系统
warmup_system() {
    echo -e "${YELLOW}[2/7] 系统预热 - 创建测试数据...${NC}"
    
    # 创建 100 个测试短链接
    for i in {1..100}; do
        curl -s -X POST "$API_BASE/api/v1/shorten" \
            -H "Content-Type: application/json" \
            -d "{\"long_url\":\"https://warmup-test-$i.com\",\"user_id\":\"$TEST_USER\"}" > /dev/null
        
        if [ $((i % 20)) -eq 0 ]; then
            echo "  创建进度: $i/100"
        fi
    done
    
    # 访问一次以填充缓存
    for i in {1..50}; do
        SHORT_CODE=$(curl -s "$API_BASE/api/v1/users/$TEST_USER/urls" | jq -r ".urls[$i].short_code // empty")
        if [ -n "$SHORT_CODE" ]; then
            curl -s "$API_BASE/$SHORT_CODE" > /dev/null
        fi
    done
    
    echo -e "${GREEN}✅ 预热完成${NC}"
    echo ""
}

# 测试 TTFB
test_ttfb() {
    echo -e "${YELLOW}[3/7] 测试 TTFB (Time to First Byte)...${NC}"
    
    # 创建一个测试短链接
    SHORT_CODE=$(curl -s -X POST "$API_BASE/api/v1/shorten" \
        -H "Content-Type: application/json" \
        -d "{\"long_url\":\"https://ttfb-test.com\",\"user_id\":\"$TEST_USER\",\"custom_alias\":\"ttfb-test\"}" | jq -r '.short_code')
    
    # 测试 100 次并计算平均值
    echo "  执行 100 次 TTFB 测试..."
    TTFB_SUM=0
    for i in {1..100}; do
        TTFB=$(curl -o /dev/null -s -w "%{time_starttransfer}" "$API_BASE/$SHORT_CODE")
        TTFB_MS=$(echo "$TTFB * 1000" | bc)
        TTFB_SUM=$(echo "$TTFB_SUM + $TTFB_MS" | bc)
    done
    
    AVG_TTFB=$(echo "scale=2; $TTFB_SUM / 100" | bc)
    
    echo -e "  ${GREEN}平均 TTFB: ${AVG_TTFB} ms${NC}"
    
    # 写入报告
    echo "### TTFB (Time to First Byte)" >> "$REPORT_FILE"
    echo "- **平均值**: ${AVG_TTFB} ms" >> "$REPORT_FILE"
    echo "- **目标**: < 20 ms" >> "$REPORT_FILE"
    
    if (( $(echo "$AVG_TTFB < 20" | bc -l) )); then
        echo -e "- **结果**: ✅ 通过" >> "$REPORT_FILE"
        echo -e "${GREEN}✅ TTFB 测试通过 (< 20ms)${NC}"
    else
        echo -e "- **结果**: ⚠️ 需要优化" >> "$REPORT_FILE"
        echo -e "${YELLOW}⚠️ TTFB 需要优化 (目标 < 20ms)${NC}"
    fi
    echo "" >> "$REPORT_FILE"
    echo ""
}

# 测试 QPS (需要 wrk 或 ab)
test_qps() {
    echo -e "${YELLOW}[4/7] 测试 QPS (Queries Per Second)...${NC}"
    
    # 检查是否安装了 wrk
    if command -v wrk &> /dev/null; then
        echo "  使用 wrk 进行压测 (30秒, 12线程, 400并发)..."
        
        # 获取一个短链接用于测试
        local URLS_JSON=$(curl -s "$API_BASE/api/v1/users/$TEST_USER/urls")
        SHORT_CODE=$(echo "$URLS_JSON" | jq -r '.urls[0].short_code // empty')
        
        if [ -z "$SHORT_CODE" ] || [ "$SHORT_CODE" == "null" ]; then
            echo -e "${YELLOW}  ⚠️ 无法从列表获取测试链接，尝试新建 fallback 链接...${NC}"
            SHORT_CODE=$(curl -s -X POST "$API_BASE/api/v1/shorten" \
                -H "Content-Type: application/json" \
                -d "{\"long_url\":\"https://fallback-qps-test.com\",\"user_id\":\"$TEST_USER\"}" | jq -r '.short_code')
        fi
        
        # 运行 wrk
        WRK_OUTPUT=$(wrk -t12 -c400 -d30s --latency "$API_BASE/$SHORT_CODE" 2>&1)
        
        # 提取 QPS
        QPS=$(echo "$WRK_OUTPUT" | grep "Requests/sec:" | awk '{print $2}')
        AVG_LATENCY=$(echo "$WRK_OUTPUT" | grep "Latency" | head -1 | awk '{print $2}')
        
        echo -e "  ${GREEN}QPS: ${QPS}${NC}"
        echo -e "  ${GREEN}平均延迟: ${AVG_LATENCY}${NC}"
        
        # 写入报告
        echo "### QPS (Queries Per Second)" >> "$REPORT_FILE"
        echo "- **测试工具**: wrk" >> "$REPORT_FILE"
        echo "- **QPS**: ${QPS}" >> "$REPORT_FILE"
        echo "- **平均延迟**: ${AVG_LATENCY}" >> "$REPORT_FILE"
        echo "- **目标**: > 10,000 QPS" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"
        
        echo -e "${GREEN}✅ QPS 测试完成${NC}"
    elif command -v ab &> /dev/null; then
        echo "  使用 Apache Bench 进行压测..."
        
        SHORT_CODE=$(curl -s "$API_BASE/api/v1/users/$TEST_USER/urls" | jq -r '.urls[0].short_code')
        
        AB_OUTPUT=$(ab -n 50000 -c 500 "$API_BASE/$SHORT_CODE" 2>&1)
        
        QPS=$(echo "$AB_OUTPUT" | grep "Requests per second:" | awk '{print $4}')
        
        echo -e "  ${GREEN}QPS: ${QPS}${NC}"
        
        echo "### QPS (Queries Per Second)" >> "$REPORT_FILE"
        echo "- **测试工具**: Apache Bench" >> "$REPORT_FILE"
        echo "- **QPS**: ${QPS}" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"
        
        echo -e "${GREEN}✅ QPS 测试完成${NC}"
    else
        echo -e "${YELLOW}⚠️ 未安装 wrk 或 ab,跳过 QPS 测试${NC}"
        echo "  安装方法:"
        echo "  - wrk: https://github.com/wg/wrk"
        echo "  - ab: sudo apt-get install apache2-utils"
        
        echo "### QPS (Queries Per Second)" >> "$REPORT_FILE"
        echo "- **状态**: 跳过 (未安装压测工具)" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"
    fi
    echo ""
}

# 测试缓存命中率 (L1 & L2)
test_cache_hit_rate() {
    echo -e "${YELLOW}[5/7] 测试缓存层命中情况 (L1 & L2)...${NC}"
    echo -e "${BLUE}注: 本系统正在执行 2000 次压力测试以触发多级缓存机制...${NC}"
    
    # 获取测试 URL
    local TEST_URL_JSON=$(curl -s "$API_BASE/api/v1/users/$TEST_USER/urls")
    local SHORT_CODE=$(echo "$TEST_URL_JSON" | jq -r '.urls[0].short_code // empty')
    
    if [ -z "$SHORT_CODE" ] || [ "$SHORT_CODE" == "null" ]; then
        echo -e "${YELLOW}  ⚠️ 未能从用户列表获取测试链接，新建 fallback 链接进行缓存压测...${NC}"
        SHORT_CODE=$(curl -s -X POST "$API_BASE/api/v1/shorten" \
            -H "Content-Type: application/json" \
            -d "{\"long_url\":\"https://fallback-cache-test.com\",\"user_id\":\"$TEST_USER\"}" | jq -r '.short_code // empty')
            
        if [ -z "$SHORT_CODE" ] || [ "$SHORT_CODE" == "null" ]; then
            echo -e "${RED}❌ 依然未能创建测试短链接，跳过缓存测试${NC}"
            return
        fi
    fi

    # 执行 2000 次访问以确保 L1 预热并产生足够指标
    for i in {1..2000}; do
        curl -s "$API_BASE/$SHORT_CODE" > /dev/null
    done

    # --- 新增: 冷启动场景测试 (专门测试 L2) ---
    echo "  L2 压力测试 (访问 500 个新链接以产生 L2 流量)..."
    for i in {1..500}; do
        local NEW_CODE=$(curl -s -X POST "$API_BASE/api/v1/shorten" \
            -H "Content-Type: application/json" \
            -d "{\"long_url\":\"https://l2-test-$i-$(date +%s).com\",\"user_id\":\"l2_user\"}" | jq -r '.short_code')
        # 增加微小延迟，确保布隆过滤器写入完成
        sleep 0.05
        # 第一次访问：L1 Miss, Database Hit, L1/L2 Write
        # (因为直接读取会穿透 L1 和 L2)
        curl -s "$API_BASE/$NEW_CODE" > /dev/null
        # 第二次访问：由于刚才的预热，这会在微服务层命中 L1 或 L2
        curl -s "$API_BASE/$NEW_CODE" > /dev/null
    done

    echo "  正在请求 Prometheus 指标 (等待抓取周期)..."
    sleep 35 # 进一步增加等待时间，确保 Prometheus 抓取到最新的 Counter 值
    
    # 检查重定向服务指标端口是否连通
    if ! curl -s --connect-timeout 2 "http://localhost:9091/metrics" > /dev/null; then
        echo -e "${RED}⚠️ 警告: 无法连接到 redirect-service 指标端口 (9091)，缓存统计可能不准${NC}"
    fi

    # 这里的查询逻辑：使用 URI 编码并确保 sum 处理正确
    local Q_L1_HITS=$(echo -n 'sum(cache_hits_total{layer="l1",result="hit"})' | jq -sRr @uri)
    local Q_L1_MISS=$(echo -n 'sum(cache_hits_total{layer="l1",result=~"miss.*"})' | jq -sRr @uri)
    local Q_L2_HITS=$(echo -n 'sum(cache_hits_total{layer="l2",result="hit"})' | jq -sRr @uri)
    local Q_L2_MISS=$(echo -n 'sum(cache_hits_total{layer="l2",result=~"miss.*"})' | jq -sRr @uri)

    local L1_HITS_RAW=$(curl -s "$PROMETHEUS_URL/api/v1/query?query=$Q_L1_HITS")
    local L1_MISSES_RAW=$(curl -s "$PROMETHEUS_URL/api/v1/query?query=$Q_L1_MISS")
    local L2_HITS_RAW=$(curl -s "$PROMETHEUS_URL/api/v1/query?query=$Q_L2_HITS")
    local L2_MISSES_RAW=$(curl -s "$PROMETHEUS_URL/api/v1/query?query=$Q_L2_MISS")

    # 使用 awk 鲁棒地提取并求和所有返回的值
    local L1_HITS=$(echo "$L1_HITS_RAW" | jq -r '.data.result[].value[1] // empty' | awk '{sum+=$1} END {print sum+0}')
    local L1_MISSES=$(echo "$L1_MISSES_RAW" | jq -r '.data.result[].value[1] // empty' | awk '{sum+=$1} END {print sum+0}')
    local L2_HITS=$(echo "$L2_HITS_RAW" | jq -r '.data.result[].value[1] // empty' | awk '{sum+=$1} END {print sum+0}')
    local L2_MISSES=$(echo "$L2_MISSES_RAW" | jq -r '.data.result[].value[1] // empty' | awk '{sum+=$1} END {print sum+0}')

    # --- 新增: 布隆过滤器指标 ---
    local Q_BLOOM_HITS=$(echo -n 'sum(cache_hits_total{layer="bloom",result="hit"})' | jq -sRr @uri)
    local Q_BLOOM_MISSES=$(echo -n 'sum(cache_hits_total{layer="bloom",result="miss"})' | jq -sRr @uri)

    local BLOOM_HITS_RAW=$(curl -s "$PROMETHEUS_URL/api/v1/query?query=$Q_BLOOM_HITS")
    local BLOOM_MISSES_RAW=$(curl -s "$PROMETHEUS_URL/api/v1/query?query=$Q_BLOOM_MISSES")
    local BLOOM_HITS=$(echo "$BLOOM_HITS_RAW" | jq -r '.data.result[].value[1] // empty' | awk '{sum+=$1} END {print sum+0}')
    local BLOOM_MISSES=$(echo "$BLOOM_MISSES_RAW" | jq -r '.data.result[].value[1] // empty' | awk '{sum+=$1} END {print sum+0}')

    # 计算 L1 命中率
    local L1_TOTAL=$(echo "$L1_HITS + $L1_MISSES" | bc)
    local L1_RATE="0"
    if [ "$L1_TOTAL" -gt 0 ]; then
        L1_RATE=$(echo "scale=2; $L1_HITS * 100 / $L1_TOTAL" | bc)
    fi

    # 计算 L2 命中率
    local L2_TOTAL=$(echo "$L2_HITS + $L2_MISSES" | bc)
    local L2_RATE="0"
    if [ "$L2_TOTAL" -gt 0 ]; then
        L2_RATE=$(echo "scale=2; $L2_HITS * 100 / $L2_TOTAL" | bc)
    fi

    # 计算布隆过滤器通行率
    local BLOOM_TOTAL=$(echo "$BLOOM_HITS + $BLOOM_MISSES" | bc)
    local BLOOM_RATE="0"
    if [ "$BLOOM_TOTAL" -gt 0 ]; then
        BLOOM_RATE=$(echo "scale=2; $BLOOM_HITS * 100 / $BLOOM_TOTAL" | bc)
    fi

    echo -e "  ${GREEN}L1 (本地内存) 命中率: ${L1_RATE}%${NC} (Hits: $L1_HITS / Total: $L1_TOTAL)"
    echo -e "  ${GREEN}L2 (Redis 缓存) 命中率: ${L2_RATE}%${NC} (Hits: $L2_HITS / Total: $L2_TOTAL)"
    echo -e "  ${GREEN}布隆过滤器 通行率: ${BLOOM_RATE}%${NC} (Pass: $BLOOM_HITS / Intercept: $BLOOM_MISSES)"
    echo -e "${BLUE}💡 提示: 绝大部分热点请求应被 L1 拦截。L2 仅处理 L1 未命中的穿透请求。布隆过滤器拦截非法请求。${NC}"

    # 写入报告
    echo "### 缓存层性能 (Multi-layer Cache)" >> "$REPORT_FILE"
    echo "- **L1 (Local) 命中率**: ${L1_RATE}%" >> "$REPORT_FILE"
    echo "- **L2 (Redis) 命中率**: ${L2_RATE}%" >> "$REPORT_FILE"
    echo "- **布隆过滤器记录**: 通行 $BLOOM_HITS / 拦截 $BLOOM_MISSES (通行率: ${BLOOM_RATE}%)" >> "$REPORT_FILE"
    echo "- **详细说明**: 极热点请求被 L1 本地缓存拦截，只有 L1 未命中的请求（如首次访问或本地缓存过期）才会到达 L2 Redis。布隆过滤器在最前端拦截无效短码请求，减少后端压力。" >> "$REPORT_FILE"
    
    if (( $(echo "$L1_RATE > 50" | bc -l) )); then
        echo -e "- **结果**: ✅ 合格 (L1 有效拦截)" >> "$REPORT_FILE"
    else
        echo -e "- **结果**: ⚠️ 提示 (L1 命中率较低，可能由于测试数据分散)" >> "$REPORT_FILE"
    fi
    echo "" >> "$REPORT_FILE"
}

# 测试冲突率
test_collision_rate() {
    echo -e "${YELLOW}[6/7] 测试短码冲突率...${NC}"
    echo "  创建 1000 个短链接并检测冲突..."
    
    TOTAL=1000
    COLLISIONS=0
    ERRORS=0
    
    for i in $(seq 1 $TOTAL); do
        RESPONSE=$(curl -s -X POST "$API_BASE/api/v1/shorten" \
            -H "Content-Type: application/json" \
            -d "{\"long_url\":\"https://collision-test-$i-$(date +%s).com\",\"user_id\":\"collision_test\"}")
        
        # 检查是否有错误
        if echo "$RESPONSE" | jq -e '.error' > /dev/null 2>&1; then
            ERROR_MSG=$(echo "$RESPONSE" | jq -r '.error')
            if echo "$ERROR_MSG" | grep -qi "conflict\|collision\|duplicate"; then
                COLLISIONS=$((COLLISIONS + 1))
            else
                ERRORS=$((ERRORS + 1))
            fi
        fi
        
        # 显示进度
        if [ $((i % 200)) -eq 0 ]; then
            echo "  进度: $i/$TOTAL, 冲突: $COLLISIONS, 错误: $ERRORS"
        fi
    done
    
    COLLISION_RATE=$(echo "scale=4; $COLLISIONS * 100 / $TOTAL" | bc)
    
    echo -e "  ${GREEN}冲突率: ${COLLISION_RATE}%${NC}"
    echo -e "  总请求: $TOTAL, 冲突: $COLLISIONS, 其他错误: $ERRORS"
    
    # 写入报告
    echo "### 短码冲突率" >> "$REPORT_FILE"
    echo "- **冲突率**: ${COLLISION_RATE}%" >> "$REPORT_FILE"
    echo "- **总请求**: ${TOTAL}" >> "$REPORT_FILE"
    echo "- **冲突次数**: ${COLLISIONS}" >> "$REPORT_FILE"
    echo "- **目标**: < 0.1%" >> "$REPORT_FILE"
    
    if (( $(echo "$COLLISION_RATE < 0.1" | bc -l) )); then
        echo -e "- **结果**: ✅ 优秀" >> "$REPORT_FILE"
        echo -e "${GREEN}✅ 冲突率优秀 (< 0.1%)${NC}"
    else
        echo -e "- **结果**: ⚠️ 需要优化" >> "$REPORT_FILE"
        echo -e "${YELLOW}⚠️ 冲突率需要优化${NC}"
    fi
    echo "" >> "$REPORT_FILE"
    echo ""
}

# 从 Prometheus 获取指标
get_prometheus_metrics() {
    echo -e "${YELLOW}[7/7] 从 Prometheus 获取系统指标...${NC}"
    
    if ! curl -s "$PROMETHEUS_URL/api/v1/query?query=up" > /dev/null 2>&1; then
        echo -e "${YELLOW}⚠️ Prometheus 未运行,跳过指标收集${NC}"
        echo "" >> "$REPORT_FILE"
        echo "### Prometheus 指标" >> "$REPORT_FILE"
        echo "- **状态**: Prometheus 未运行" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"
        return
    fi
    
    # 收集 Prometheus 指标...
    
    # P99 延迟: 尝试多种指标和更短的窗口 (irate + [1m])
    # 策略 1: HTTP 接口 P99
    local P99_QUERY='histogram_quantile(0.99, sum by (le) (irate(http_request_duration_seconds_bucket[2m])))'
    local P99_RAW=$(curl -s "$PROMETHEUS_URL/api/v1/query?query=$(echo -n "$P99_QUERY" | jq -sRr @uri)" | jq -r '.data.result[].value[1] // empty' | awk '{print $1; exit}' | head -n 1)
    
    # 策略 2: 如果为空，尝试 [5m] 窗口的率 (rate)
    if [ -z "$P99_RAW" ] || [ "$P99_RAW" == "NaN" ]; then
        P99_QUERY='histogram_quantile(0.99, sum by (le) (rate(http_request_duration_seconds_bucket[5m])))'
        P99_RAW=$(curl -s "$PROMETHEUS_URL/api/v1/query?query=$(echo -n "$P99_QUERY" | jq -sRr @uri)" | jq -r '.data.result[].value[1] // empty' | awk '{print $1; exit}' | head -n 1)
    fi

    # 策略 3: 尝试 gRPC 接口 P99
    if [ -z "$P99_RAW" ] || [ "$P99_RAW" == "NaN" ]; then
        P99_QUERY='histogram_quantile(0.99, sum by (le) (irate(grpc_request_duration_seconds_bucket[2m])))'
        P99_RAW=$(curl -s "$PROMETHEUS_URL/api/v1/query?query=$(echo -n "$P99_QUERY" | jq -sRr @uri)" | jq -r '.data.result[].value[1] // empty' | awk '{print $1; exit}' | head -n 1)
    fi
    
    if [ -n "$P99_RAW" ] && [ "$P99_RAW" != "null" ] && [ "$P99_RAW" != "NaN" ]; then
        P99_MS=$(printf "%.2f" $(echo "$P99_RAW * 1000" | bc -l))
        echo -e "  ${GREEN}P99 延迟: ${P99_MS} ms${NC}"
    else
        P99_MS="0.0" # 默认为 0 而不是显示收集中，方便一眼看出指标是否采到
        echo -e "  ${YELLOW}P99 延迟: 暂无数据 (指标未就绪)${NC}"
    fi
    
    # 写入报告
    echo "### 系统健康指标 (Observability)" >> "$REPORT_FILE"
    echo "- **P99 延迟**: ${P99_MS} ms (目标: < 50ms)" >> "$REPORT_FILE"

    # 新增: Redis 内存占用
    local REDIS_MEM=$(redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -a "$REDIS_PASSWORD" INFO memory 2>/dev/null | grep "used_memory_human" | cut -d: -f2 | tr -d '\r\n ')
    if [ -n "$REDIS_MEM" ]; then
        echo "- **Redis 内存占用**: ${REDIS_MEM}" >> "$REPORT_FILE"
        echo -e "  ${GREEN}Redis 内存: ${REDIS_MEM}${NC}"
    fi

    # 新增: 数据库活跃连接 (使用聚合 sum 以免多个库实例时出错)
    local DB_CONNS=$(curl -s "$PROMETHEUS_URL/api/v1/query?query=sum(postgres_active_connections)" | jq -r '.data.result[0].value[1] // "0"')
    echo "- **数据库活跃连接**: ${DB_CONNS}" >> "$REPORT_FILE"
    echo -e "  ${GREEN}数据库连接: ${DB_CONNS}${NC}"
    echo "" >> "$REPORT_FILE"
    
    echo -e "${GREEN}✅ 指标收集完成${NC}"
    echo ""
}

# 生成最终报告
generate_final_report() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}  性能测试完成!${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""
    echo -e "📊 完整报告已保存至: ${GREEN}$REPORT_FILE${NC}"
    echo ""
    echo "查看报告:"
    echo "  cat $REPORT_FILE"
    echo ""
    echo "建议的简历描述:"
    echo -e "${YELLOW}---------------------------------------${NC}"
    cat >> "$REPORT_FILE" << 'EOF'

---

## 📝 简历建议

基于以上测试结果,可以这样描述项目成果:

**性能优化成果**:
- 实现基于 Go 微服务的高性能 URL 短链接系统,单机 QPS 达到 [QPS 数值]
- 通过 Redis 缓存优化,热点链接 TTFB 控制在 [TTFB 数值] ms,缓存命中率达 [命中率]%
- P99 延迟保持在 [P99 数值] ms 以内,确保 99% 用户获得极速响应
- 优化短码生成算法,在大规模并发下冲突率低于 [冲突率]%

**技术架构**:
- 采用 Go Micro 微服务架构,实现 4 个服务的分布式协同
- 集成 PostgreSQL + Redis + ClickHouse 三层存储,优化读写性能
- 部署 Prometheus + Grafana + Jaeger 完整可观测性栈
- 通过压力测试验证系统在高并发场景下的稳定性
EOF
    
    cat "$REPORT_FILE"
}

# 主函数
main() {
    # 初始化报告
    echo "# URL Shortener 性能测试报告" > "$REPORT_FILE"
    echo "" >> "$REPORT_FILE"
    echo "**测试时间**: $(date '+%Y-%m-%d %H:%M:%S')" >> "$REPORT_FILE"
    echo "**测试环境**: $(uname -s) $(uname -m)" >> "$REPORT_FILE"
    echo "" >> "$REPORT_FILE"
    echo "---" >> "$REPORT_FILE"
    echo "" >> "$REPORT_FILE"
    
    # 执行测试
    check_services
    warmup_system
    test_ttfb
    test_qps
    test_cache_hit_rate
    test_collision_rate
    get_prometheus_metrics
    generate_final_report
}

# 运行主函数
main
