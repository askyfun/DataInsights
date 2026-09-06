package entity

// Share represents a shared chart link
type Share struct {
	ID      int    `json:"id"`
	Token   string `json:"token"`
	ChartID int    `json:"chart_id"`
	// Password never leaves the API surface: it holds the plaintext only
	// transiently on create and the bcrypt hash after storage. has_password is
	// the only password-related signal exposed to clients.
	Password  *string `json:"-"`
	ExpiresAt *string `json:"expires_at"`
	CreatedAt string  `json:"created_at"`
	// HasPassword tells clients whether the share is password protected.
	HasPassword bool `json:"has_password"`
}

// ShareService defines operations for share management
type ShareService interface {
	// Create a new share link
	Create(chartID int, password *string, expiresAt *string) (*Share, error)

	// Get share by token
	GetByToken(token string) (*Share, error)

	// Validate password if required
	ValidatePassword(token, password string) error
}
