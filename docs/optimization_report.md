# 项目优化实施报告

**日期**: 2025-12-27
**状态**: 已实施

本报告详细说明了针对 Go 微服务短链接项目的性能优化实施细节。本次优化主要集中在解决短码生成冲突、提升短链接解析性能以及增强系统防护能力。

## 1. 核心优化一：ID 生成策略升级

### 目标
解决之前“随机字符串 + 数据库重试”方案在高并发下的冲突问题，并实现短码长度的自然增长。

### 实施细节
*   **废弃算法**: 删除了 `url-shortener-svc` 中的随机字符串生成逻辑。
*   **新算法**: **Redis INCR (原子自增) + 洗牌 Base62 编码**。
*   **代码位置**: `utils/idgen/idgen.go`
*   **核心逻辑**:
    1.  利用 Redis `INCR` 命令获取全局唯一的连续整数 ID。
    5.  **自然扩展机制 (6位 -> 7位)**:
        *   当前设置初始 ID 为 `10,000,000,000` (100亿)，Base62 编码后为 6 位字符（如 `a3b9zX`）。
        *   **阈值**: 当 ID 增长到 `56,800,235,584` ($62^6$) 时，生成的短码将自动变为 7 位。
        *   **容量**: 6 位短码可提供约 560 亿个组合，足以支撑当前业务数年；7 位短码可提供 3.5 万亿个组合。
        *   此机制无需人工干预，随着业务量增长平滑过渡。

### 收益
*   **零冲突**: 严格的数学唯一性，彻底消除了数据库冲突检测的开销。
*   **高性能**: 仅需一次 Redis RT，写入延迟大幅降低。

## 2. 核心优化二：重定向服务多级缓存与防护

### 目标
将短链接跳转（Read Path）的 P99 延迟控制在 5ms 以内，并防止缓存穿透和击穿。

### 实施细节

#### 2.1 引入布隆过滤器 (Bloom Filter)
*   **代码位置**: `utils/bloom/bloom.go`, `services/redirect-svc/store/store.go`
*   **逻辑**:
    *   在查询 Redis 和数据库之前，先查询 Redis Bitmaps 实现的布隆过滤器。
    *   如果布隆过滤器返回 `false`，说明短码绝对不存在，直接返回 404，拦截无效流量。
    *   在生成短码时（`url-shortener-svc`），同步将新短码加入布隆过滤器中。
*   **防护**: 有效解决了恶意攻击导致的缓存穿透问题，保护了数据库。

#### 2.2 引入本地缓存 (Local Cache / L1)
*   **代码位置**: `services/redirect-svc/store/store.go`
*   **技术**: 使用 `go-cache` 库实现的进程内内存缓存。
*   **逻辑**:
    *   针对热点短链接，解析结果会缓存到服务实例的内存中（默认过期时间 10 分钟）。
    *   请求到达时，优先查 L1 缓存。命中则直接返回，耗时 < 1ms。
*   **收益**: 对头部热点流量实现极致响应速度，减少 Redis 网络 IO。

#### 2.3 引入 Singleflight (防缓存击穿)
*   **代码位置**: `services/redirect-svc/store/store.go`
*   **技术**: `golang.org/x/sync/singleflight`
*   **逻辑**:
    *   当 L1 和 L2（Redis）缓存同时未命中时（例如热点 Key 突然过期），大量并发请求会涌向数据库。
    *   使用 `singleflight.Group` 将针对同一 `shortCode` 的所有并发请求合并。
    *   仅允许一个请求去执行“查 Redis -> 查 DB -> 回填缓存”的重型操作，其他请求等待并共享结果。
*   **收益**: 彻底杜绝了缓存击穿导致的数据库雪崩风险。

## 3. 代码修改清单

| 服务/模块 | 文件路径 | 修改内容 |
| :--- | :--- | :--- |
| **Common Utils** | `utils/idgen/idgen.go` | **[新增]** 实现 RedisIDGenerator (INCR + Shuffle Base62) |
| **Common Utils** | `utils/bloom/bloom.go` | **[新增]** 实现 RedisBloomFilter (Bitmaps + Hash) |
| **Write Service** | `services/url-shortener-svc/domain/service.go` | **[修改]** 注入 IdGen 和 BloomFilter；替换 ShortenURL 逻辑；添加新短码到 Bloom |
| **Read Service** | `services/redirect-svc/store/store.go` | **[修改]** 引入 `go-cache` 和 `singleflight`；重构 `ResolveURL` 实现三级解析流程 (Local->Bloom->Singleflight(Redis->DB)) |

## 4. 优化后系统架构图 (文字版)

**Write Path (生成短链):**
1. API 请求 -> 2. Redis INCR (获取 ID) -> 3. Base62 编码 (生成短码) -> 4. 写入 DB -> 5. 写入 Bloom Filter -> 6. 返回。

**Read Path (解析短链):**
1. API 请求 -> 2. **Local Cache** (命中即返) -> 3. **Bloom Filter** (不存在即返 404) -> 4. **Singleflight** (合并并发) -> 5. **Redis** (命中即返) -> 6. **DB** (回源 + 回填缓存) -> 7. 返回。
