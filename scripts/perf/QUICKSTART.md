# 性能测试工具包 - 快速开始指南

## 🎯 目标

帮助你获取可写入简历的关键性能指标:
- ⚡ **TTFB**: < 20ms
- 🚀 **QPS**: > 10,000
- 💾 **缓存命中率**: > 95%
- 📊 **P99 延迟**: < 50ms
- 🔄 **冲突率**: < 0.1%

## ⚡ 快速开始 (3 步)

### 1. 启动服务
```bash
cd f:\code_file\golong\go-url-shortener-main
make docker-up-all
# 等待 15 秒
```

### 2. 运行测试
```bash
# Windows (使用 Git Bash 或 WSL)
bash scripts/perf/run_all_tests.sh

# 或者单独测试
bash scripts/perf/test_ttfb.sh
bash scripts/perf/test_cache_hit_rate.sh
```

### 3. 查看报告
```bash
# 查看最新报告
cat performance_results/performance_report_*.md
```

## 📁 文件说明

| 文件 | 说明 |
|------|------|
| `run_all_tests.sh` | 完整测试套件 (自动化) |
| `test_ttfb.sh` | TTFB 测试 |
| `test_cache_hit_rate.sh` | Redis 缓存命中率测试 |
| `create_url.lua` | wrk 压测脚本 |
| `create_url.json` | Apache Bench 测试数据 |
| `README.md` | 快速参考 |
| `USAGE.md` | 详细使用说明 |

## 📊 测试命令速查

```bash
# 完整测试 (5-10 分钟)
bash scripts/perf/run_all_tests.sh

# TTFB 测试 (30 秒)
bash scripts/perf/test_ttfb.sh

# 缓存测试 (1 分钟)
bash scripts/perf/test_cache_hit_rate.sh

# QPS 测试 (需要 wrk)
wrk -t12 -c400 -d30s --latency http://localhost:8080/your-short-code

# 使用 wrk Lua 脚本测试创建 API
wrk -t12 -c400 -d30s -s scripts/perf/create_url.lua http://localhost:8080/api/v1/shorten
```

## 🛠️ 工具安装 (可选,用于 QPS 测试)

### Windows
```powershell
# 方式 1: 使用 WSL (推荐)
wsl --install
# 在 WSL 中: sudo apt-get install wrk

# 方式 2: 使用 Chocolatey
choco install wrk
```

### Linux
```bash
sudo apt-get install wrk apache2-utils
```

## 📝 简历示例

基于测试结果,你可以这样写:

```
URL 短链接服务性能优化 | Go, Redis, PostgreSQL
• 优化短链接服务性能,实现 Redis 缓存层,TTFB 降至 8ms (提升 82%)
• 单机 QPS 达 18,500,P99 延迟 32ms,支持高并发场景
• Redis 缓存命中率 98.7%,短码冲突率 < 0.05%
• 集成 Prometheus + Grafana + Jaeger 完整监控栈
```

## 📖 详细文档

- **完整指南**: [docs/PERFORMANCE_TESTING.md](../../docs/PERFORMANCE_TESTING.md)
- **使用说明**: [USAGE.md](./USAGE.md)
- **项目 README**: [../../README.md](../../README.md)

## ⚠️ 注意事项

1. **Windows 用户**: 需要使用 Git Bash 或 WSL 运行 bash 脚本
2. **端口配置**: 默认使用 8080 端口,如需修改请设置环境变量 `API_BASE`
3. **Redis 连接**: 默认 localhost:6379,可通过环境变量修改
4. **测试数据**: 测试使用独立的测试用户 ID,不影响真实数据

## 🔍 故障排查

### 问题: 脚本无法执行
```bash
# 添加执行权限
chmod +x scripts/perf/*.sh
```

### 问题: Redis 连接失败
```bash
# 检查 Redis 是否运行
redis-cli ping

# 或检查 Docker 容器
docker ps | grep redis
```

### 问题: API 服务未响应
```bash
# 检查服务状态
curl http://localhost:8080/health

# 查看服务日志
docker logs url-shortener-rest-api
```

## 💡 提示

- 首次测试前建议先预热系统 (创建一些测试数据)
- 多次运行测试取平均值,结果更准确
- 保存测试报告,记录优化历程
- 在 Grafana (http://localhost:3000) 中可以看到实时监控数据

---

**需要帮助?** 查看 [详细文档](../../docs/PERFORMANCE_TESTING.md) 或提交 Issue
