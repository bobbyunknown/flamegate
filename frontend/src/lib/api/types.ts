
export interface RegionOption {
  id: string;
  label: string;
  base_url: string;
}

export interface Provider {
  id: string;
  display_name: string;
  alias: string;
  dialect: string;
  auth_kind: string;
  auth_modes: string[];
  service_kinds: string[];
  color: string;
  website: string;
  api_key_url: string;
  icon: string;
  deprecated: boolean;
  hidden: boolean;
  pinned: boolean;
  notice: string;
  drivable: boolean;
  input_per_m: number;
  output_per_m: number;
  regions?: RegionOption[];
  default_region?: string;
  // base_url is populated for user-defined custom provider instances.
  base_url?: string;
  // custom marks user-defined dynamic provider instances (editable/deletable).
  custom?: boolean;
}

// ProviderModel is a single model entry returned by providerModels(). Custom
// models carry a db_id so they can be edited/removed; discovered marks models
// that came from the upstream /models endpoint rather than the static catalog.
export interface ProviderModel {
  id: string;
  name: string;
  kind: string;
  custom?: boolean;
  db_id?: string;
  discovered?: boolean;
  tier?: string;
  tags?: string[];
  context_window?: number;
  max_output_tokens?: number;
  input_modalities?: string[];
  output_modalities?: string[];
  vision?: boolean;
  reasoning?: boolean;
  tools?: boolean;
}

export interface TestModelResult {
  status: "ok" | "error";
  latency_ms: number;
  response_text?: string;
  error?: string;
}

export interface TestChatMessage {
  role: "user" | "assistant" | "system";
  content: string;
}

export interface TestChatResult {
  status: "ok" | "error";
  response_text?: string;
  latency_ms: number;
  prompt_tokens?: number;
  completion_tokens?: number;
  model?: string;
  error?: string;
}

// CustomProvider is a user-defined provider instance (OpenAI- or Anthropic-
// compatible) with its own unique id, base URL, accounts, and models.
export interface CustomProvider {
  id: string;
  display_name: string;
  alias: string;
  dialect: string; // "openai" | "anthropic"
  base_url: string;
  custom: true;
  created_at?: string;
  updated_at?: string;
}

// CustomModel is a user-registered model on a provider (custom or built-in).
export interface CustomModel {
  db_id: string;
  provider_id: string;
  id: string;
  name: string;
  kind: string;
  context_window: number;
  input_per_m: number;
  output_per_m: number;
}

export interface CustomModelInput {
  id: string;
  name?: string;
  kind?: string;
  context_window?: number;
  input_per_m?: number;
  output_per_m?: number;
}

// Extension is an installed WASM provider module (admin /extensions API).
export interface Extension {
  id: string;
  slug: string;
  name: string;
  version: string;
  description: string;
  state: string; // ACTIVE | DISABLED
  entrypoints?: string;
  capabilities?: string;
  last_error?: string;
  /** When true, install/enable runs list_models into extension_models. */
  auto_sync_models: boolean;
  model_count: number;
  installed_at?: string;
  updated_at?: string;
}


export interface BrandingSettings {
  name: string;
  logo_url: string;
  favicon_url: string;
  tagline: string;
  color_palette: string;
}

export interface EndpointSettings {
  rtk_enabled: boolean;
  rtk_filter_level: string;
  caveman_enabled: boolean;
  caveman_level: string;
  terse_enabled: boolean;
  terse_level: string;
  headroom_enabled: boolean;
  headroom_url: string;
  headroom_compress_user_messages: boolean;
  headroom_timeout_ms: number;
  ponytail_enabled: boolean;
  ponytail_level: "lite" | "full" | "ultra";
  routing_strategy: string;
  sticky_limit: number;
  combo_strategy: string;
  combo_sticky_limit: number;
  outbound_proxy_enabled: boolean;
  outbound_proxy_url: string;
  outbound_no_proxy: string;
  observability_enabled?: boolean;
  rate_limits_enabled: boolean;
  stream_stall_timeout_ms: number;
  response_header_timeout_ms: number;
  request_timeout_ms: number;
}

