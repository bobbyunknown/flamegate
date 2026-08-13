// Package auth manages dashboard authentication: user accounts with
// argon2id-hashed passwords and short-lived HMAC-signed session tokens.
//
// On first run a default admin/flamegate account is seeded. Session tokens
// are signed with a key persisted in settings so sessions survive restarts.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/golang-jwt/jwt/v5"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/shared/crypto"
)

// DefaultPassword is seeded on first run. The onboarding flow prompts the
// operator to change it; a warning is logged while it remains in use.
const DefaultPassword = "flamegate"

// DefaultUsername is the seeded admin account name.
const DefaultUsername = "admin"

// Settings keys used to persist auth state.
const (
	keySigningKey = "auth.signing_key"
	keyOnboarding = "onboarding.complete"
)

// ErrInvalidPassword is returned when a login password does not match.
var ErrInvalidPassword = errors.New("auth: invalid password")

// Service provides password and session operations backed by the users table.
type Service struct {
	users    *persistence.UserRepo
	settings *persistence.SettingsRepo
	// adminUser is cached after EnsureDefaults to avoid repeated lookups.
	adminUser  *schema.User
	signingKey []byte
	ttl        time.Duration
}

// New builds an auth Service. configKey, when non-empty, overrides the persisted
// signing key (e.g. from FLAMEGATE_SECURITY__JWT_SECRET).
func New(users *persistence.UserRepo, settings *persistence.SettingsRepo, configKey string, ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Service{users: users, settings: settings, signingKey: []byte(configKey), ttl: ttl}
}

// EnsureDefaults seeds a default admin user and signing key on first run. It
// returns true when the default user was just created, so the caller can
// warn the operator. It is safe to call on every startup.
func (s *Service) EnsureDefaults(ctx context.Context) (seededDefault bool, err error) {
	// Signing key: prefer configured value, else load/generate a persisted one.
	if len(s.signingKey) == 0 {
		key, kerr := s.loadOrCreateSigningKey(ctx)
		if kerr != nil {
			return false, kerr
		}
		s.signingKey = key
	}

	// User: seed default admin if none exists yet.
	u, gerr := s.users.GetByUsername(ctx, DefaultUsername)
	if errors.Is(gerr, schema.ErrNotFound) {
		hash, herr := crypto.HashPassword(DefaultPassword)
		if herr != nil {
			return false, herr
		}
		u = schema.User{
			ID:           uuid.NewString(),
			Username:     DefaultUsername,
			DisplayName:  "Administrator",
			PasswordHash: hash,
			Status:       "active",
		}
		if serr := s.users.Create(ctx, &u); serr != nil {
			return false, serr
		}
		_ = s.settings.Set(ctx, keyOnboarding, "false")
		s.adminUser = &u
		return true, nil
	} else if gerr != nil {
		return false, gerr
	}
	s.adminUser = &u
	return false, nil
}

func (s *Service) loadOrCreateSigningKey(ctx context.Context) ([]byte, error) {
	if v, err := s.settings.Get(ctx, keySigningKey); err == nil {
		decoded, derr := base64.StdEncoding.DecodeString(v)
		if derr == nil && len(decoded) >= 32 {
			return decoded, nil
		}
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("auth: generate signing key: %w", err)
	}
	if err := s.settings.Set(ctx, keySigningKey, base64.StdEncoding.EncodeToString(key)); err != nil {
		return nil, err
	}
	return key, nil
}

// VerifyPassword reports whether the given password matches the stored hash
// for the specified username. Returns the User on success so callers can
// embed claims.
func (s *Service) VerifyPassword(ctx context.Context, username, password string) (schema.User, bool, error) {
	u, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		return schema.User{}, false, err
	}
	if u.Status == "disabled" {
		return schema.User{}, false, nil
	}
	ok, err := crypto.VerifyPassword(password, u.PasswordHash)
	if err != nil {
		return schema.User{}, false, err
	}
	return u, ok, nil
}

// SetPassword changes the password for the given username.
func (s *Service) SetPassword(ctx context.Context, username, newPassword string) error {
	if len(newPassword) < 6 {
		return errors.New("auth: password must be at least 6 characters")
	}
	u, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		return err
	}
	hash, err := crypto.HashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.users.UpdatePassword(ctx, u.ID, hash)
}

// UsingDefaultPassword reports whether the admin user's password still matches
// the seeded default, so the UI can nudge the operator to change it.
func (s *Service) UsingDefaultPassword(ctx context.Context) bool {
	_, ok, err := s.VerifyPassword(ctx, DefaultUsername, DefaultPassword)
	return err == nil && ok
}

// OnboardingComplete reports whether the operator finished onboarding.
func (s *Service) OnboardingComplete(ctx context.Context) bool {
	v, err := s.settings.Get(ctx, keyOnboarding)
	return err == nil && v == "true"
}

// CompleteOnboarding marks onboarding finished.
func (s *Service) CompleteOnboarding(ctx context.Context) error {
	return s.settings.Set(ctx, keyOnboarding, "true")
}

// Claims is the JWT payload for dashboard sessions.
type Claims struct {
	jwt.RegisteredClaims
	Username string `json:"username,omitempty"`
}

// IssueSession mints a signed JWT session token valid for the configured TTL.
func (s *Service) IssueSession(username string) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "dashboard",
			Issuer:    "flamegate",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Username: username,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.signingKey)
}

// VerifySession reports whether a JWT session token is valid and unexpired.
func (s *Service) VerifySession(tokenStr string) bool {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.signingKey, nil
	})
	if err != nil {
		return false
	}
	_, ok := token.Claims.(*Claims)
	return ok && token.Valid
}

// SessionUsername returns the username embedded in a valid session token.
func (s *Service) SessionUsername(tokenStr string) (string, bool) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.signingKey, nil
	})
	if err != nil {
		return "", false
	}
	c, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return "", false
	}
	return c.Username, true
}

// TTL returns the session lifetime.
func (s *Service) TTL() time.Duration { return s.ttl }
