import { useState, useEffect, useRef } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Clock, RefreshCw, Power, PowerOff,
  Loader2, Trash2, ToggleLeft, ToggleRight, ChevronDown,
  Activity, Zap, DollarSign, Server,
} from "lucide-react";
import { api, connectUsageStream, type QuotaAccount, type UpstreamQuota } from "../lib/api";
import { PageHeader } from "@/components/composite/page-header";
import { Card } from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";
import { EmptyState } from "@/components/composite/empty-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Progress } from "@/components/ui/progress";
import { StatusDot } from "@/components/composite/status-dot";
import { StatCard } from "@/components/composite/stat-card";
import { useToast } from "../components/Toast";

const periods = [
  { value: "today", label: "Today" },
  { value: "week", label: "Last 7 days" },
  { value: "month", label: "Last 30 days" },
];

const DEPLETED_THRESHOLD = 5;
const REFRESH_INTERVAL = 10_000; // 10s near-real-time refresh

const statusMeta: Record<string, { label: string; tone: "success" | "warning" | "danger" }> = {
  active: { label: "Active", tone: "success" },
  paused: { label: "Paused", tone: "warning" },
  needs_attention: { label: "Needs attention", tone: "danger" },
};

function fmtTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return n.toLocaleString();
}

function getColor(remainingPct: number) {
  if (remainingPct > 70) return { bar: "bg-emerald-500", text: "text-emerald-500", emoji: "🟢" };
  if (remainingPct >= 30) return { bar: "bg-amber-500", text: "text-amber-500", emoji: "🟡" };
  return { bar: "bg-red-500", text: "text-red-500", emoji: "🔴" };
}

function formatCountdown(resetAt: string | number | undefined | null): string {
  if (!resetAt) return "-";
  try {
    const resetTime = typeof resetAt === "number" ? resetAt : new Date(resetAt).getTime();
    const diff = resetTime - Date.now();
    if (diff <= 0) return "now";
    const days = Math.floor(diff / 86400000);
    const hours = Math.floor((diff % 86400000) / 3600000);
    const mins = Math.floor((diff % 3600000) / 60000);
    if (days > 0) return `${days}d ${hours}h`;
    if (hours > 0) return `${hours}h ${mins}m`;
    return `${mins}m`;
  } catch { return "-"; }
}

function isDepleted(account: QuotaAccount): boolean {
  const quotas = account.upstream_quotas ?? [];
  if (quotas.length === 0) return false;
  return quotas.some((q) => {
    const pct = q.limit > 0 ? Math.round((q.remaining / q.limit) * 100) : 100;
    return pct < DEPLETED_THRESHOLD;
  });
}

function earliestReset(account: QuotaAccount): number | null {
  let earliest: number | null = null;
  for (const q of account.upstream_quotas ?? []) {
    if (q.reset_at) {
      const t = new Date(q.reset_at).getTime();
      if (!isNaN(t) && (earliest === null || t < earliest)) earliest = t;
    }
  }
  return earliest;
}

