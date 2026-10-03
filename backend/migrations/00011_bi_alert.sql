-- +goose Up
-- 指标预警（issue #155）：规则表 + 触发记录表。
--
-- Design constraints:
--   * bi_alert_rule 绑定「图表 id + 指标（数据行的输出列 alias 键）+ 算子 + 阈值」。
--     不落指标快照：每次评估实时调图表取数管道，图表改动后规则天然跟随。
--   * id 是 UUIDv7 字符串（同 bi_dashboard 的理由：无认证系统下可枚举主键等于让人
--     遍历整张表），VARCHAR(64) 容纳 uuid 文本形态。
--   * last_triggered_date 是 UTC 日期的「同日去重」标记：评估器触发后写当天日期，
--     同日再次评估直接跳过。NULL = 从未触发。
--   * bi_alert_trigger 只追加不改写：notified / notify_error 记录通知结果一次成型，
--     不做重试队列（本期通知只有邮件，失败原因落库供人排查）。
--   * 触发记录带 rule_id 但不建 FK：规则软删后历史触发记录仍要可读（读出口按
--     规则存在性过滤），软删语义下 FK 会拒绝这个合法历史态。
--   * created_at / updated_at 用 TIMESTAMP（对齐 00005 / 00007），无 FK。
--   * deleted_at 遵循「绝不物理删除」。

CREATE TABLE IF NOT EXISTS bi_alert_rule (
    id                  VARCHAR(64) PRIMARY KEY,
    name                VARCHAR(255) NOT NULL,
    chart_id            INTEGER NOT NULL,
    metric              VARCHAR(255) NOT NULL,
    operator            VARCHAR(8) NOT NULL,
    threshold           DOUBLE PRECISION NOT NULL,
    enabled             BOOLEAN NOT NULL DEFAULT TRUE,
    last_triggered_date DATE,
    owner_id            INTEGER,
    tenant_id           INTEGER,
    created_at          TIMESTAMP NOT NULL DEFAULT now(),
    updated_at          TIMESTAMP NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMP NULL
);

-- 评估器每轮只扫 enabled 且未软删的规则；列表读出口按 created_at ASC。
-- partial 谓词用 IS NULL（IMMUTABLE，同 00005 的注释）。
CREATE INDEX IF NOT EXISTS idx_alert_rule_enabled
    ON bi_alert_rule (enabled) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS bi_alert_trigger (
    id           VARCHAR(64) PRIMARY KEY,
    rule_id      VARCHAR(64) NOT NULL,
    metric_value DOUBLE PRECISION NOT NULL,
    threshold    DOUBLE PRECISION NOT NULL,
    message      TEXT NOT NULL,
    notified     BOOLEAN NOT NULL DEFAULT FALSE,
    notify_error TEXT,
    created_at   TIMESTAMP NOT NULL DEFAULT now()
);

-- 触发历史按规则回看（GET /api/alerts/{id}/triggers：倒序取最近 50 条）。
CREATE INDEX IF NOT EXISTS idx_alert_trigger_rule_created
    ON bi_alert_trigger (rule_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_alert_trigger_rule_created;
DROP TABLE IF EXISTS bi_alert_trigger;

DROP INDEX IF EXISTS idx_alert_rule_enabled;
DROP TABLE IF EXISTS bi_alert_rule;
