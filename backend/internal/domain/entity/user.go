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
