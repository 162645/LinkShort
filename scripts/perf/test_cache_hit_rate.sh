#!/bin/bash

# Redis 缓存命中率测试脚本

REDIS_HOST="${REDIS_HOST:-localhost}"
REDIS_PORT="${REDIS_PORT:-6379}"
REDIS_PASSWORD="${REDIS_PASSWORD:-redispassword}"  # 默认密码
API_BASE="${API_BASE:-http://localhost:8080}"
TEST_USER="cache_test_user"

echo "🎯 Redis 缓存命中率测试"
echo "========================"
echo ""

# 检查 Redis 连接
echo "1. 检查 Redis 连接..."
REDIS_OK=false

# 尝试多种方式检测 Redis (带密码)
if redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -a "$REDIS_PASSWORD" ping 2>/dev/null | grep -q "PONG"; then
    REDIS_OK=true
elif docker exec url-shortener-redis redis-cli -a "$REDIS_PASSWORD" ping 2>/dev/null | grep -q "PONG"; then
    REDIS_OK=true
elif timeout 2 bash -c "echo -e '\n' | nc -w 1 $REDIS_HOST $REDIS_PORT" > /dev/null 2>&1; then
    REDIS_OK=true
elif docker ps 2>/dev/null | grep -q redis; then
    REDIS_OK=true
fi

if [ "$REDIS_OK" = false ]; then
    echo "❌ 无法连接到 Redis ($REDIS_HOST:$REDIS_PORT)"
    echo "请检查 Redis 是否运行: docker ps | grep redis"
    exit 1
fi

echo "✅ Redis 连接正常"
echo ""

# 创建测试数据
echo "1. 创建测试短链接..."
SHORT_CODE=$(curl -s -X POST "$API_BASE/api/v1/shorten" \
    -H "Content-Type: application/json" \
    -d "{\"long_url\":\"https://cache-hit-test.com\",\"user_id\":\"$TEST_USER\",\"custom_alias\":\"cache-test\"}" | jq -r '.short_code')

if [ -z "$SHORT_CODE" ]; then
    echo "❌ 创建短链接失败"
    exit 1
fi

echo "✅ 短链接创建成功: $SHORT_CODE"
echo ""

# 获取初始统计
echo "2. 获取初始 Redis 统计..."
# 使用密码连接 Redis
if command -v redis-cli &> /dev/null; then
    STATS_BEFORE=$(redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -a "$REDIS_PASSWORD" INFO stats 2>/dev/null)
else
    STATS_BEFORE=$(docker exec url-shortener-redis redis-cli -a "$REDIS_PASSWORD" INFO stats 2>/dev/null)
fi
HITS_BEFORE=$(echo "$STATS_BEFORE" | grep "keyspace_hits:" | cut -d: -f2 | tr -d '\r\n ')
MISSES_BEFORE=$(echo "$STATS_BEFORE" | grep "keyspace_misses:" | cut -d: -f2 | tr -d '\r\n ')

echo "  初始命中: $HITS_BEFORE"
echo "  初始未命中: $MISSES_BEFORE"
echo ""

# 执行访问测试
echo "3. 执行 2000 次访问测试..."
for i in {1..2000}; do
    curl -s "$API_BASE/$SHORT_CODE" > /dev/null
    
    if [ $((i % 400)) -eq 0 ]; then
        echo "  进度: $i/2000"
    fi
done

echo ""

# 获取最终统计
echo "4. 获取最终 Redis 统计..."
sleep 1
# 使用密码连接 Redis
if command -v redis-cli &> /dev/null; then
    STATS_AFTER=$(redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -a "$REDIS_PASSWORD" INFO stats 2>/dev/null)
else
    STATS_AFTER=$(docker exec url-shortener-redis redis-cli -a "$REDIS_PASSWORD" INFO stats 2>/dev/null)
fi
HITS_AFTER=$(echo "$STATS_AFTER" | grep "keyspace_hits:" | cut -d: -f2 | tr -d '\r\n ')
MISSES_AFTER=$(echo "$STATS_AFTER" | grep "keyspace_misses:" | cut -d: -f2 | tr -d '\r\n ')

echo "  最终命中: $HITS_AFTER"
echo "  最终未命中: $MISSES_AFTER"
echo ""

# 计算增量
HITS_DELTA=$((HITS_AFTER - HITS_BEFORE))
MISSES_DELTA=$((MISSES_AFTER - MISSES_BEFORE))
TOTAL_DELTA=$((HITS_DELTA + MISSES_DELTA))

echo "5. 计算缓存命中率..."
echo "  增量命中: $HITS_DELTA"
echo "  增量未命中: $MISSES_DELTA"
echo "  总请求: $TOTAL_DELTA"
echo ""

if [ $TOTAL_DELTA -gt 0 ]; then
    HIT_RATE=$(echo "scale=2; $HITS_DELTA * 100 / $TOTAL_DELTA" | bc)
    
    echo "📊 测试结果:"
    echo "  缓存命中率: $HIT_RATE%"
    echo ""
    
    if (( $(echo "$HIT_RATE > 95" | bc -l) )); then
        echo "✅ 优秀! 缓存命中率 > 95%"
    elif (( $(echo "$HIT_RATE > 80" | bc -l) )); then
        echo "⚠️  良好,但可以优化 (目标 > 95%)"
    else
        echo "❌ 需要优化缓存策略"
    fi
    
    echo ""
    echo "💡 简历建议:"
    echo "   \"实现 Redis 缓存层,热点数据缓存命中率达到 ${HIT_RATE}%\""
else
    echo "❌ 未检测到 Redis 访问,请检查缓存配置"
fi

echo ""
echo "📈 Redis 内存使用情况:"
if command -v redis-cli &> /dev/null; then
    redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" -a "$REDIS_PASSWORD" INFO memory 2>/dev/null | grep "used_memory_human"
else
    docker exec url-shortener-redis redis-cli -a "$REDIS_PASSWORD" INFO memory 2>/dev/null | grep "used_memory_human"
fi
