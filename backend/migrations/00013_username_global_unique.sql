-- +goose Up
-- R-91（issue #216）：用户名唯一性覆盖已禁用（软删）行。
--
-- 旧约束是 `WHERE deleted_at IS NULL` 的 partial unique，于是禁用一个人之后
-- 他的名字可以被重新注册 —— 历史归属与审计会指向两个人。改成全表唯一后，
-- 被禁用的用户名永久保留。
--
-- 升级前可能已存在「存活行 + 若干软删行」同名的情况（旧约束允许）。直接建
-- 唯一索引会失败，所以先把重复的软删行改名（存活行永不改），保证升级不中断。
UPDATE bi_user u
SET username = left(u.username, 240) || '~deleted-' || u.id
FROM (
    SELECT id
    FROM (
        SELECT id,
               row_number() OVER (
                   PARTITION BY username
                   ORDER BY (deleted_at IS NULL) DESC, id
               ) AS rn
        FROM bi_user
    ) ranked
    WHERE ranked.rn > 1
      AND EXISTS (SELECT 1 FROM bi_user d WHERE d.id = ranked.id AND d.deleted_at IS NOT NULL)
) dup
WHERE u.id = dup.id;

DROP INDEX IF EXISTS idx_user_username;
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_username ON bi_user (username);

-- +goose Down
DROP INDEX IF EXISTS idx_user_username;
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_username
    ON bi_user (username) WHERE deleted_at IS NULL;
