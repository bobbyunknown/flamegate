// Package guardrail defines the Guardrail aggregate.
package guardrail

import "time"

// Action is the enforcement action for a guardrail policy.
type Action string

const (
	ActionBlock Action = "block"
	ActionMask  Action = "mask"
	ActionLog   Action = "log"
	ActionWarn  Action = "warn"
)

// Scope identifies which dimension a policy targets.
type Scope string

const (
	ScopeGlobal   Scope = "global"
	ScopeProvider Scope = "provider"
	ScopeModel    Scope = "model"
	ScopeChain    Scope = "chain"
	ScopeAPIKey   Scope = "apikey"
)

// Policy is a stored safety policy.
type Policy struct {
	ID        string
	TenantID  string
	Scope     Scope
	ScopeID   string
	Name      string
	Enabled   bool
	Config    string // JSON blob
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsGlobal reports whether this policy applies globally.
func (p *Policy) IsGlobal() bool {
	return p.Scope == ScopeGlobal && p.ScopeID == ""
}
