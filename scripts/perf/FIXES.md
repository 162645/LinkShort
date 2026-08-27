# 性能测试脚本修复说明

## 🔧 问题原因

原脚本使用 `redis-cli ping` 检测 Redis 时,在某些 Docker 环境下可能无法正确识别 Redis 的 PONG 响应。

## ✅ 已修复的文件

### 1. run_all_tests.sh (完整测试套件)
- 增加了 3 种 Redis 检测方式:
  1. `redis-cli ping` 并检查 PONG 响应
  2. 使用 `nc` (netcat) 测试端口连接
  3. 检查 Docker 容器是否运行

### 2. test_cache_hit_rate.sh (缓存测试)
- 同样增加了多种 Redis 检测方式
- 提供更友好的错误提示

### 3. quick_test.sh (新增 - 快速测试)
- 专为 Docker 环境优化
- 不依赖 redis-cli
- 更好的错误处理
- 适合快速验证系统性能

## 🚀 现在可以使用的测试方式

### 方式 1: 快速测试 (推荐,无需 redis-cli)
```bash
bash scripts/perf/quick_test.sh
```
**特点**:
- ✅ 不需要 redis-cli
- ✅ 测试 TTFB、简单 QPS、批量创建
- ✅ 约 1-2 分钟完成
- ✅ 适合快速验证

### 方式 2: 完整测试
```bash
bash scripts/perf/run_all_tests.sh
```
**特点**:
- ✅ 修复了 Redis 检测问题
- ✅ 测试所有 5 个指标
- ✅ 生成详细报告
- ⏱️ 约 5-10 分钟

### 方式 3: 单项测试
```bash
# TTFB 测试
bash scripts/perf/test_ttfb.sh

# 缓存命中率测试 (需要 redis-cli)
bash scripts/perf/test_cache_hit_rate.sh
```

## 📊 测试示例

### 快速测试输出示例:
```
🚀 URL Shortener 性能快速测试
==============================

[1/4] 检查 API 服务...
✅ API 服务正常

[2/4] 测试 TTFB (Time to First Byte)...
创建测试短链接...
测试 50 次 TTFB...
✅ 平均 TTFB: 12.34 ms (50 次测试)
   🎯 目标达成 (< 20ms)

[3/4] 简单 QPS 测试 (100 请求)...
✅ 简单 QPS 测试: ~50 请求/秒 (100 请求耗时 2 秒)
   💡 使用 wrk 可以获得更准确的 QPS 数据

[4/4] 批量创建测试 (检测冲突)...
✅ 批量创建完成: 成功 20, 失败 0
   🎯 无冲突,系统稳定

==============================
📊 测试总结
==============================

✅ 测试完成!

💡 简历建议:
   "实现短链接服务,TTFB 控制在 12.34ms"
   "通过 Go 微服务架构实现高性能 URL 短链接系统"
```

## 🔍 故障排查

### 如果仍然遇到 Redis 检测问题:

1. **检查 Redis 是否真的在运行**:
```bash
docker ps | grep redis
```

2. **手动测试 Redis 连接**:
```bash
# 方式 1: redis-cli
redis-cli -h localhost -p 6379 ping

# 方式 2: telnet
telnet localhost 6379

# 方式 3: nc (netcat)
echo "PING" | nc localhost 6379
```

3. **使用快速测试脚本**:
```bash
# 这个脚本不依赖 Redis 检测
bash scripts/perf/quick_test.sh
```

## 💡 建议

1. **首次测试**: 使用 `quick_test.sh`,快速验证系统
2. **详细数据**: 安装 wrk 后使用 `run_all_tests.sh`
3. **简历数据**: 运行完整测试,获取所有 5 个指标

## 📝 下一步

```bash
# 1. 运行快速测试
bash scripts/perf/quick_test.sh

# 2. 如果需要详细数据,安装 wrk
sudo apt-get install wrk

# 3. 运行完整测试
bash scripts/perf/run_all_tests.sh

# 4. 查看报告
cat performance_results/performance_report_*.md
```
