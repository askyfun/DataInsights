-- +goose Up
-- 仪表盘文件夹：树结构归档（第一期只归档仪表盘，不归档图表）。
--
-- Design constraints:
--   * id 是 UUIDv7（同 bi_dashboard 的理由：无认证系统下可枚举主键等于让人
--     遍历整张表）。
--   * 树用 parent_id 自引用表达，不落 path 列（物化路径是冗余，改一次父级要
--     重写整棵子树；前端按扁平数组组树，后端只多一次递归守卫查询）。
--   * parent_id 指向**已软删**的文件夹是允许落库的：本表不建 FK，与
--     bi_dashboard 不建 bi_chart FK 同理（软删语义下 FK 会拒绝合法历史态）。
--     读出口一律过滤 deleted_at，孤儿 parent_id 只让子夹在前端退化成根级展示，
--     不会报错。
--   * 环形移动（把 A 移到 A 自己的后代下）由应用层守卫（service/dashboard 的
--     UpdateFolder），DB 层不加约束——PG 没有递归 CHECK。
--   * bi_dashboard.folder_id 同口径：无 FK、NULL = 未归档。
--   * owner_id / tenant_id 按 00005 的不可逆审计列约定带上。
--   * deleted_at 遵循「绝不物理删除」。

CREATE TABLE IF NOT EXISTS bi_dashboard_folder (
    id         UUID PRIMARY KEY,
    name       VARCHAR(255) NOT NULL,
    parent_id  UUID,
    owner_id   INTEGER,
    tenant_id  INTEGER,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now(),
    deleted_at TIMESTAMP NULL
);

-- 建索引 + 子级存在性探测（删除守卫「有无子夹」、环检测「自下往上爬」）都走这一条。
-- partial 谓词用 IS NULL（IMMUTABLE，合法，同 00005 的注释）。
CREATE INDEX IF NOT EXISTS idx_dashboard_folder_parent
    ON bi_dashboard_folder (parent_id) WHERE deleted_at IS NULL;

-- 先加列，再给它建索引：反过来的话 CREATE INDEX 会在「列还不存在」上直接失败
-- （goose 逐条执行，不做整篇预解析 —— 首次实跑就真的炸在这里）。
ALTER TABLE bi_dashboard
    ADD COLUMN IF NOT EXISTS folder_id UUID;

-- 仪表盘侧的归档归属：按文件夹列出一盘（第一期由前端在内存过滤，索引留给后续
-- 「服务端按夹过滤」；同时也是删除守卫数盘的那条路）。
CREATE INDEX IF NOT EXISTS idx_dashboard_folder_id
    ON bi_dashboard (folder_id) WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_dashboard_folder_id;

ALTER TABLE bi_dashboard
    DROP COLUMN IF EXISTS folder_id;

DROP INDEX IF EXISTS idx_dashboard_folder_parent;
DROP TABLE IF EXISTS bi_dashboard_folder;