export function QuotaPage() {
  const [period, setPeriod] = useState("month");
  const [providerFilter, setProviderFilter] = useState("all");
  const [statusFilter, setStatusFilter] = useState("all");
  const [expiringFirst, setExpiringFirst] = useState(false);
  const [autoRefresh, setAutoRefresh] = useState(() => localStorage.getItem("quotaAutoRefresh") !== "false");
  const [countdown, setCountdown] = useState(REFRESH_INTERVAL / 1000);
  const countdownRef = useRef(REFRESH_INTERVAL / 1000);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [activeTab, setActiveTab] = useState<"paid" | "free">("paid");
  const [expandedProviders, setExpandedProviders] = useState<Set<string>>(new Set());
  const qc = useQueryClient();
  const toast = useToast();

  const toggleProvider = (provider: string) => {
    setExpandedProviders(prev => {
      const next = new Set(prev);
      if (next.has(provider)) next.delete(provider);
      else next.add(provider);
      return next;
    });
  };

  const quota = useQuery({
    queryKey: ["quota", period],
    queryFn: () => api.quota(period),
    refetchInterval: autoRefresh ? REFRESH_INTERVAL : false,
  });

  // Subscribe to SSE usage stream for instant cache invalidation. When a new
  // usage record is inserted (e.g. a chain-mode request completes), the meter
  // publishes to the hub, which pushes an SSE event here. We invalidate the
  // quota query so the next render shows fresh data without waiting for the
  // polling interval.
  useEffect(() => {
    return connectUsageStream(() => {
      qc.invalidateQueries({ queryKey: ["quota"] });
    });
  }, [qc]);

  const accounts = quota.data?.accounts ?? [];
  const providers = [...new Set(accounts.map((a) => a.provider))].sort();

  const filtered = accounts.filter((a) => {
    if (providerFilter !== "all" && a.provider !== providerFilter) return false;
    if (statusFilter === "active" && a.status !== "active") return false;
    if (statusFilter === "paused" && a.status === "active") return false;
    return true;
  });

  const sorted = [...filtered].sort((a, b) => {
    if (expiringFirst) {
      const ae = earliestReset(a), be = earliestReset(b);
      if (ae !== null && be !== null) return ae - be;
      if (ae !== null) return -1;
      if (be !== null) return 1;
    }
    const ah = (a.upstream_quotas?.length ?? 0) > 0, bh = (b.upstream_quotas?.length ?? 0) > 0;
    if (ah && !bh) return -1;
    if (!ah && bh) return 1;
    const ar = a.upstream_quotas?.[0] ? a.upstream_quotas[0].remaining / Math.max(1, a.upstream_quotas[0].limit) : 1;
    const br = b.upstream_quotas?.[0] ? b.upstream_quotas[0].remaining / Math.max(1, b.upstream_quotas[0].limit) : 1;
    return ar - br;
  });

  // Selection
  const toggleSelect = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const selectAll = () => {
    if (selected.size === sorted.length) setSelected(new Set());
    else setSelected(new Set(sorted.map((a) => a.id)));
  };

  // Countdown timer
  useEffect(() => {
    if (!autoRefresh) return;
    countdownRef.current = REFRESH_INTERVAL / 1000;
    setCountdown(REFRESH_INTERVAL / 1000);
    const iv = setInterval(() => {
      countdownRef.current -= 1;
      if (countdownRef.current <= 0) countdownRef.current = REFRESH_INTERVAL / 1000;
      setCountdown(countdownRef.current);
    }, 1000);
    return () => clearInterval(iv);
  }, [autoRefresh, quota.dataUpdatedAt]);

  // Pause when tab hidden
  useEffect(() => {
    if (!autoRefresh) return;
    const h = () => { if (document.hidden) qc.cancelQueries({ queryKey: ["quota"] }); };
    document.addEventListener("visibilitychange", h);
    return () => document.removeEventListener("visibilitychange", h);
  }, [autoRefresh, qc]);

  useEffect(() => { localStorage.setItem("quotaAutoRefresh", String(autoRefresh)); }, [autoRefresh]);

  // Mutations
  const toggleAccount = useMutation({
    mutationFn: ({ id, disabled }: { id: string; disabled: boolean }) => api.updateAccount(id, { disabled }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["quota"] }),
  });

  const deleteAccount = useMutation({
    mutationFn: (id: string) => api.deleteAccount(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["quota"] });
      toast.success("Account removed", "The upstream provider account has been deleted and its secrets purged.");
    },
    onError: (e: Error) => toast.error("Account removal failed", e.message),
  });

  const handleBulkEnable = () => {
    const ids = [...selected].filter((id) => sorted.find((a) => a.id === id)?.status === "paused");
    ids.forEach((id) => toggleAccount.mutate({ id, disabled: false }));
    toast.success("Accounts enabled", `${ids.length} paused account${ids.length !== 1 ? "s" : ""} reactivated for upstream routing.`);
    setSelected(new Set());
  };

  const handleBulkDisable = () => {
    const ids = [...selected].filter((id) => sorted.find((a) => a.id === id)?.status === "active");
    ids.forEach((id) => toggleAccount.mutate({ id, disabled: true }));
    toast.success("Accounts disabled", `${ids.length} active account${ids.length !== 1 ? "s" : ""} paused. Traffic will bypass them.`);
    setSelected(new Set());
  };

  const handleBulkDelete = () => {
    const ids = [...selected];
    ids.forEach((id) => deleteAccount.mutate(id));
    toast.success("Accounts deleted", `${ids.length} account${ids.length !== 1 ? "s" : ""} permanently removed and secrets purged.`);
    setSelected(new Set());
  };

  const handleTurnOffEmpty = () => {
    const depleted = sorted.filter((a) => a.status === "active" && isDepleted(a));
    depleted.forEach((a) => toggleAccount.mutate({ id: a.id, disabled: true }));
    toast.success("Depleted accounts paused", `${depleted.length} account${depleted.length !== 1 ? "s" : ""} with exhausted quotas have been disabled to prevent errors.`);
  };

  const handleTurnOnAvailable = () => {
    const avail = sorted.filter((a) => a.status === "paused" && !isDepleted(a));
    avail.forEach((a) => toggleAccount.mutate({ id: a.id, disabled: false }));
    toast.success("Available accounts reactivated", `${avail.length} paused account${avail.length !== 1 ? "s" : ""} with remaining quota have been re-enabled.`);
  };

  const handleRefresh = () => {
    qc.invalidateQueries({ queryKey: ["quota"] });
    countdownRef.current = REFRESH_INTERVAL / 1000;
    setCountdown(REFRESH_INTERVAL / 1000);
    toast.success("Quota data refreshed", "Upstream quota information has been re-fetched from all providers.");
  };

  const depletedCount = sorted.filter((a) => a.status === "active" && isDepleted(a)).length;
  const pausedAvailCount = sorted.filter((a) => a.status === "paused" && !isDepleted(a)).length;

  // Aggregate stats across all accounts
  const totalRequests = accounts.reduce((s, a) => s + a.total_requests, 0);
  const totalPromptTokens = accounts.reduce((s, a) => s + a.prompt_tokens, 0);
  const totalCompletionTokens = accounts.reduce((s, a) => s + a.completion_tokens, 0);
  const totalCost = accounts.reduce((s, a) => s + a.cost_usd, 0);
  const activeCount = accounts.filter((a) => a.status === "active").length;

  return (
    <>
      <PageHeader
        title="Quota Tracker"
        icon={Clock}
        description="Monitor upstream quota limits and consumption per connected account."
        action={
          <Select value={period} onValueChange={setPeriod}>
            <SelectTrigger className="h-8 w-32 border-border bg-background text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {periods.map((p) => (
                <SelectItem key={p.value} value={p.value}>{p.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        }
      />

      {/* Summary stat cards */}
      {!quota.isLoading && accounts.length > 0 && (
        <div className="mb-5 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-5">
          <StatCard label="Total Requests" value={totalRequests.toLocaleString()} icon={Activity} iconTone="accent" />
          <StatCard label="Input Tokens" value={fmtTokens(totalPromptTokens)} icon={Zap} iconTone="accent" />
          <StatCard label="Output Tokens" value={fmtTokens(totalCompletionTokens)} icon={Zap} iconTone="accent" />
          <StatCard label="Total Cost" value={`$${totalCost.toFixed(2)}`} icon={DollarSign} iconTone="warning" />
          <StatCard label="Active Accounts" value={`${activeCount} / ${accounts.length}`} icon={Server} iconTone="accent" />
        </div>
      )}

      <Card>
        {/* ── Toolbar ──────────────────────────────────────────── */}
        <div className="flex flex-wrap items-center gap-2 border-b border-border px-4 py-2.5">
          {/* Select all */}
          <Button variant="ghost" size="sm" onClick={selectAll} className="flex items-center gap-2 text-xs text-muted-foreground hover:text-foreground">
            <input type="checkbox" checked={selected.size === sorted.length && sorted.length > 0} onChange={selectAll}
              className="h-3.5 w-3.5 rounded border-border accent-primary cursor-pointer" />
            Select all
          </Button>

          <div className="mx-1 h-4 w-px bg-border" />

          {/* Filters */}
          <Select value={providerFilter} onValueChange={setProviderFilter}>
            <SelectTrigger className="h-8 w-36 border-border bg-background text-xs">
              <SelectValue placeholder="All providers" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All providers</SelectItem>
              {providers.map((p) => (
                <SelectItem key={p} value={p} className="capitalize">{p}</SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Select value={statusFilter} onValueChange={setStatusFilter}>
            <SelectTrigger className="h-8 w-32 border-border bg-background text-xs">
              <SelectValue placeholder="All accounts" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All accounts</SelectItem>
              <SelectItem value="active">Active</SelectItem>
              <SelectItem value="paused">Paused</SelectItem>
            </SelectContent>
          </Select>
          <Button 
            variant={expiringFirst ? "default" : "ghost"} 
            size="sm"
            onClick={() => setExpiringFirst(!expiringFirst)}
            className={expiringFirst ? "" : ""}
          >
            <Clock className="h-3 w-3" />
            <span className="hidden sm:inline">Expiring</span>
          </Button>

          {/* Semantic bulk actions */}
          {depletedCount > 0 && (
            <Button variant="destructive" size="sm" onClick={handleTurnOffEmpty}>
              <PowerOff className="h-3 w-3" />
              <span className="hidden sm:inline">Off Empty</span> ({depletedCount})
            </Button>
          )}
          {pausedAvailCount > 0 && (
            <Button 
              variant="default" 
              size="sm"
              onClick={handleTurnOnAvailable}
              className="border-accent-500/30 text-accent-500 hover:bg-accent-500/10"
            >
              <Power className="h-3 w-3" />
              <span className="hidden sm:inline">On Avail</span> ({pausedAvailCount})
            </Button>
          )}

          <div className="flex-1" />

          {/* Selection bulk actions */}
          {selected.size > 0 && (
            <div className="flex items-center gap-1">
              <Button variant="default" size="sm" onClick={handleBulkEnable} className="border-accent-500/30 text-accent-500 hover:bg-accent-500/10">
                <ToggleRight className="h-3 w-3" /> Enable
              </Button>
              <Button variant="secondary" size="sm" onClick={handleBulkDisable} className="border-amber-500/30 text-amber-500 hover:bg-amber-500/10">
                <ToggleLeft className="h-3 w-3" /> Disable
              </Button>
              <Button variant="destructive" size="sm" onClick={handleBulkDelete}>
                <Trash2 className="h-3 w-3" /> Delete
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setSelected(new Set())} className="text-on-surface-variant hover:text-on-surface">
                Clear
              </Button>
            </div>
          )}

          {/* Auto-refresh + manual refresh */}
          <Button 
            variant={autoRefresh ? "default" : "ghost"} 
            size="sm"
            onClick={() => setAutoRefresh(!autoRefresh)}
            className={autoRefresh ? "border-accent-500/40 bg-accent-500/10 text-accent-500" : ""}
          >
            <RefreshCw className={`h-3 w-3 ${autoRefresh ? "animate-spin" : ""}`} style={{ animationDuration: "3s" }} />
            {autoRefresh ? `${countdown}s` : "off"}
          </Button>
          <Button variant="ghost" size="icon-sm" onClick={handleRefresh} className="text-on-surface-variant hover:bg-surface-container">
            <RefreshCw className="h-3 w-3" />
          </Button>
          <span className="text-xs text-muted-foreground">{sorted.length}</span>
        </div>

        {/* ── Tabs ─────────────────────────────────────────────── */}
        <div className="flex gap-4 border-b border-border px-4">
          <Button
            variant="ghost"
            className={`py-2.5 text-xs font-semibold border-b-2 rounded-none transition-colors ${
              activeTab === "paid" 
                ? "border-primary text-foreground" 
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
            onClick={() => setActiveTab("paid")}
          >
            Paid Providers
          </Button>
          <Button
            variant="ghost"
            className={`py-2.5 text-xs font-semibold border-b-2 rounded-none transition-colors ${
              activeTab === "free" 
                ? "border-primary text-foreground" 
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
            onClick={() => setActiveTab("free")}
          >
            Free Providers
          </Button>
        </div>

        {/* ── Account list ─────────────────────────────────────── */}
        {quota.isLoading ? (
          <div className="flex items-center justify-center py-12"><Spinner /></div>
        ) : sorted.length === 0 ? (
          <EmptyState title="No connected accounts" hint="Add a provider account to start tracking quota usage." />
        ) : (
          <div className="flex flex-col">
            {(() => {
              // Filter by tab
              const tabAccounts = sorted.filter(a => activeTab === "paid" ? a.usage_type === "credit" : a.usage_type === "token");
              
              if (tabAccounts.length === 0) {
                return (
                  <div className="py-12 text-center text-sm text-muted-foreground">
                    No {activeTab} provider accounts found.
                  </div>
                );
              }

              // Group accounts by provider
              const tabProviders = [...new Set(tabAccounts.map(a => a.provider))].sort();
              const groupedAccounts = tabProviders.map(p => {
                const pAccounts = tabAccounts.filter(a => a.provider === p);
                return {
                  provider: p,
                  provider_name: pAccounts[0]?.provider_name || p,
                  accounts: pAccounts
                };
              });

              return groupedAccounts.map((group, i) => (
                <div key={group.provider} className={i > 0 ? "border-t border-border" : ""}>
                  <div 
                    className="flex cursor-pointer select-none items-center gap-2 border-b border-border bg-muted/30 px-4 py-2 hover:bg-muted transition-colors"
                    onClick={() => toggleProvider(group.provider)}
                  >
                    <ChevronDown className={`h-4 w-4 text-muted-foreground transition-transform ${expandedProviders.has(group.provider) ? "rotate-180" : ""}`} />
                    <ProviderIcon provider={group.provider} className="h-5 w-5" />
                    <span className="text-sm font-semibold capitalize text-foreground">{group.provider_name}</span>
                    <span className="text-xs text-muted-foreground bg-popover px-1.5 py-0.5 rounded-full border border-border">
                      {group.accounts.length}
                    </span>
                  </div>
                  {expandedProviders.has(group.provider) && (
                    <div className="divide-y divide-border">
                      {group.accounts.map((account) => (
                        <QuotaRow
                          key={account.id}
                          account={account}
                          selected={selected.has(account.id)}
                          onSelect={() => toggleSelect(account.id)}
                          onToggle={() => toggleAccount.mutate({ id: account.id, disabled: account.status !== "paused" })}
                          onDelete={() => deleteAccount.mutate(account.id)}
                          hideProviderIcon
                        />
                      ))}
                    </div>
                  )}
                </div>
              ));
            })()}
          </div>
        )}
      </Card>
    </>
  );
}

// ─── Quota Row (compact single-line layout) ──────────────────────────────────

function QuotaRow({
  account,
  selected,
  onSelect,
  onToggle,
  onDelete,
  hideProviderIcon,
}: {
  account: QuotaAccount;
  selected: boolean;
  onSelect: () => void;
  onToggle: () => void;
  onDelete: () => void;
  hideProviderIcon?: boolean;
}) {
  const qc = useQueryClient();
  const toast = useToast();
  const meta = statusMeta[account.status] ?? statusMeta.active;
  const quotas = account.upstream_quotas ?? [];
  const hasQuota = quotas.length > 0;
  const depleted = isDepleted(account);
  const [expanded, setExpanded] = useState(false);

  const refreshMut = useMutation({
    mutationFn: () => api.accountQuota(account.id),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["quota"] }); toast.success("Quota refreshed", "Upstream quota data re-fetched for this account."); },
    onError: (e: Error) => toast.error("Quota refresh failed", e.message),
  });

  // Compute summary stats from quotas
  const avgRemaining = quotas.length > 0
    ? Math.round(quotas.reduce((s, q) => s + (q.limit > 0 ? (q.remaining / q.limit) * 100 : 100), 0) / quotas.length)
    : 100;
  const color = getColor(avgRemaining);
  const reset = formatCountdown(earliestReset(account));

  return (
    <div className={`transition-colors hover:bg-muted/50 ${account.status === "paused" ? "opacity-60" : ""}`}>
      {/* Main row */}
      <div className="flex items-center gap-3 px-4 py-2.5">
        {/* Checkbox */}
        <input
          type="checkbox"
          checked={selected}
          onChange={onSelect}
          className="h-3.5 w-3.5 shrink-0 rounded border-border accent-accent-600"
        />

        {/* Provider icon */}
        {!hideProviderIcon && <ProviderIcon provider={account.provider} />}

        {/* Info */}
        <div className="min-w-0 flex-1 pl-1">
          <div className="flex items-center gap-1.5">
            {!hideProviderIcon && <span className="text-sm font-semibold capitalize truncate">{account.provider_name}</span>}
            <span className={`text-sm ${hideProviderIcon ? "font-medium" : "text-muted-foreground"} truncate`}>
              {account.label || account.auth_kind}
            </span>
            {account.plan_name && <Badge variant="default">{account.plan_name}</Badge>}
            {depleted && account.status === "active" && <Badge variant="destructive">depleted</Badge>}
            {account.status !== "active" && (
              <span className="inline-flex items-center gap-1">
                <StatusDot tone={meta.tone} />
                <span className="text-[11px] font-medium">{meta.label}</span>
              </span>
            )}
          </div>
          <p className="truncate text-xs text-muted-foreground">
            {!hideProviderIcon && `${account.label || account.auth_kind} `}
            {hasQuota ? `${!hideProviderIcon ? "· " : ""}${quotas.length} quota${quotas.length !== 1 ? "s" : ""}` : (!hideProviderIcon ? "" : "No quota configured")}
          </p>
        </div>

        {/* Inline quota summary (if has quotas) */}
        {hasQuota && (
          <div className="hidden items-center gap-3 sm:flex">
            <div className="w-24 space-y-1">
              <Progress value={Math.max(2, 100 - avgRemaining)} indicatorClassName={color.bar} className="h-1.5" />
              <p className="text-[10px] text-muted-foreground">{avgRemaining}% left</p>
            </div>
            {reset !== "-" && (
              <span className={`text-[11px] ${avgRemaining < 30 ? "font-medium text-red-500" : "text-muted-foreground"}`}>
                resets {reset}
              </span>
            )}
          </div>
        )}

        {/* Stats — always show request/token/cost; credit accounts also show quota inline */}
        <div className="hidden items-center gap-3 text-[11px] text-muted-foreground md:flex">
          <span className="flex items-center gap-1">
            <Activity className="h-3 w-3" />
            <span className="font-medium text-foreground">{account.total_requests.toLocaleString()}</span> req
          </span>
          <span className="flex items-center gap-1">
            <Zap className="h-3 w-3" />
            {fmtTokens(account.prompt_tokens + account.completion_tokens)} tok
          </span>
          <span className="flex items-center gap-1">
            <DollarSign className="h-3 w-3" />
            <span className="font-medium tabular-nums text-foreground">${account.cost_usd.toFixed(4)}</span>
          </span>
          {account.usage_type === "credit" && (() => {
            const q = account.upstream_quotas?.[0];
            if (!q) return null;
            const remainingPct = q.limit > 0 ? Math.round((q.remaining / q.limit) * 100) : null;
            return (
              <>
                <div className="mx-0.5 h-3 w-px bg-border" />
                <span>
                  <span className="font-medium text-foreground">{q.used.toLocaleString()}</span>
                  <span className="mx-0.5">/</span>
                  <span>{q.limit > 0 ? q.limit.toLocaleString() : "∞"}</span>
                  <span className="ml-0.5">used</span>
                </span>
                <span>
                  <span className={`font-medium ${remainingPct !== null && remainingPct < 30 ? "text-red-500" : remainingPct !== null && remainingPct < 70 ? "text-amber-500" : "text-accent-500"}`}>
                    {q.remaining.toLocaleString()}
                  </span>
                  <span className="ml-0.5">left</span>
                </span>
              </>
            );
          })()}
        </div>

        {/* Actions */}
        <div className="flex shrink-0 items-center gap-0.5">
          <Button variant="ghost" size="icon-sm" onClick={() => refreshMut.mutate()} disabled={refreshMut.isPending} title="Refresh">
            {refreshMut.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
          </Button>
          <Button 
            variant="ghost" 
            size="icon-sm"
            onClick={onToggle}
            title={account.status === "paused" ? "Enable" : "Disable"}
          >
            {account.status === "paused" ? <ToggleLeft className="h-3.5 w-3.5" /> : <ToggleRight className="h-3.5 w-3.5 text-accent-500" />}
          </Button>
          <Button variant="ghost" size="icon-sm" onClick={onDelete} className="hover:bg-red-500/10 hover:text-red-500" title="Delete">
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
          {hasQuota && (
            <Button 
              variant="ghost" 
              size="icon-sm"
              onClick={() => setExpanded(!expanded)}
              title={expanded ? "Collapse" : "Expand"}
            >
              <ChevronDown className={`h-3.5 w-3.5 transition-transform ${expanded ? "rotate-180" : ""}`} />
            </Button>
          )}
        </div>
      </div>

      {/* Expanded quota details */}
      {expanded && hasQuota && (
        <div className="border-t border-border bg-muted/30 px-4 py-2">
          <QuotaTable quotas={quotas} />
          {/* Mobile stats (hidden on md+) — always show req/tokens/cost */}
          <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground md:hidden">
            <span>{account.total_requests.toLocaleString()} requests</span>
            <span>{fmtTokens(account.prompt_tokens + account.completion_tokens)} tokens</span>
            <span className="font-medium tabular-nums">${account.cost_usd.toFixed(4)}</span>
            {account.usage_type === "credit" && (() => {
              const q = account.upstream_quotas?.[0];
              if (!q) return null;
              const remainingPct = q.limit > 0 ? Math.round((q.remaining / q.limit) * 100) : null;
              return (
                <span>
                  {q.used.toLocaleString()} / {q.limit > 0 ? q.limit.toLocaleString() : "∞"} used
                  {" · "}
                  <span className={remainingPct !== null && remainingPct < 30 ? "text-red-500" : remainingPct !== null && remainingPct < 70 ? "text-amber-500" : "text-accent-500"}>
                    {q.remaining.toLocaleString()} left
                  </span>
                </span>
              );
            })()}
          </div>
        </div>
      )}
    </div>
  );
}

// ─── Quota Table (compact inline detail) ─────────────────────────────────────

function QuotaTable({ quotas }: { quotas: UpstreamQuota[] }) {
  const [page, setPage] = useState(0);
  const PAGE_SIZE = 8;
  const totalPages = Math.ceil(quotas.length / PAGE_SIZE);
  const pageQuotas = quotas.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);

  return (
    <div>
      <table className="w-full table-fixed">
        <tbody>
          {pageQuotas.map((q) => (
            <QuotaDetailRow key={q.resource_type} quota={q} />
          ))}
        </tbody>
      </table>
      {quotas.length > PAGE_SIZE && (
        <div className="mt-1 flex items-center justify-between rounded-md border border-border bg-muted px-2 py-1">
          <span className="text-[10px] text-muted-foreground">
            {quotas.length} quotas · Page {page + 1}/{totalPages}
          </span>
          <div className="flex items-center gap-1">
            <Button variant="ghost" size="sm" onClick={() => setPage(Math.max(0, page - 1))} disabled={page === 0} className="text-[10px]">
              Prev
            </Button>
            <Button variant="ghost" size="sm" onClick={() => setPage(Math.min(totalPages - 1, page + 1))} disabled={page >= totalPages - 1} className="text-[10px]">
              Next
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

function QuotaDetailRow({ quota }: { quota: UpstreamQuota }) {
  const usedPct = quota.limit > 0 ? Math.min(100, Math.round((quota.used / quota.limit) * 100)) : 0;
  const remainingPct = quota.limit > 0 ? Math.round((quota.remaining / quota.limit) * 100) : 0;
  const color = getColor(remainingPct);
  const countdown = formatCountdown(quota.reset_at);
  const name = quota.resource_type.toLowerCase().replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());

  return (
    <tr className="border-b border-black/5 dark:border-white/5 hover:bg-black/[0.02] dark:hover:bg-white/[0.02]">
      <td className="w-[30%] truncate py-1 px-1.5">
        <div className="flex items-center gap-1.5">
          <span className="text-[10px]">{color.emoji}</span>
          <span className="truncate text-[11px] font-medium">{name}</span>
        </div>
      </td>
      <td className="w-[45%] py-1 px-1.5">
        <div className="space-y-0.5">
          <div className="h-1 w-full overflow-hidden rounded-full bg-muted">
            <div className={`h-full rounded-full ${color.bar}`} style={{ width: `${Math.max(2, usedPct)}%` }} />
          </div>
          <div className="flex items-center justify-between text-[10px] text-muted-foreground">
            <span>{quota.used.toLocaleString()} / {quota.limit > 0 ? quota.limit.toLocaleString() : "∞"}</span>
            <span className={`font-semibold ${color.text}`}>{remainingPct}%</span>
          </div>
        </div>
      </td>
      <td className="w-[25%] py-1 px-1.5 text-right">
        <span className={`text-[11px] ${remainingPct < 30 ? "font-medium text-red-500" : "text-muted-foreground"}`}>
          {countdown !== "-" ? `resets ${countdown}` : "-"}
        </span>
      </td>
    </tr>
  );
}

// ─── Provider Icon ───────────────────────────────────────────────────────────

function ProviderIcon({ provider, className }: { provider: string, className?: string }) {
  const [errored, setErrored] = useState(false);
  const sizeClass = className || "h-7 w-7";
  if (errored) {
    return (
      <div className={`flex shrink-0 items-center justify-center rounded-md bg-muted text-[10px] font-bold text-muted-foreground ${sizeClass}`}>
        {provider.slice(0, 2).toUpperCase()}
      </div>
    );
  }
  return (
    <img src={`/providers/${provider}.png`} alt={provider} onError={() => setErrored(true)}
      className="h-7 w-7 shrink-0 rounded-md object-contain" />
  );
}