export interface ProviderRoutingSettings {
  routing_strategy: "inherit" | "fill-first" | "round-robin" | "smart-round-robin" | string;
  sticky_limit: number;
  affinity_ttl_minutes: number;
}

// HeadroomTestResult is returned by POST /settings/headroom-test and reports
// whether the configured Headroom proxy is reachable and behaving correctly.
// endpoint is always masked (no credentials/query string).
export interface HeadroomTestResult {
  ok: boolean;
  reachable: boolean;
  status: number;
  latency_ms: number;
  endpoint: string;
  message: string;
}

export interface OAuthProvider {
  provider: string;
  display_name: string;
  flow: string; // authorization_code_pkce | authorization_code | device_code
  icon: string;
  color: string;
  callback_path?: string;
  fixed_port?: number;
  loopback_host?: string;
}

export interface DeviceCode {
  device_code: string;
  user_code: string;
  verification_uri: string;
  verification_uri_complete: string;
  expires_in: number;
  interval: number;
  // Client-device-code step 1 response (browser must make the upstream call).
  _client_device_code?: boolean;
  _pkce_challenge?: string;
  _pkce_nonce?: string;
  _device_code_url?: string;
  _client_id?: string;
  _scopes?: string[];
  _pkce_method?: string;
}

export interface OAuthPollResult {
  status: string; // pending | complete
  slow_down?: boolean;
  id?: string;
  provider?: string;
}

export interface Plan {
  id: string;
  name: string;
  description: string;
  limit_micros: number;
  limit_tokens: number;
  rpm_limit: number;
  tpm_limit: number;
  concurrency_limit: number;
  period: string;
  alert_pct: number;
  hard_cutoff: boolean;
  allowed_models: string[] | null;
  key_count: number;
  created_at: string;
  updated_at: string;
}

export interface APIKey {
  id: string;
  name: string;
  display: string;
  disabled: boolean;
  plan_id: string;
  plan_name?: string;
  created_at: string;
  allowed_models?: string[];
}

export interface CreatedKey {
  id: string;
  name: string;
  key: string;
  display: string;
  plan_id: string;
  budget?: {
    id: string;
    scope_kind: string;
    limit_micros: number;
    limit_tokens: number;
    period: string;
    alert_pct: number;
    hard_cutoff: boolean;
  };
  allowed_models?: string[];
  plan?: {
    id: string;
    name: string;
  };
}

export interface Account {
  id: string;
  provider: string;
  label: string;
  auth_kind: string;
  priority: number;
  disabled: boolean;
  proxy_pool_id?: string;
  needs_reconnect?: boolean;
  created_at: string;
}

export interface AccountInput {
  provider: string;
  label: string;
  api_key?: string;
  base_url?: string;
  region?: string;
  account_id?: string;
  azure_endpoint?: string;
  azure_deployment?: string;
  azure_api_version?: string;
  azure_organization?: string;
  proxy_pool_id?: string;
  priority?: number;
}

// BulkAccountItem is one credential in a bulk import. Only api_key (and an
// optional per-item base_url / label) varies per row; shared provider config
// lives on BulkAccountInput.
export interface BulkAccountItem {
  label?: string;
  api_key?: string;
  base_url?: string;
}

export interface BulkAccountInput {
  provider: string;
  base_url?: string;
  region?: string;
  account_id?: string;
  azure_endpoint?: string;
  azure_deployment?: string;
  azure_api_version?: string;
  azure_organization?: string;
  priority?: number;
  proxy_pool_id?: string;
  validate?: boolean;
  items: BulkAccountItem[];
}

export interface BulkAccountResult {
  index: number;
  label: string;
  status: "created" | "error" | "skipped";
  id?: string;
  error?: string;
}

export interface BulkAccountResponse {
  total: number;
  created: number;
  failed: number;
  skipped: number;
  results: BulkAccountResult[];
}

export interface ChainStep {
  provider: string;
  model: string;
  position: number;
}

export interface Chain {
  id: string;
  name: string;
  strategy: string;
  fallback_provider?: string;
  fallback_model?: string;
  steps: ChainStep[];
}

export interface Budget {
  id: string;
  scope_kind: string;
  scope_id: string;
  limit_micros: number;
  limit_tokens: number;
  period: string;
  alert_pct: number;
  hard_cutoff: boolean;
}

