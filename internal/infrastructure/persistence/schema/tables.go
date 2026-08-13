package schema

import "time"

// Tenant scopes all data in multi-tenant deployments.
type Tenant struct {
	ID        string    `gorm:"primaryKey;type:varchar(64)"`
	Name      string    `gorm:"type:varchar(255)"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

// APIKey is a stored inbound credential.
type APIKey struct {
	ID         string `gorm:"primaryKey;type:varchar(64)"`
	TenantID   string `gorm:"type:varchar(64);index"`
	ProjectID  string `gorm:"type:varchar(64)"`
	PlanID     string `gorm:"type:varchar(64)"`
	Name       string `gorm:"type:varchar(255)"`
	KeyHash    string `gorm:"type:varchar(255)"`
	LookupHash string `gorm:"type:varchar(255);uniqueIndex"`
	Display    string `gorm:"type:varchar(32)"`
	Scopes     string `gorm:"type:text"`
	Disabled   bool   `gorm:"default:false"`
	LastUsedAt *time.Time
	CreatedAt  time.Time `gorm:"autoCreateTime"`
}

// APIKeyModelAccess is a per-key model gating row.
type APIKeyModelAccess struct {
	APIKeyID  string    `gorm:"primaryKey;type:varchar(64)"`
	Model     string    `gorm:"primaryKey;type:varchar(255)"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

// Plan is a reusable template for budget limits and model restrictions.
type Plan struct {
	ID               string    `gorm:"primaryKey;type:varchar(64)"`
	TenantID         string    `gorm:"type:varchar(64);index"`
	Name             string    `gorm:"type:varchar(255)"`
	Description      string    `gorm:"type:text"`
	LimitMicros      int64     `gorm:"default:0"`
	LimitTokens      int64     `gorm:"default:0"`
	RPMLimit         int64     `gorm:"default:0"`
	TPMLimit         int64     `gorm:"default:0"`
	ConcurrencyLimit int64     `gorm:"default:0"`
	Period           string    `gorm:"type:varchar(32)"`
	AlertPct         int       `gorm:"default:0"`
	HardCutoff       bool      `gorm:"default:false"`
	AllowedModels    string    `gorm:"type:text"`
	CreatedAt        time.Time `gorm:"autoCreateTime"`
	UpdatedAt        time.Time `gorm:"autoUpdateTime"`
}

// Account holds an upstream provider credential.
type Account struct {
	ID                string `gorm:"primaryKey;type:varchar(64)"`
	TenantID          string `gorm:"type:varchar(64);index"`
	Provider          string `gorm:"type:varchar(128);index"`
	Label             string `gorm:"type:varchar(255)"`
	AuthKind          string `gorm:"type:varchar(32)"`
	SecretWrappedDEK  string `gorm:"type:text"`
	SecretCiphertext  string `gorm:"type:text"`
	TokenWrappedDEK   string `gorm:"type:text"`
	TokenCiphertext   string `gorm:"type:text"`
	RefreshWrappedDEK string `gorm:"type:text"`
	RefreshCiphertext string `gorm:"type:text"`
	TokenExpiresAt    *time.Time
	Metadata          string `gorm:"type:text"`
	Priority          int    `gorm:"default:0"`
	BackoffLevel      int    `gorm:"default:0"`
	Disabled          bool   `gorm:"default:false"`
	CooldownUntil     *time.Time
	ProxyPoolID       string    `gorm:"type:varchar(64)"`
	NeedsReconnect    bool      `gorm:"default:false"`
	CreatedAt         time.Time `gorm:"autoCreateTime"`
	UpdatedAt         time.Time `gorm:"autoUpdateTime"`
}

// Chain is an ordered fallback definition.
type Chain struct {
	ID               string      `gorm:"primaryKey;type:varchar(64)"`
	TenantID         string      `gorm:"type:varchar(64);index"`
	Name             string      `gorm:"type:varchar(255)"`
	Strategy         string      `gorm:"type:varchar(32)"`
	FallbackProvider string      `gorm:"type:varchar(128)"`
	FallbackModel    string      `gorm:"type:varchar(255)"`
	Steps            []ChainStep `gorm:"foreignKey:ChainID"`
	CreatedAt        time.Time   `gorm:"autoCreateTime"`
	UpdatedAt        time.Time   `gorm:"autoUpdateTime"`
}

// ChainStep is one candidate target within a chain.
type ChainStep struct {
	ID        string `gorm:"primaryKey;type:varchar(64)"`
	ChainID   string `gorm:"type:varchar(64);index"`
	Position  int
	Provider  string    `gorm:"type:varchar(128)"`
	Model     string    `gorm:"type:varchar(255)"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

// UsageRecord meters one completed request.
type UsageRecord struct {
	ID                  string `gorm:"primaryKey;type:varchar(64)"`
	TenantID            string `gorm:"type:varchar(64);index"`
	ProjectID           string `gorm:"type:varchar(64)"`
	APIKeyID            string `gorm:"type:varchar(64);index"`
	Provider            string `gorm:"type:varchar(128);index"`
	Model               string `gorm:"type:varchar(255);index"`
	AccountID           string `gorm:"type:varchar(64);index"`
	Client              string `gorm:"type:varchar(128)"`
	PromptTokens        int
	CompletionTokens    int
	CachedTokens        int
	CacheWriteTokens    int
	CostMicros          int64
	CacheHit            bool
	LatencyMS           int
	TTFTMS              int `gorm:"column:ttft_ms"`
	SlimBytesSaved      int
	SlimTokensSaved     int
	SlimRules           string `gorm:"type:text"`
	CavemanActive       bool
	TerseActive         bool
	HeadroomTokensSaved int
	HeadroomBytesSaved  int
	HeadroomActive      bool
	PonytailActive      bool
	CreatedAt           time.Time `gorm:"autoCreateTime;index"`
}

// Budget enforces a spend and/or token limit over a period.
type Budget struct {
	ID          string    `gorm:"primaryKey;type:varchar(64)"`
	TenantID    string    `gorm:"type:varchar(64);index"`
	ScopeKind   string    `gorm:"type:varchar(32)"`
	ScopeID     string    `gorm:"type:varchar(64)"`
	LimitMicros int64     `gorm:"default:0"`
	LimitTokens int64     `gorm:"default:0"`
	Period      string    `gorm:"type:varchar(32)"`
	AlertPct    int       `gorm:"default:0"`
	HardCutoff  bool      `gorm:"default:false"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

// AuditEntry is one append-only audit record.
type AuditEntry struct {
	ID        string    `gorm:"primaryKey;type:varchar(64)"`
	TenantID  string    `gorm:"type:varchar(64);index"`
	Actor     string    `gorm:"type:varchar(255)"`
	Action    string    `gorm:"type:varchar(128)"`
	Target    string    `gorm:"type:varchar(255)"`
	Detail    string    `gorm:"type:text"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

// ModelCooldown locks a specific model on an account.
type ModelCooldown struct {
	ID            string `gorm:"primaryKey;type:varchar(64)"`
	AccountID     string `gorm:"type:varchar(64);index"`
	Model         string `gorm:"type:varchar(255)"`
	CooldownUntil time.Time
	CreatedAt     time.Time `gorm:"autoCreateTime"`
}

// ChainRotation persists round-robin cursor for a routing chain.
type ChainRotation struct {
	ChainID   string `gorm:"primaryKey;type:varchar(64)"`
	LastIndex int
	HitCount  int
	UpdatedAt time.Time
}

// TargetRotation persists round-robin cursor for a provider/model target.
type TargetRotation struct {
	ScopeKey  string `gorm:"primaryKey;type:varchar(255)"`
	LastIndex int
	HitCount  int
	UpdatedAt time.Time
}

// AccountAffinity pins a conversation key to an account.
type AccountAffinity struct {
	ScopeKey  string `gorm:"primaryKey;type:varchar(255)"`
	AccountID string `gorm:"type:varchar(64)"`
	ExpiresAt time.Time
	UpdatedAt time.Time
}

// AccountHealth stores background probe status.
type AccountHealth struct {
	ID                   string `gorm:"primaryKey;type:varchar(64)"`
	TenantID             string `gorm:"type:varchar(64);index"`
	AccountID            string `gorm:"type:varchar(64);index"`
	Provider             string `gorm:"type:varchar(128)"`
	Model                string `gorm:"type:varchar(255)"`
	Status               string `gorm:"type:varchar(32)"`
	LatencyMS            int
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
	LastOKAt             *time.Time
	LastCheckedAt        time.Time
	LastError            string `gorm:"type:text"`
	UpdatedAt            time.Time
}

// ResourceSample is one resource_samples row.
type ResourceSample struct {
	ID                 int64     `gorm:"primaryKey;autoIncrement"`
	TenantID           string    `gorm:"type:varchar(64);index"`
	CreatedAt          time.Time `gorm:"autoCreateTime;index"`
	Goroutines         int64
	HeapAllocBytes     int64
	HeapSysBytes       int64
	GCPauseNS          int64
	NextGCBytes        int64
	NumGC              int64
	ProcCPUPercent     float64
	ProcRSSBytes       int64
	ProcThreads        int64
	ProcOpenFDs        *int64
	HostCPUPercent     float64
	HostMemUsedBytes   int64
	HostMemTotalBytes  int64
	HostDiskUsedBytes  int64
	HostDiskTotalBytes int64
	HostNetSentBytes   int64
	HostNetRecvBytes   int64
	HostLoad1          *float64
	HostLoad5          *float64
	HostLoad15         *float64
	InflightRequests   int64
}

// GuardrailPolicy is a stored safety policy.
type GuardrailPolicy struct {
	ID        string    `gorm:"primaryKey;type:varchar(64)"`
	TenantID  string    `gorm:"type:varchar(64);index"`
	Scope     string    `gorm:"type:varchar(32)"`
	ScopeID   string    `gorm:"type:varchar(64)"`
	Name      string    `gorm:"type:varchar(255)"`
	Enabled   bool      `gorm:"default:true"`
	Config    string    `gorm:"type:text"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// GuardrailLog is one detector decision recorded for audit.
type GuardrailLog struct {
	ID        string    `gorm:"primaryKey;type:varchar(64)"`
	TenantID  string    `gorm:"type:varchar(64);index"`
	RequestID string    `gorm:"type:varchar(64)"`
	APIKeyID  string    `gorm:"type:varchar(64)"`
	Provider  string    `gorm:"type:varchar(128)"`
	Model     string    `gorm:"type:varchar(255)"`
	ChainID   string    `gorm:"type:varchar(64)"`
	Detector  string    `gorm:"type:varchar(32)"`
	Direction string    `gorm:"type:varchar(16)"`
	Action    string    `gorm:"type:varchar(16)"`
	Severity  string    `gorm:"type:varchar(16)"`
	Reason    string    `gorm:"type:text"`
	Findings  string    `gorm:"type:text"`
	CreatedAt time.Time `gorm:"autoCreateTime;index"`
}

// ResourceBucket is one aggregated time slice for the resources timeline.
type ResourceBucket struct {
	ID             int64 `gorm:"primaryKey;autoIncrement"`
	Bucket         int
	ProcCPUAvg     float64
	ProcCPUMax     float64
	HostCPUAvg     float64
	HostCPUMax     float64
	ProcRSSAvg     float64
	ProcRSSMax     int64
	HostMemUsedAvg float64
	HostMemUsedMax int64
	GoroutinesAvg  float64
	GoroutinesMax  int64
	HeapAllocAvg   float64
	HeapAllocMax   int64
	NetSentDelta   int64
	NetRecvDelta   int64
	GCPauseAvg     float64
	GCPauseMax     int64
	InflightAvg    float64
	InflightMax    int64
}

// ModelAlias maps a bare name to a provider/model target.
type ModelAlias struct {
	Alias  string `gorm:"primaryKey;type:varchar(255)"`
	Target string `gorm:"type:varchar(255)"`
}

// ProxyPool is a named proxy endpoint.
type ProxyPool struct {
	ID         string `gorm:"primaryKey;type:varchar(64)"`
	Name       string `gorm:"type:varchar(255)"`
	Type       string `gorm:"type:varchar(32)"`
	ProxyURL   string `gorm:"type:text"`
	NoProxy    string `gorm:"type:text"`
	Strict     bool   `gorm:"default:false"`
	IsActive   bool   `gorm:"default:true"`
	TestStatus string `gorm:"type:varchar(32)"`
	LastTested *time.Time
	LastError  string    `gorm:"type:text"`
	CreatedAt  time.Time `gorm:"autoCreateTime"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime"`
}

// Settings is a key/value configuration store.
type Settings struct {
	Key   string `gorm:"primaryKey;type:varchar(255)"`
	Value string `gorm:"type:text"`
}

// CustomProvider is a user-defined provider instance.
type CustomProvider struct {
	ID          string    `gorm:"primaryKey;type:varchar(64)"`
	TenantID    string    `gorm:"type:varchar(64);index"`
	DisplayName string    `gorm:"type:varchar(255)"`
	Alias       string    `gorm:"type:varchar(128)"`
	Dialect     string    `gorm:"type:varchar(32)"`
	BaseURL     string    `gorm:"type:text"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

// User is a dashboard operator account. FlameGate seeds a default admin
// on first run; the onboarding flow prompts a password change.
type User struct {
	ID           string `gorm:"primaryKey;size:36"`
	Username     string `gorm:"uniqueIndex;size:64;not null"`
	DisplayName  string `gorm:"size:128;not null"`
	PasswordHash string `gorm:"size:255;not null"`
	Status       string `gorm:"size:16;not null;default:active"` // active | disabled
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// CustomModel is a user-registered model on a provider.
type CustomModel struct {
	ID            string `gorm:"primaryKey;type:varchar(64)"`
	TenantID      string `gorm:"type:varchar(64);index"`
	ProviderID    string `gorm:"type:varchar(64);index"`
	ModelID       string `gorm:"type:varchar(255)"`
	DisplayName   string `gorm:"type:varchar(255)"`
	Kind          string `gorm:"type:varchar(32)"`
	ContextWindow int
	InputPerM     float64
	OutputPerM    float64
	CreatedAt     time.Time `gorm:"autoCreateTime"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime"`
}

// Extension is an installed WASM provider extension.
type Extension struct {
	ID           string     `gorm:"primaryKey;type:varchar(64)"`
	TenantID     string     `gorm:"type:varchar(64);index"`
	Slug         string     `gorm:"uniqueIndex;type:varchar(128)"`
	Name         string     `gorm:"type:varchar(255)"`
	Version      string     `gorm:"type:varchar(32)"`
	Description  string     `gorm:"type:text"`
	WasmPath     string     `gorm:"type:text"`
	SchemaPath   string     `gorm:"type:text"`
	State        string     `gorm:"type:varchar(16);not null;default:PENDING"`
	Capabilities string     `gorm:"type:text"`
	Entrypoints  string     `gorm:"type:text"`
	Config       string     `gorm:"type:text"`
	DefaultAccountKey string `gorm:"type:varchar(255)"`
	// AutoSyncModels controls whether install/enable runs list_models into
	// extension_models. Default true; user can disable per extension.
	AutoSyncModels bool `gorm:"not null;default:true"`
	LastError    string     `gorm:"type:text"`
	CompiledAt   *time.Time `gorm:"autoCreateTime:false"`
	InstalledAt  time.Time  `gorm:"autoCreateTime"`
	UpdatedAt    time.Time  `gorm:"autoUpdateTime"`
}

// ExtensionModel is a model registered by an extension (discovered or custom).
type ExtensionModel struct {
	ID            string `gorm:"primaryKey;type:varchar(64)"`
	ExtensionID   string `gorm:"type:varchar(64);index"`
	TenantID      string `gorm:"type:varchar(64);index"`
	Slug          string `gorm:"type:varchar(255)"`
	DisplayName   string `gorm:"type:varchar(255)"`
	Source        string `gorm:"type:varchar(16);not null;default:discovered"`
	ContextWindow int
	MaxOutput     int
	Enabled       bool      `gorm:"default:true"`
	Metadata      string    `gorm:"type:text"`
	CreatedAt     time.Time `gorm:"autoCreateTime"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime"`
}
