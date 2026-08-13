// Package key defines the APIKey aggregate.
package key

import (
	"errors"
	"time"
)

var (
	ErrEmptyName = errors.New("key: name must not be empty")
	ErrEmptyID   = errors.New("key: id must not be empty")
	ErrRevoked   = errors.New("key: already revoked")
)

// Key is the APIKey aggregate root. It represents an inbound credential.
type Key struct {
	ID          string
	TenantID    string
	Name        string
	KeyHash     string // argon2id verifier
	LookupHash  string // sha-256 index
	Display     string // masked form
	PlanID      string
	Permissions string
	Disabled    bool
	RevokedAt   *time.Time
	LastUsedAt  *time.Time
	CreatedAt   time.Time
}

// NewKey constructs a Key with required fields validated.
func NewKey(id, tenantID, name, hash, lookup string, planID string, now time.Time) (*Key, error) {
	if id == "" {
		return nil, ErrEmptyID
	}
	if name == "" {
		return nil, ErrEmptyName
	}
	return &Key{
		ID:         id,
		TenantID:   tenantID,
		Name:       name,
		KeyHash:    hash,
		LookupHash: lookup,
		PlanID:     planID,
		CreatedAt:  now,
	}, nil
}

// Revoke marks the key as revoked. Returns ErrRevoked if already revoked.
func (k *Key) Revoke(now time.Time) error {
	if k.RevokedAt != nil {
		return ErrRevoked
	}
	k.RevokedAt = &now
	return nil
}

// IsRevoked reports whether the key has been revoked.
func (k *Key) IsRevoked() bool {
	return k.RevokedAt != nil
}

// HasPermission reports whether the key carries the given permission.
// An empty permissions string means all permissions are granted.
func (k *Key) HasPermission(perm string) bool {
	if k.Permissions == "" {
		return true
	}
	for _, p := range splitPerms(k.Permissions) {
		if p == perm || p == "*" {
			return true
		}
	}
	return false
}

func splitPerms(s string) []string {
	var out []string
	start := 0
	for i := range len(s) {
		if s[i] == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}