export interface BudgetStatus {
  id: string;
  scope_kind: string;
  scope_id: string;
  scope_name: string;
  limit_micros: number;
  limit_tokens: number;
  period: string;
  alert_pct: number;
  hard_cutoff: boolean;
  spent_micros: number;
  spent_tokens: number;
  pct_used: number;
  tokens_pct_used: number;
  period_start: string;
}

export interface UsageSummary {
  total_requests: number;
  prompt_tokens: number;
  completion_tokens: number;
  cached_tokens: number;
  cost_usd: number;
  cache_hits: number;
  since: string;
}

export interface ProviderUsage {
  provider: string;
  display_name: string;
  color: string;
  icon: string;
  total_requests: number;
  prompt_tokens: number;
  completion_tokens: number;
  cost_usd: number;
  share_pct: number;
}

export interface RecentActivity {
  id: string;
  provider: string;
  model: string;
  tokens: number;
  cost_usd: number;
  cache_hit: boolean;
  latency_ms: number;
  created_at: string;
  ttft_ms?: number;
  slim_bytes_saved?: number;
  slim_tokens_saved?: number;
  slim_rules?: string;
  caveman_active?: boolean;
  terse_active?: boolean;
}

export interface RuleSaving {
  rule: string;
  count: number;
  bytes_saved: number;
  tokens_saved: number;
}

export interface ClientSaving {
  client: string;
  requests: number;
  bytes_saved: number;
  tokens_saved: number;
  usd_saved: number;
  caveman_requests: number;
  terse_requests: number;
  // Headroom/Ponytail per-client savings. Optional for backward-compat with
  // payloads recorded before these savers existed; treat missing as 0.
  headroom_tokens_saved?: number;
  ponytail_requests?: number;
}

export interface TokenSavings {
  slim_bytes_saved: number;
  slim_tokens_saved: number;
  caveman_requests: number;
  terse_requests: number;
  usd_saved?: number;
  usd_saved_estimate?: boolean;
  // Headroom/Ponytail summary savings. Optional for backward-compat with
  // payloads recorded before these savers existed; treat missing as 0.
  headroom_tokens_saved?: number;
  ponytail_requests?: number;
  headroom_requests?: number;
  rules: RuleSaving[];
  by_client?: ClientSaving[];
}

export interface ModelUsage {
  provider: string;
  provider_name: string;
  model: string;
  total_requests: number;
  prompt_tokens: number;
  completion_tokens: number;
  cost_usd: number;
  input_per_m?: number;
  output_per_m?: number;
  cached_input_per_m?: number;
}

export interface SeriesPoint {
  label: string;
  count: number;
}

export interface UsageInsights {
  summary: {
    total_requests: number;
    prompt_tokens: number;
    completion_tokens: number;
    cached_tokens: number;
    cost_usd: number;
    cache_hits: number;
    success_rate: number;
    avg_latency_ms: number;
    avg_ttft_ms: number;
    since: string;
  };
  savings: TokenSavings;
  providers: ProviderUsage[];
  recent: RecentActivity[];
  series: SeriesPoint[];
  busiest: string;
}

export interface UpstreamQuota {
  resource_type: string;
  used: number;
  limit: number;
  remaining: number;
  reset_at?: string;
}

export interface QuotaAccount {
  id: string;
  provider: string;
  provider_name: string;
  label: string;
  auth_kind: string;
  priority: number;
  status: string; // active | paused | needs_attention
  usage_type: string; // token | credit
  total_requests: number;
  prompt_tokens: number;
  completion_tokens: number;
  cached_tokens: number;
  cost_usd: number;
  input_per_m: number;
  output_per_m: number;
  notice?: string;
  plan_name?: string;
  message?: string;
  upstream_quotas?: UpstreamQuota[];
  updated_at: string;
}

// Console log uses structured entries streamed via SSE (/api/console/stream)
// and fetched as history from /api/console, which returns { logs: ConsoleLogEntry[] }.
export interface ConsoleLogEntry {
  seq: number;
  time: string; // HH:MM:SS.mmm
  level: string; // DEBUG | INFO | WARN | ERROR | LOG
  msg: string; // human-readable summary
  detail?: string; // optional technical detail, revealed on expand
}

