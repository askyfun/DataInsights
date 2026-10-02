package model

import (
	"database/sql"
	"time"

	"github.com/uptrace/bun"
)

// User is the bun model for bi_user (migration 00010, R-82 / issue #183).
//
// PasswordHash holds a bcrypt digest; the plaintext never touches storage. The
// very first registered user gets role "admin" (bootstrap-only registration is
// enforced in the service, not the DB). id is INTEGER to line up with the
// owner_id audit column added in #171 — 0 stays reserved as SystemOwnerID, real
// users begin at 1.
type User struct {
	bun.BaseModel `bun:"bi_user"`

	ID           int          `bun:"id,pk,autoincrement"`
	Username     string       `bun:"username,notnull"`
	PasswordHash string       `bun:"password_hash,notnull"`
	Role         string       `bun:"role,notnull"`
	CreatedAt    time.Time    `bun:"created_at,notnull"`
	UpdatedAt    time.Time    `bun:"updated_at,notnull"`
	DeletedAt    sql.NullTime `bun:"deleted_at,nullzero"`
}

// TableName satisfies bun's table naming.
func (*User) TableName() string { return "bi_user" }

// Token is the bun model for bi_token: one table backing BOTH the short human
// session token and the long machine PAT (issue #183/#184).
//
// Only TokenHash (sha256 hex of the raw secret) is stored — the plaintext is
// returned exactly once at issuance. Prefix is a short, non-secret identifier
// ("di_pat_ab12…") shown in listings so a user can recognise which token to
// revoke. RevokedAt / ExpiresAt gate validity with no silent cache window
// (#184 acceptance 4).
type Token struct {
	bun.BaseModel `bun:"bi_token"`

	ID         int            `bun:"id,pk,autoincrement"`
	UserID     int            `bun:"user_id,notnull"`
	Kind       string         `bun:"kind,notnull"`
	Name       sql.NullString `bun:"name"`
	TokenHash  string         `bun:"token_hash,notnull"`
	Prefix     string         `bun:"prefix,notnull"`
	ExpiresAt  sql.NullTime   `bun:"expires_at"`
	LastUsedAt sql.NullTime   `bun:"last_used_at"`
	LastUsedIP sql.NullString `bun:"last_used_ip"`
	LastUsedUA sql.NullString `bun:"last_used_ua"`
	RevokedAt  sql.NullTime   `bun:"revoked_at,nullzero"`
	CreatedAt  time.Time      `bun:"created_at,notnull"`
}

// TableName satisfies bun's table naming.
func (*Token) TableName() string { return "bi_token" }

// Token kind / user role literals. Kept as constants so the middleware, service
// and tests share one vocabulary (avoid string drift between "session"/"pat").
const (
	TokenKindSession = "session"
	TokenKindPAT     = "pat"

	RoleAdmin = "admin"
	RoleUser  = "user"
)
