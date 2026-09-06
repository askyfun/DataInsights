package share

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"dataray/internal/domain/entity"
	"dataray/internal/model"

	"crypto/rand"
	"encoding/hex"
	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is the bcrypt work factor used for share passwords.
const bcryptCost = 10

// Service defines the interface for share operations
type Service interface {
	// Create a new share link
	Create(ctx context.Context, chartID int, password *string, expiresAt *string) (*entity.Share, error)

	// List all shares
	List(ctx context.Context) ([]entity.Share, error)

	// Get share by token
	GetByToken(ctx context.Context, token string) (*entity.Share, error)

	// Validate password if required
	ValidatePassword(ctx context.Context, token, password string) error
}

// shareService implements the Service interface
type shareService struct {
	db *bun.DB
}

// NewService creates a new share service
func NewService(db *bun.DB) Service {
	return &shareService{db: db}
}

// Create creates a new share link
func (s *shareService) Create(ctx context.Context, chartID int, password *string, expiresAt *string) (*entity.Share, error) {
	token := generateToken()

	m := &model.Share{
		Token:   token,
		ChartID: chartID,
	}

	if password != nil && *password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcryptCost)
		if err != nil {
			return nil, fmt.Errorf("hash share password: %w", err)
		}
		m.Password = sql.NullString{String: string(hash), Valid: true}
	}
	if expiresAt != nil && *expiresAt != "" {
		t, err := time.Parse(time.RFC3339, *expiresAt)
		if err == nil {
			m.ExpiresAt = sql.NullTime{Time: t, Valid: true}
		}
	}

	if _, err := s.db.NewInsert().Model(m).Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to create share: %w", err)
	}
	return toShareEntity(m), nil
}

// List returns all shares
func (s *shareService) List(ctx context.Context) ([]entity.Share, error) {
	var shares []model.Share
	if err := s.db.NewSelect().Model(&shares).Order("id DESC").Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to list shares: %w", err)
	}
	result := make([]entity.Share, len(shares))
	for i, m := range shares {
		result[i] = *toShareEntity(&m)
	}
	return result, nil
}

// GetByToken returns a share by token
func (s *shareService) GetByToken(ctx context.Context, token string) (*entity.Share, error) {
	share := &model.Share{}
	if err := s.db.NewSelect().Model(share).Where("token = ?", token).Scan(ctx); err != nil {
		return nil, fmt.Errorf("share not found: %w", err)
	}
	return toShareEntity(share), nil
}

// ValidatePassword validates the password for a share. Stored bcrypt hashes are
// verified with bcrypt.CompareHashAndPassword; legacy plaintext values are
// compared directly and transparently upgraded to a bcrypt hash on success.
func (s *shareService) ValidatePassword(ctx context.Context, token, password string) error {
	m := &model.Share{}
	if err := s.db.NewSelect().Model(m).Where("token = ?", token).Scan(ctx); err != nil {
		return fmt.Errorf("share not found: %w", err)
	}

	if !m.Password.Valid || m.Password.String == "" {
		return nil
	}
	stored := m.Password.String

	if strings.HasPrefix(stored, "$2") {
		if err := bcrypt.CompareHashAndPassword([]byte(stored), []byte(password)); err != nil {
			return fmt.Errorf("invalid password")
		}
		return nil
	}

	// Legacy plaintext storage: compare directly, then upgrade to bcrypt.
	if password != stored {
		return fmt.Errorf("invalid password")
	}
	return s.upgradeLegacyPassword(ctx, m.ID, password)
}

// upgradeLegacyPassword writes a bcrypt hash of the verified plaintext back to
// the share, mirroring the datasource legacy-password auto-upgrade flow.
func (s *shareService) upgradeLegacyPassword(ctx context.Context, id int, plaintext string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcryptCost)
	if err != nil {
		return fmt.Errorf("upgrade legacy share password: %w", err)
	}
	m := &model.Share{ID: id, Password: sql.NullString{String: string(hash), Valid: true}}
	if _, err := s.db.NewUpdate().Model(m).Column("password").WherePK().Exec(ctx); err != nil {
		return fmt.Errorf("upgrade legacy share password: %w", err)
	}
	slog.Info("upgraded legacy plaintext share password", "share_id", id)
	return nil
}

// Helper functions

func generateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		now := time.Now().UnixNano()
		return fmt.Sprintf("%d-%d", now, now%100000000)
	}
	return fmt.Sprintf("%s-%x", hex.EncodeToString(b[:8]), b[8:])
}

// Conversion functions

func toShareEntity(m *model.Share) *entity.Share {
	e := &entity.Share{
		ID:          m.ID,
		Token:       m.Token,
		ChartID:     m.ChartID,
		HasPassword: m.Password.Valid && m.Password.String != "",
	}
	if m.Password.Valid {
		e.Password = &m.Password.String
	}
	if m.ExpiresAt.Valid {
		t := m.ExpiresAt.Time.Format(time.RFC3339)
		e.ExpiresAt = &t
	}
	if m.CreatedAt.Valid {
		e.CreatedAt = m.CreatedAt.Time.Format(time.RFC3339)
	}
	return e
}
