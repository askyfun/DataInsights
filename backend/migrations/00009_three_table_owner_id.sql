-- +goose Up
-- R-22 三表归属欠账（issue #171，MS-1 · P0 · 不可逆）：
-- bi_datasource / bi_dataset / bi_chart 补 owner_id 审计列。
--
-- 为什么只加列、不做鉴权/过滤：L0（单团队内网、无账号）刻意不过滤；但归属一旦
-- 缺失就无法事后还原（每多一天没有 owner_id，就多一天"创建者未知"的存量行），
-- 所以趁 Phase C 触发前先把列 + 存量占位钉死。
--   * tenant_id 归 Phase C（R-03），本条不带：与 owner 不同，租户可由 owner 推导，
--     不是"随时间流失"的信息，延后加没有不可逆代价（v5.0 口径）。
--   * bi_share 已由 00008 退役（只标注不 DROP、代码零读写），不在此补列。
--   * 占位常量：存量行与无登录态新建一律写 0（见 internal/model.SystemOwnerID）。
--     0 被预留，永不与真实 bi_user.id（自增从 1 起）相碰；#183 落地后由请求上下文
--     里的真实用户 id 取代。
--   * 类型取 INTEGER：本 issue 文字写 BIGINT，但全仓现存 owner_id（bi_query /
--     bi_dashboard / bi_dashboard_folder）均为 INTEGER，一致性优先，Phase C 统一。
ALTER TABLE bi_datasource ADD COLUMN IF NOT EXISTS owner_id INTEGER;
ALTER TABLE bi_dataset    ADD COLUMN IF NOT EXISTS owner_id INTEGER;
ALTER TABLE bi_chart      ADD COLUMN IF NOT EXISTS owner_id INTEGER;

-- 存量回填占位（幂等：只刷未赋值行，重放无副作用）。
UPDATE bi_datasource SET owner_id = 0 WHERE owner_id IS NULL;
UPDATE bi_dataset    SET owner_id = 0 WHERE owner_id IS NULL;
UPDATE bi_chart      SET owner_id = 0 WHERE owner_id IS NULL;

-- 归属反查索引：Phase C 的"我的资源"过滤（#119）走这条；L0 不读，先备好。
CREATE INDEX IF NOT EXISTS idx_datasource_owner_id ON bi_datasource (owner_id);
CREATE INDEX IF NOT EXISTS idx_dataset_owner_id    ON bi_dataset (owner_id);
CREATE INDEX IF NOT EXISTS idx_chart_owner_id      ON bi_chart (owner_id);

-- +goose Down
DROP INDEX IF EXISTS idx_chart_owner_id;
DROP INDEX IF EXISTS idx_dataset_owner_id;
DROP INDEX IF EXISTS idx_datasource_owner_id;
ALTER TABLE bi_chart      DROP COLUMN IF EXISTS owner_id;
ALTER TABLE bi_dataset    DROP COLUMN IF EXISTS owner_id;
ALTER TABLE bi_datasource DROP COLUMN IF EXISTS owner_id;
