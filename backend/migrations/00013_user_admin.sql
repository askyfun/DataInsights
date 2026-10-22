-- +goose Up
-- R-86 最小用户管理（issue #202）：补两个字段支撑管理员建号/禁用与列表展示。
--
--   * display_name：列表展示用的显示名，可为空串（不强制填写）。
--   * disabled_at：禁用位用时间戳而不是布尔，语义与 deleted_at/revoked_at 一致
--     （谁禁用、何时禁用一目了然，且 NULL 即启用）。禁用只吊销凭据，不软删账号。
--   * 会话/PAT 的「禁用即刻失效」不靠新字段：Verify 在查用户时加
--     disabled_at IS NULL，禁用动作同时把该用户全部未撤销 token 打上 revoked_at，
--     双保险，不留缓存窗口。
ALTER TABLE bi_user ADD COLUMN IF NOT EXISTS display_name VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE bi_user ADD COLUMN IF NOT EXISTS disabled_at  TIMESTAMP;

-- +goose Down
ALTER TABLE bi_user DROP COLUMN IF EXISTS disabled_at;
ALTER TABLE bi_user DROP COLUMN IF EXISTS display_name;
