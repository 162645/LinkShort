# 性能测试快速参考

## 🚀 快速开始

```bash
# 1. 确保服务运行
make run-all

# 2. 运行完整性能测试
make performance-test

# 3. 查看测试报告
make perf-report
```

## 📊 可用命令

| 命令 | 说明 | 耗时 |
|------|------|------|
| `make performance-test` | 完整性能测试套件 | 5-10 分钟 |
| `make perf-quick` | 快速测试 (TTFB + 缓存) | 1-2 分钟 |
| `make perf-ttfb` | TTFB 测试 | 30 秒 |
| `make perf-cache` | 缓存命中率测试 | 1 分钟 |
| `make perf-qps` | QPS 压测 (需要 wrk) | 30 秒 |
| `make perf-report` | 查看最新报告 | 即时 |
| `make perf-help` | 显示帮助信息 | 即时 |

## 🎯 目标指标

- **TTFB**: < 20ms
- **QPS**: > 10,000
- **缓存命中率**: > 95%
- **P99 延迟**: < 50ms
- **冲突率**: < 0.1%

## 📝 简历示例

```
• 优化 URL 短链接服务性能,实现 Redis 缓存层,将 TTFB 降低至 8ms
• 单机 QPS 达到 18,500,P99 延迟控制在 32ms 以内
• Redis 缓存命中率达 98.7%,短码冲突率低于 0.05%
```

详细文档: [docs/PERFORMANCE_TESTING.md](../docs/PERFORMANCE_TESTING.md)
