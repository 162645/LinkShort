# 性能测试脚本 Redis 密码问题修复

## ✅ 已修复

我发现了问题的根本原因并已修复:

### 问题
1. **Redis 需要密码认证**: `redispassword`
2. **测试脚本没有使用密码**连接 Redis
3. 导致缓存命中率测试失败 (显示 0%)

### 修复内容

已更新以下文件,添加 Redis 密码支持:

1. ✅ `scripts/perf/run_all_tests.sh`
2. ✅ `scripts/perf/test_cache_hit_rate.sh`

### 修复细节

- 添加了 `REDIS_PASSWORD` 环境变量 (默认: `redispassword`)
- 所有 `redis-cli` 命令现在使用 `-a "$REDIS_PASSWORD"` 参数
- 添加了 Docker exec 作为备用方案 (当 redis-cli 未安装时)

## 🚀 现在可以重新测试

```bash
# 重新运行完整测试
bash scripts/perf/run_all_tests.sh

# 或者只测试缓存
bash scripts/perf/test_cache_hit_rate.sh
```

## 📊 预期结果

现在应该能看到**真实的缓存命中率**数据了！

基于你的 TTFB 只有 5.8ms,我预测:
- 缓存命中率应该 > 90%
- 这将解释为什么响应这么快

## 💡 关于 QPS 较低的问题

QPS 只有 1670 (目标 > 10,000) 可能的原因:

1. **并发配置**: wrk 使用 400 并发可能超过了系统限制
2. **数据库连接池**: 可能需要增加连接池大小
3. **Go 并发设置**: 检查 GOMAXPROCS 设置
4. **网络延迟**: WSL 环境可能有额外的网络开销

### 建议优化

查看应用配置:
```bash
# 检查服务日志看是否有性能瓶颈
docker logs rest-api-service | tail -50
docker logs url-shortener-service | tail -50
```

## 📝 更新后的简历建议

基于当前数据 (修复 Redis 后会更好):

```
URL 短链接服务性能优化 | Go, Redis, PostgreSQL
• 优化短链接重定向性能,TTFB 降至 5.8ms,达到毫秒级响应
• 实现 Redis 缓存层,缓存命中率达 [待测试]%
• 基于 Go Micro 微服务架构,支持分布式部署
• 通过压力测试验证系统稳定性,零冲突率
```

现在重新运行测试,应该能看到正确的缓存数据了! 🎉
