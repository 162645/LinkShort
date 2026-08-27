#!/bin/bash

# 快速 TTFB 测试脚本

API_BASE="${API_BASE:-http://localhost:8080}"
TEST_USER="ttfb_test_user"

echo "🚀 TTFB (Time to First Byte) 快速测试"
echo "======================================"
echo ""

# 创建测试短链接
echo "1. 创建测试短链接..."
RESPONSE=$(curl -s -X POST "$API_BASE/api/v1/shorten" \
    -H "Content-Type: application/json" \
    -d "{\"long_url\":\"https://ttfb-quick-test.com\",\"user_id\":\"$TEST_USER\",\"custom_alias\":\"ttfb-quick\"}")

SHORT_CODE=$(echo "$RESPONSE" | jq -r '.short_code // empty')

if [ -z "$SHORT_CODE" ]; then
    echo "❌ 创建短链接失败"
    echo "$RESPONSE"
    exit 1
fi

echo "✅ 短链接创建成功: $SHORT_CODE"
echo ""

# 测试 TTFB
echo "2. 测试 TTFB (100 次请求)..."
echo ""

TTFB_SUM=0
MIN_TTFB=999999
MAX_TTFB=0

for i in {1..100}; do
    TTFB=$(curl -o /dev/null -s -w "%{time_starttransfer}" "$API_BASE/$SHORT_CODE")
    TTFB_MS=$(echo "$TTFB * 1000" | bc)
    TTFB_SUM=$(echo "$TTFB_SUM + $TTFB_MS" | bc)
    
    # 更新最小值和最大值
    if (( $(echo "$TTFB_MS < $MIN_TTFB" | bc -l) )); then
        MIN_TTFB=$TTFB_MS
    fi
    
    if (( $(echo "$TTFB_MS > $MAX_TTFB" | bc -l) )); then
        MAX_TTFB=$TTFB_MS
    fi
    
    # 显示进度
    if [ $((i % 20)) -eq 0 ]; then
        echo "  进度: $i/100"
    fi
done

AVG_TTFB=$(echo "scale=2; $TTFB_SUM / 100" | bc)

echo ""
echo "📊 测试结果:"
echo "  ├─ 平均 TTFB: $AVG_TTFB ms"
echo "  ├─ 最小 TTFB: $MIN_TTFB ms"
echo "  └─ 最大 TTFB: $MAX_TTFB ms"
echo ""

if (( $(echo "$AVG_TTFB < 20" | bc -l) )); then
    echo "✅ 测试通过! TTFB < 20ms (目标达成)"
else
    echo "⚠️  需要优化: TTFB 应该 < 20ms"
fi

echo ""
echo "💡 简历建议:"
echo "   \"通过 Redis 缓存优化,将短链接重定向的 TTFB 控制在 ${AVG_TTFB}ms\""