export interface ProxyPool {
  id: string;
  name: string;
  type: string; // http | vercel | cloudflare | deno
  proxy_url: string;
  no_proxy: string;
  strict: boolean;
  is_active: boolean;
  test_status: string; // unknown | active | error
  last_tested?: string;
  last_error?: string;
}

export interface Skill {
  id: string;
  name: string;
  description: string;
  prompt: string;
  enabled: boolean;
  created_at: string;
}

export interface AccessSettings {
  local_enabled: boolean;
  tunnel_enabled: boolean;
  tailscale_enabled: boolean;
  tunnel_url?: string;
  tailscale_url?: string;
  endpoint_url: string;
}

export interface TunnelStatus {
  enabled: boolean;
  settingsEnabled: boolean;
  tunnelUrl: string;
  shortId: string;
  publicUrl: string;
  running: boolean;
}

export interface TailscaleStatus {
  enabled: boolean;
  settingsEnabled: boolean;
  tunnelUrl: string;
  running: boolean;
  loggedIn: boolean;
  installed: boolean;
  platform: string;
}

export interface TunnelCombinedStatus {
  tunnel: TunnelStatus;
  tailscale: TailscaleStatus;
  download: { downloading: boolean; progress: number };
}

export interface TunnelEnableResult {
  success: boolean;
  tunnelUrl: string;
  shortId: string;
  publicUrl: string;
  alreadyRunning?: boolean;
}

export interface TailscaleCheckResult {
  installed: boolean;
  loggedIn: boolean;
  platform: string;
  daemonRunning: boolean;
  hasCachedPassword: boolean;
}

export interface TailscaleEnableResult {
  success: boolean;
  tunnelUrl?: string;
  needsLogin?: boolean;
  authUrl?: string;
  funnelNotEnabled?: boolean;
  enableUrl?: string;
  error?: string;
}

export interface CLITool {
  id: string;
  name: string;
  dialect: string;
  instructions: string;
  snippet: string;
  installed: boolean;
  configured: boolean;
  config_path: string;
}

export interface CLIToolsResponse {
  base_url: string;
  model: string;
  tools: CLITool[];
}

export interface AuthStatus {
  authenticated: boolean;
  username?: string;
  using_default: boolean;
  onboarding_complete: boolean;
}

export interface SystemSnapshot {
  cpu_pct: number;
  cpu_per_core: number[];
  mem_total_mb: number;
  mem_used_mb: number;
  mem_available_mb: number;
  mem_pct: number;
  disk_total_gb: number;
  disk_used_gb: number;
  disk_free_gb: number;
  disk_pct: number;
  goroutines: number;
  heap_alloc_mb: number;
  heap_sys_mb: number;
  heap_inuse_mb: number;
  heap_idle_mb: number;
  gc_pause_total_ms: number;
  gc_pause_last_ms: number;
  gc_cycles: number;
  open_fds: number;
  net_conns: number;
  uptime_s: number;
  pid: number;
  host: string;
  os: string;
  arch: string;
  // Process-level metrics
  proc_cpu_pct: number;
  proc_rss_mb: number;
  proc_threads: number;
  proc_open_fds: number;
}

export interface SystemSample {
  ts: number;
  cpu_pct: number;
  mem_pct: number;
  goroutines: number;
  heap_mb: number;
  cpu_spike?: boolean;
  mem_spike?: boolean;
  // Process-level metrics
  proc_cpu_pct?: number;
  proc_rss_mb?: number;
  proc_threads?: number;
  proc_open_fds?: number;
}

export interface SystemHistory {
  interval_sec: number;
  max_size: number;
  spikes: SystemSample[];
  samples: SystemSample[];
}

// ============================================================================
// Guardrails
// ============================================================================

export type GuardrailScope = "global" | "provider" | "model" | "chain" | "apikey";
export type GuardrailAction = "allow" | "log_only" | "warn" | "mask" | "block";
export type GuardrailSeverity = "low" | "medium" | "high";
export type PIIStrategy = "redact" | "replace" | "mask" | "hash" | "block" | "anonymize";

