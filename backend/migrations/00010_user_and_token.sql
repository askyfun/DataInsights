-- +goose Up
-- R-82 用户系统 v1（issue #183）：账号 + 首个用户即系统管理员 + 统一 token 表。
--
-- 设计约束：
--   * bi_user.id 用 SERIAL(INTEGER)，与 #171 的 owner_id INTEGER 同宽；0 是系统占位
--     （model.SystemOwnerID），真实用户自增从 1 起，永不相碰。
--   * password_hash 存 bcrypt 摘要，明文绝不落库。
--   * 注册引导（保守默认，单团队内网）：仅当无存活用户时允许注册，且该用户为 admin；
--     之后注册关闭。避免任意人自助接入可连任意数据源的平台。
--   * bi_token 统一承载「会话 Token（给人，24h）」与「PAT（给机器，长）」：
--       - kind ∈ {session,pat}；token_hash = sha256(raw) 十六进制，明文只在签发响应里回显一次。
--       - prefix 存可识别前缀（di_sess_ / di_pat_ + 若干位）供列表辨认/撤销；不含完整密钥。
--       - expires_at NULL = 不过期（PAT 显式勾选长期）；撤销写 revoked_at，即时生效、无缓存窗口。
--       - last_used_at/ip/ua 记最近使用，供用户自查吊销。
--     #184 落地 PAT 的管理端点与前端；本表先建好，两类 token 共用同一校验中间件（#184 验收 3）。
--   * 软删（deleted_at），全仓「绝不物理删除」。
CREATE TABLE IF NOT EXISTS bi_user (
    id            SERIAL PRIMARY KEY,
    username      VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role          VARCHAR(20)  NOT NULL DEFAULT 'user',
    created_at    TIMESTAMP    NOT NULL DEFAULT now(),
    updated_at    TIMESTAMP    NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMP
);
-- 存活用户内用户名唯一（软删行不占用名字）。partial 谓词 IS NULL 是 IMMUTABLE，合法。
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_username
    ON bi_user (username) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS bi_token (
    id           SERIAL PRIMARY KEY,
    user_id      INTEGER     NOT NULL,
    kind         VARCHAR(20) NOT NULL,
    name         VARCHAR(255),
    token_hash   VARCHAR(64) NOT NULL,
    prefix       VARCHAR(64) NOT NULL,
    expires_at   TIMESTAMP,
    last_used_at TIMESTAMP,
    last_used_ip VARCHAR(64),
    last_used_ua VARCHAR(255),
    revoked_at   TIMESTAMP,
    created_at   TIMESTAMP   NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_token_hash ON bi_token (token_hash);
CREATE INDEX IF NOT EXISTS idx_token_user ON bi_token (user_id);

-- +goose Down
DROP INDEX IF EXISTS idx_token_user;
DROP INDEX IF EXISTS idx_token_hash;
DROP TABLE IF EXISTS bi_token;
DROP INDEX IF EXISTS idx_user_username;
DROP TABLE IF EXISTS bi_user;
