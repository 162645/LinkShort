#!/bin/bash

# 简化版性能测试脚本 - 针对 Docker 环境优化
# 适用于 WSL/Linux 环境

set -e

# 配置
API_BASE="${API_BASE:-http://localhost:8080}"
TEST_USER="perf_test_$(date +%s)"

echo "🚀 URL Shortener 性能快速测试"
echo "=============================="
echo ""

# 1. 检查 API 服务
echo "[1/4] 检查 API 服务..."
if curl -s "$API_BASE/health" > /dev/null 2>&1; then
    echo "✅ API 服务正常"
else
    echo "❌ API 服务未运行 ($API_BASE)"
    echo "请启动服务: make docker-up-all"
    exit 1
fi
echo ""

# 2. TTFB 测试
echo "[2/4] 测试 TTFB (Time to First Byte)..."
echo "创建测试短链接..."
RESPONSE=$(curl -s -X POST "$API_BASE/api/v1/shorten" \
    -H "Content-Type: application/json" \
    -d "{\"long_url\":\"https://ttfb-test-$(date +%s).com\",\"user_id\":\"$TEST_USER\"}")

SHORT_CODE=$(echo "$RESPONSE" | jq -r '.short_code // .data.short_code // empty')

if [ -z "$SHORT_CODE" ]; then
    echo "⚠️  创建短链接失败,尝试使用备用方法..."
    echo "响应: $RESPONSE"
    SHORT_CODE="test-fallback"
fi

echo "测试 50 次 TTFB..."
TTFB_SUM=0
COUNT=0

for i in {1..50}; do
    TTFB=$(curl -o /dev/null -s -w "%{time_starttransfer}" "$API_BASE/$SHORT_CODE" 2>/dev/null || echo "0")
    if [ "$TTFB" != "0" ]; then
        TTFB_MS=$(echo "$TTFB * 1000" | bc 2>/dev/null || echo "0")
        TTFB_SUM=$(echo "$TTFB_SUM + $TTFB_MS" | bc 2>/dev/null || echo "$TTFB_SUM")
        ((COUNT++))
    fi
done

if [ $COUNT -gt 0 ]; then
    AVG_TTFB=$(echo "scale=2; $TTFB_SUM / $COUNT" | bc)
    echo "✅ 平均 TTFB: ${AVG_TTFB} ms (${COUNT} 次测试)"
    
    if (( $(echo "$AVG_TTFB < 20" | bc -l 2>/dev/null || echo 0) )); then
        echo "   🎯 目标达成 (< 20ms)"
    fi
else
    echo "⚠️  TTFB 测试失败"
fi
echo ""

# 3. 简单 QPS 测试 (使用 curl)
echo "[3/4] 简单 QPS 测试 (100 请求)..."
START_TIME=$(date +%s)

for i in {1..100}; do
    curl -s "$API_BASE/$SHORT_CODE" > /dev/null 2>&1 &
    
    # 每 20 个请求等待一下,避免过载
    if [ $((i % 20)) -eq 0 ]; then
        wait
    fi
done

wait  # 等待所有后台任务完成
END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

if [ $DURATION -gt 0 ]; then
    QPS=$((100 / DURATION))
    echo "✅ 简单 QPS 测试: ~${QPS} 请求/秒 (100 请求耗时 ${DURATION} 秒)"
    echo "   💡 使用 wrk 可以获得更准确的 QPS 数据"
else
    echo "✅ 100 请求完成 (耗时 < 1 秒)"
fi
echo ""

# 4. 创建多个短链接测试
echo "[4/4] 批量创建测试 (检测冲突)..."
SUCCESS=0
FAILED=0

for i in {1..20}; do
    RESPONSE=$(curl -s -X POST "$API_BASE/api/v1/shorten" \
        -H "Content-Type: application/json" \
        -d "{\"long_url\":\"https://batch-test-$i-$(date +%s%N).com\",\"user_id\":\"$TEST_USER\"}" 2>/dev/null)
    
    if echo "$RESPONSE" | jq -e '.short_code // .data.short_code' > /dev/null 2>&1; then
        ((SUCCESS++))
    else
        ((FAILED++))
    fi
done

echo "✅ 批量创建完成: 成功 $SUCCESS, 失败 $FAILED"
if [ $FAILED -eq 0 ]; then
    echo "   🎯 无冲突,系统稳定"
fi
echo ""

# 总结
echo "=============================="
echo "📊 测试总结"
echo "=============================="
echo ""
echo "✅ 测试完成!"
echo ""
echo "💡 简历建议:"
if [ $COUNT -gt 0 ]; then
    echo "   \"实现短链接服务,TTFB 控制在 ${AVG_TTFB}ms\""
fi
echo "   \"通过 Go 微服务架构实现高性能 URL 短链接系统\""
echo ""
echo "📚 详细测试:"
echo "   完整测试: bash scripts/perf/run_all_tests.sh"
echo "   TTFB 测试: bash scripts/perf/test_ttfb.sh"
echo "   缓存测试: bash scripts/perf/test_cache_hit_rate.sh"
echo ""
echo "🔧 安装 wrk 获取准确 QPS:"
echo "   sudo apt-get install wrk"
echo "   wrk -t12 -c400 -d30s http://localhost:8080/$SHORT_CODE"