export interface PIIConfig {
  enabled: boolean;
  types?: string[];
  strategy?: PIIStrategy;
  min_score?: number;
  scan_output?: boolean;
  engine?: string;
}

export interface InjectionConfig {
  enabled: boolean;
  severity_threshold?: GuardrailSeverity;
  action?: GuardrailAction;
}

export interface TopicsConfig {
  enabled: boolean;
  mode?: "allow" | "block";
  topics?: string[];
  action?: GuardrailAction;
  engine?: "keyword" | "embedding";
  similarity_threshold?: number;
}

export interface ToxicityConfig {
  enabled: boolean;
  categories?: string[];
  threshold?: number;
  action?: GuardrailAction;
  engine?: "native" | "openai";
}

export interface BiasConfig {
  enabled: boolean;
  categories?: string[];
  threshold?: number;
  action?: GuardrailAction;
}

export interface GuardrailPolicyConfig {
  enabled?: boolean;
  pii?: PIIConfig;
  injection?: InjectionConfig;
  topics?: TopicsConfig;
  toxicity?: ToxicityConfig;
  bias?: BiasConfig;
}

export interface GuardrailPolicy {
  id: string;
  name: string;
  scope: GuardrailScope;
  scope_id: string;
  enabled: boolean;
  config: GuardrailPolicyConfig;
  created_at: string;
  updated_at: string;
}

export interface GuardrailFinding {
  entity: string;
  score: number;
  start: number;
  end: number;
  original?: string;
  redacted?: string;
}

export interface GuardrailDecision {
  detector: string;
  action: GuardrailAction;
  severity?: GuardrailSeverity;
  reason?: string;
  findings?: GuardrailFinding[];
  direction?: "inbound" | "outbound";
}

export interface GuardrailTestResult {
  action: GuardrailAction;
  reason: string;
  decisions: GuardrailDecision[];
}

export interface GuardrailLogEntry {
  id: string;
  request_id: string;
  api_key_id: string;
  provider: string;
  model: string;
  chain_id: string;
  detector: string;
  direction: "inbound" | "outbound";
  action: GuardrailAction;
  severity: GuardrailSeverity | "";
  reason: string;
  findings: GuardrailFinding[] | null;
  created_at: string;
}

export interface EffectiveGuardrail {
  scope: {
    tenant_id?: string;
    provider?: string;
    model?: string;
    chain_id?: string;
    apikey_id?: string;
  };
  policy: GuardrailPolicyConfig;
}

export interface UpdateInfo {
  current: string;
  latest: string;
  update_available: boolean;
  changelog: string;
  published_at: string;
  html_url: string;
  checked: boolean;
}

export interface SQLiteStatus {
  available: boolean;
  dialect: string;
  path?: string;
}

export interface SQLiteRestoreResult {
  ok: boolean;
  restart_required: boolean;
  safety_backup: string;
}

export interface KeyUsageData {
  key_id: string;
  key_name: string;
  budgets: {
    period: string;
    limit_tokens: number;
    tokens_used: number;
    tokens_remaining: number;
    tokens_pct_used: number;
    limit_usd: number;
    spent_usd: number;
    usd_remaining: number;
    usd_pct_used: number;
    alert: boolean;
  }[];
  allowed_models: string[];
  current_period: {
    prompt_tokens: number;
    completion_tokens: number;
    total_requests: number;
    cost_usd: number;
  };
  daily?: {
    date: string;
    requests: number;
    prompt_tokens: number;
    completion_tokens: number;
    cost_usd: number;
  }[];
  models?: {
    provider: string;
    model: string;
    total_requests: number;
    prompt_tokens: number;
    completion_tokens: number;
    cost_usd: number;
  }[];
}

export interface GuardrailTemplate {
  id: string;
  name: string;
  description: string;
  config: GuardrailPolicyConfig;
}

export interface GuardrailBundle {
  version: number;
  exported_at?: string;
  policies: Array<{
    name: string;
    scope: string;
    scope_id?: string;
    enabled: boolean;
    config: GuardrailPolicyConfig;
  }>;
}

export interface UsageEvent {
  provider: string;
  model: string;
  account_id: string;
  tokens: number;
}
