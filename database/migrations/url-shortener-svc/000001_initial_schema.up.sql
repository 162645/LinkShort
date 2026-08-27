-- 短链接服务 - 初始架构迁移脚本
-- 此迁移脚本用于创建 URL 映射功能的核心数据库表

-- 启用必要的扩展模块
-- uuid-ossp: 用于生成 UUID（通用唯一识别码）
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
-- pg_stat_statements: 用于追踪和分析 SQL 语句执行性能
CREATE EXTENSION IF NOT EXISTS "pg_stat_statements";

-- URL 映射表（系统的主数据存储表）
CREATE TABLE url_mappings (
                              id BIGSERIAL PRIMARY KEY,           -- 自增主键，使用大整型以支持海量数据
                              short_code VARCHAR(10) UNIQUE NOT NULL, -- 短链接后缀，必须唯一且不能为空
                              long_url TEXT NOT NULL,             -- 原始长链接地址
                              user_id VARCHAR(50) NOT NULL,       -- 创建该链接的用户 ID
                              created_at TIMESTAMPTZ DEFAULT NOW(), -- 创建时间，默认为当前系统时间
                              updated_at TIMESTAMPTZ DEFAULT NOW(), -- 最后更新时间，默认为当前系统时间
                              expires_at TIMESTAMPTZ,             -- 失效/过期时间，可选字段
                              click_count BIGINT DEFAULT 0,       -- 累计点击次数，默认为 0
                              last_accessed TIMESTAMPTZ,          -- 最后一次被访问/跳转的时间
                              is_active BOOLEAN DEFAULT true,     -- 激活状态，默认为 true（有效）
                              metadata JSONB DEFAULT '{}'::jsonb  -- 扩展元数据，使用 JSON 格式存储额外信息
);

-- 性能优化索引（未使用 CONCURRENTLY 关键字以确保迁移脚本的兼容性）
CREATE INDEX idx_url_mappings_short_code ON url_mappings(short_code); -- 加速短码查询（最常用）
CREATE INDEX idx_url_mappings_user_id ON url_mappings(user_id);       -- 加速按用户筛选链接
CREATE INDEX idx_url_mappings_created_at ON url_mappings(created_at); -- 加速按创建时间排序/查询
-- 条件索引：仅针对有效状态的链接建索引，减小索引体积并加速活跃链接查询
CREATE INDEX idx_url_mappings_active ON url_mappings(is_active) WHERE is_active = true;
-- 条件索引：仅针对设置了过期时间的记录建索引，优化后台清理任务的效率
CREATE INDEX idx_url_mappings_expires_at ON url_mappings(expires_at) WHERE expires_at IS NOT NULL;

-- 创建自动更新时间戳的触发器函数
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    -- 当记录被修改时，自动将 updated_at 字段更新为当前时间
    NEW.updated_at = NOW();
RETURN NEW;
END;
$$ language 'plpgsql';

-- 将触发器绑定到 url_mappings 表
-- 在每一行数据执行 UPDATE 操作之前，自动调用上述函数
CREATE TRIGGER update_url_mappings_updated_at
    BEFORE UPDATE ON url_mappings
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();