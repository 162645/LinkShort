-- 回滚短链接服务 - 初始架构迁移脚本
-- 该脚本的作用是销毁所有已创建的数据库对象，恢复到初始状态

-- 1. 删除触发器
-- 如果 url_mappings 表上存在名为 update_url_mappings_updated_at 的触发器，则将其删除
DROP TRIGGER IF EXISTS update_url_mappings_updated_at ON url_mappings;

-- 2. 删除函数
-- 删除用于自动更新 updated_at 字段的函数
DROP FUNCTION IF EXISTS update_updated_at_column();

-- 3. 删除所有索引
-- 删除为了优化查询性能而创建的 5 个索引
DROP INDEX IF EXISTS idx_url_mappings_short_code;  -- 删除短码索引
DROP INDEX IF EXISTS idx_url_mappings_user_id;     -- 删除用户ID索引
DROP INDEX IF EXISTS idx_url_mappings_created_at;  -- 删除创建时间索引
DROP INDEX IF EXISTS idx_url_mappings_active;      -- 删除有效状态条件索引
DROP INDEX IF EXISTS idx_url_mappings_expires_at;  -- 删除过期时间条件索引

-- 4. 删除核心数据表
-- 如果存在 url_mappings 表，则将其删除
-- CASCADE (级联删除) 表示如果其他对象（如视图或外键约束）依赖此表，也会一并处理，确保彻底删干净
DROP TABLE IF EXISTS url_mappings CASCADE;