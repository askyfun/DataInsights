-- +goose Up
-- 内置示例数据集（R-79 / issue #174）：首次部署即可分析的数据。
--
-- 载体是同一 PostgreSQL 库内的两张独立前缀表（di_sample_），由迁移灌数；
-- 「加载示例」只创建一张隐藏的示例数据源（is_sample）与两个数据集，
-- 不新增驱动、也不把示例表混进用户业务表的命名空间。
ALTER TABLE bi_datasource ADD COLUMN IF NOT EXISTS is_sample BOOLEAN NOT NULL DEFAULT FALSE;

-- 至多一个「活跃」示例数据源：并发首次加载 / 重复点击的兜底（软删后可重新加载）。
CREATE UNIQUE INDEX IF NOT EXISTS idx_bi_datasource_single_sample
    ON bi_datasource (is_sample)
    WHERE is_sample = TRUE AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS di_sample_orders (
    order_id   INTEGER PRIMARY KEY,
    order_date DATE NOT NULL,
    region     TEXT NOT NULL,
    category   TEXT NOT NULL,
    quantity   INTEGER NOT NULL,
    amount     NUMERIC(12, 2) NOT NULL,
    status     TEXT NOT NULL
);

INSERT INTO di_sample_orders (order_id, order_date, region, category, quantity, amount, status)
SELECT
    g,
    CURRENT_DATE - ((g * 7) % 90),
    (ARRAY['华东', '华北', '华南', '西南', '东北'])[(g % 5) + 1],
    (ARRAY['数码家电', '家居日用', '服饰鞋包', '食品饮料'])[(g % 4) + 1],
    (g % 6) + 1,
    round(((g % 37) * 9.9 + 49.5)::numeric, 2),
    (ARRAY['已完成', '已完成', '已完成', '已取消', '退款中'])[(g % 5) + 1]
FROM generate_series(1, 180) AS g
ON CONFLICT (order_id) DO NOTHING;

CREATE TABLE IF NOT EXISTS di_sample_products (
    product_id   INTEGER PRIMARY KEY,
    product_name TEXT NOT NULL,
    category     TEXT NOT NULL,
    price        NUMERIC(10, 2) NOT NULL,
    stock        INTEGER NOT NULL,
    launched_on  DATE NOT NULL
);

INSERT INTO di_sample_products (product_id, product_name, category, price, stock, launched_on)
SELECT
    g,
    '示例商品 ' || g,
    (ARRAY['数码家电', '家居日用', '服饰鞋包', '食品饮料'])[(g % 4) + 1],
    round(((g % 23) * 19.9 + 29.9)::numeric, 2),
    (g * 13) % 500,
    CURRENT_DATE - (g * 11)
FROM generate_series(1, 24) AS g
ON CONFLICT (product_id) DO NOTHING;

-- +goose Down
DROP INDEX IF EXISTS idx_bi_datasource_single_sample;
DROP TABLE IF EXISTS di_sample_products;
DROP TABLE IF EXISTS di_sample_orders;
ALTER TABLE bi_datasource DROP COLUMN IF EXISTS is_sample;
