package entity

// User is the API-facing user record. It deliberately carries NO password or
// password_hash — those live only on model.User and never leave the service.
type User struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

// AuthResult is the payload of POST /api/auth/register and /api/auth/login:
// the freshly minted session token (plaintext, shown once) plus the user it
// belongs to.
type AuthResult struct {
	Token     string `json:"token"`
	Kind      string `json:"kind"`
	ExpiresAt string `json:"expires_at"`
	User      User   `json:"user"`
}

// TokenInfo is the API-facing PAT record for the account-settings list (#184).
// It NEVER carries token_hash or the plaintext — only what a user needs to
// recognise and revoke a token: name, a short non-secret prefix, timestamps,
// last-used context, and revoked state.
type TokenInfo struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at"`
	LastUsedIP string `json:"last_used_ip"`
	LastUsedUA string `json:"last_used_ua"`
	ExpiresAt  string `json:"expires_at"` // "" = never expires
	Revoked    bool   `json:"revoked"`
}

// PATCreateResult is the payload of POST /api/tokens: the plaintext token (the
// ONLY time it is ever returned) plus its persisted metadata.
type PATCreateResult struct {
	Token string    `json:"token"`
	Info  TokenInfo `json:"info"`
}
