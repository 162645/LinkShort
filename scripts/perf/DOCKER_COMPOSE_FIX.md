# 部署脚本修复说明

## 问题
你的系统使用 `docker compose` (v2) 而不是 `docker-compose` (v1)

## 解决方案
脚本已更新,现在会自动检测并使用正确的命令:
- 如果有 `docker-compose` → 使用 v1 命令
- 如果有 `docker compose` → 使用 v2 命令

## 现在可以运行
```bash
bash scripts/perf/deploy_optimization.sh
```

## 或者手动部署
```bash
# 使用 docker compose (v2)
docker compose down
make clean && make build-all
docker compose build --no-cache redirect-svc
docker compose up -d
sleep 20
bash scripts/perf/run_all_tests.sh
```
