# 数据库连接池参数优化指南

## ❓ 这些值是不是越大越好？

**答案**: ❌ **不是！** 需要根据实际情况调优。

---

## 📊 参数说明

### 1. MaxOpenConns (最大打开连接数)

**作用**: 限制同时打开的数据库连接总数

**不是越大越好的原因**:
- ✅ **太小**: 请求排队,QPS 低 (当前问题)
- ❌ **太大**: 
  - 数据库服务器压力过大
  - 内存消耗增加
  - 连接管理开销增加
  - PostgreSQL 默认最大连接数通常是 100-200

**推荐值计算**:
```
MaxOpenConns = 预期并发数 × 1.5

例如:
- 400 并发 → 200 连接 ✅
- 1000 并发 → 500 连接 (需要同时调整数据库配置)
```

**最佳实践**:
- 开发环境: 25-50
- 生产环境 (中等负载): 100-200 ✅ **推荐**
- 生产环境 (高负载): 200-500
- ⚠️ **不建议超过 500**

---

### 2. MaxIdleConns (最大空闲连接数)

**作用**: 保持一定数量的空闲连接,减少连接建立开销

**不是越大越好的原因**:
- ✅ **太小**: 频繁创建/销毁连接,性能差
- ❌ **太大**: 
  - 占用内存
  - 空闲连接可能超时失效

**推荐值计算**:
```
MaxIdleConns = MaxOpenConns × 20-30%

例如:
- MaxOpenConns = 200 → MaxIdleConns = 50 ✅
```

**最佳实践**:
- 至少保持 10-20 个空闲连接
- 不要超过 MaxOpenConns 的 50%

---

### 3. ConnMaxLifetime (连接最大生命周期)

**作用**: 连接存活多久后强制关闭并重建

**不是越大越好的原因**:
- ✅ **太小**: 频繁重建连接,开销大
- ❌ **太大**: 
  - 可能遇到数据库端超时
  - 连接泄漏风险
  - 无法应用数据库配置更新

**推荐值**:
- **5-15 分钟** ✅
- 不要超过 30 分钟

---

### 4. ConnMaxIdleTime (空闲连接最大存活时间)

**作用**: 空闲连接多久没用就关闭

**推荐值**:
- **3-5 分钟** ✅
- 应该小于 ConnMaxLifetime

---

## 🎯 推荐配置

### 场景 1: 中等负载 (推荐,适合你的情况)

```go
db.SetMaxOpenConns(200)              // 支持 400 并发
db.SetMaxIdleConns(50)               // 保持 50 个空闲
db.SetConnMaxLifetime(10 * time.Minute)
db.SetConnMaxIdleTime(5 * time.Minute)
```

**适用于**:
- 并发 200-500
- QPS 目标 5,000-15,000

---

### 场景 2: 高负载

```go
db.SetMaxOpenConns(500)              // 支持 1000 并发
db.SetMaxIdleConns(100)              
db.SetConnMaxLifetime(15 * time.Minute)
db.SetConnMaxIdleTime(5 * time.Minute)
```

**注意**: 需要同时调整 PostgreSQL 配置:
```sql
-- 在 PostgreSQL 中设置
ALTER SYSTEM SET max_connections = 600;
```

---

### 场景 3: 低负载 (开发环境)

```go
db.SetMaxOpenConns(25)               // 原始值
db.SetMaxIdleConns(5)                
db.SetConnMaxLifetime(5 * time.Minute)
```

---

## ⚠️ 重要提醒

### 1. 数据库服务器限制

PostgreSQL 默认最大连接数:
```bash
# 查看当前限制
docker exec url-shortener-postgres psql -U postgres -c "SHOW max_connections;"

# 通常是 100
```

**如果你设置 MaxOpenConns > 数据库 max_connections**:
- 会出现连接错误
- 需要调整数据库配置

---

### 2. 内存消耗

每个连接大约占用:
- PostgreSQL: 5-10 MB
- 200 个连接 ≈ 1-2 GB 内存

---

### 3. 监控指标

优化后应该监控:
```go
stats := db.Stats()
fmt.Printf("Open: %d, InUse: %d, Idle: %d\n", 
    stats.OpenConnections,
    stats.InUse, 
    stats.Idle)
```

**健康状态**:
- InUse 接近 MaxOpenConns → 需要增加
- Idle 总是很高 → 可以减少 MaxIdleConns

---

## 📈 性能预期

| 配置 | MaxOpenConns | 预期 QPS | 适用场景 |
|------|--------------|----------|----------|
| 保守 | 25 | 1,000-2,000 | 开发环境 |
| **推荐** | **200** | **8,000-15,000** | **生产环境** ✅ |
| 激进 | 500 | 20,000-30,000 | 高负载 |

---

## 💡 总结

**核心原则**: 
1. ❌ **不是越大越好**
2. ✅ **根据并发量计算**: `MaxOpenConns ≈ 并发数 × 1.5`
3. ✅ **考虑数据库限制**: 不要超过数据库 max_connections
4. ✅ **监控和调优**: 根据实际运行情况微调

**你的情况**:
- 当前: 25 连接,400 并发 → QPS 1,736 ❌
- 优化: 200 连接,400 并发 → QPS 10,000+ ✅
- **这是最佳平衡点**
