import { useState, useMemo, useEffect, useRef, useLayoutEffect } from "react";
import { Button } from "@/components/ui/button";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity, DollarSign, Zap, RefreshCw, TrendingUp, Clock, Box, TerminalSquare, ArrowUpDown, ArrowUp, ArrowDown, Search,
  Layers, ChevronRight, Shield, ArrowRight
} from "lucide-react";
import {
  AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid
} from "recharts";
import { api, connectUsageStream, type ProviderUsage, type RecentActivity, type ModelUsage, type SeriesPoint, type Chain, type Provider } from "../lib/api";
import { PageHeader } from "@/components/composite/page-header";
import { Spinner } from "@/components/ui/spinner";
import { ErrorCard } from "@/components/composite/error-card";
import { StatCard } from "@/components/composite/stat-card";
import { useToast } from "../components/Toast";
import { TokenSavingsBreakdown } from "../components/SavingsBreakdown";

const periods = [
  { value: "today", label: "Today" },
  { value: "24h", label: "24h" },
  { value: "week", label: "7D" },
  { value: "month", label: "30D" },
];

const USAGE_REFRESH_DEBOUNCE_MS = 8000;

export function UsagePage() {
  const [period, setPeriod] = useState("today");
  const qc = useQueryClient();
  const toast = useToast();
  const refreshTimer = useRef<number | null>(null);

  const insights = useQuery({
    queryKey: ["usage-insights", period],
    queryFn: () => api.usageInsights(period),
    staleTime: 12_000,
    refetchInterval: 60_000,
    placeholderData: (previous) => previous,
  });

  const modelUsage = useQuery({
    queryKey: ["usage-models", period],
    queryFn: () => api.modelUsage(period),
    staleTime: 12_000,
    refetchInterval: 60_000,
    placeholderData: (previous) => previous,
  });

  const chains = useQuery({
    queryKey: ["chains"],
    queryFn: () => api.listChains(),
    staleTime: 30_000,
  });

  const providerCatalog = useQuery({
    queryKey: ["providers"],
    queryFn: () => api.providers(),
    staleTime: 60_000,
  });

  useEffect(() => {
    const scheduleRefresh = () => {
      if (refreshTimer.current != null) return;
      refreshTimer.current = window.setTimeout(() => {
        refreshTimer.current = null;
        qc.invalidateQueries({ queryKey: ["usage-insights", period] });
        qc.invalidateQueries({ queryKey: ["usage-models", period] });
      }, USAGE_REFRESH_DEBOUNCE_MS);
    };
    return connectUsageStream(() => {
      scheduleRefresh();
    });
  }, [qc, period]);

  useEffect(() => {
    return () => {
      if (refreshTimer.current != null) {
        window.clearTimeout(refreshTimer.current);
      }
    };
  }, []);

  const handleRefresh = () => {
    qc.invalidateQueries({ queryKey: ["usage-insights", period] });
    qc.invalidateQueries({ queryKey: ["usage-models", period] });
    toast.success("Usage data refreshed", "All usage metrics and breakdowns have been re-fetched from the server.");
  };

  return (
    <>
      <PageHeader
        title="Usage"
        icon={Activity}
        description="Monitor request flow and provider distribution."
        action={
          <div className="flex items-center gap-2">
            <div className="flex items-center gap-1 rounded-lg border border-border bg-muted p-1">
              {periods.map((p) => (
                <Button
                  key={p.value}
                  variant={period === p.value ? "default" : "ghost"}
                  size="xs"
                  onClick={() => setPeriod(p.value)}
                >
                  {p.label}
                </Button>
              ))}
            </div>
            <Button
              variant="outline"
              size="icon"
              onClick={handleRefresh}
            >
              <RefreshCw className="h-4 w-4" />
            </Button>
          </div>
        }
      />

      {insights.isLoading ? <Spinner />
        : insights.isError ? <ErrorCard message="Failed to load usage. Is the backend running?" />
        : insights.data ? <UsageContent data={insights.data} models={modelUsage.data?.models ?? []} chains={chains.data?.chains ?? []} providerCatalog={providerCatalog.data?.providers ?? []} period={period} /> : null}
    </>
  );
}

function UsageContent({ data, models, chains, providerCatalog, period }: { data: any; models: ModelUsage[]; chains: Chain[]; providerCatalog: Provider[]; period: string }) {
  const { summary, savings, providers, recent, series } = data;

  return (
    <div className="space-y-8 pb-12">
      {/* Precision Minimalist Stat Grid */}
      <div className="grid grid-cols-2 lg:grid-cols-5 gap-4">
        <StatCard label="REQUESTS" value={fmtNum(summary.total_requests)} icon={Activity} />
        <StatCard label="INPUT TOKENS" value={fmtNum(summary.prompt_tokens)} icon={Zap} />
        <StatCard label="OUTPUT TOKENS" value={fmtNum(summary.completion_tokens)} icon={TrendingUp} />
        <StatCard label="EST. COST" value={`$${summary.cost_usd.toFixed(2)}`} icon={DollarSign} />
        
        <div className="flex flex-col justify-between p-5 rounded-xl border border-border bg-background shadow-sm transition-colors hover:bg-muted relative overflow-hidden group col-span-2 lg:col-span-1">
          <div className="absolute inset-0 bg-gradient-to-br from-emerald-500/5 to-transparent opacity-0 transition-opacity duration-500 group-hover:opacity-100 dark:from-emerald-500/10" />
          <div className="relative flex items-center justify-between mb-4">
            <div className="flex items-center gap-2">
              <Clock className="h-4 w-4 text-muted-foreground" />
              <span className="text-xs font-medium tracking-wide uppercase text-muted-foreground">EFFICIENCY</span>
            </div>
          </div>
          <div className="relative space-y-2">
            <div className="flex items-baseline justify-between">
              <div className="text-lg font-light tracking-tight tabular-nums text-foreground">
                {summary.avg_latency_ms > 0 ? `${summary.avg_latency_ms}ms` : summary.total_requests > 0 ? "<1ms" : "—"}
              </div>
              <div className="text-[11px] text-muted-foreground">latency</div>
            </div>
            <div className="flex items-baseline justify-between">
              <div className="text-lg font-light tracking-tight tabular-nums text-foreground">
                {summary.success_rate != null ? (summary.success_rate * 100).toFixed(1) : summary.total_requests > 0 ? "—" : "100"}%
              </div>
              <div className="text-[11px] text-muted-foreground">success</div>
            </div>
          </div>
        </div>
      </div>

      {/* Main Layout Grid */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(320px,1fr)] items-stretch">
        {/* Custom SVG Topology */}
        <div className="flex flex-col rounded-xl border border-border bg-background shadow-sm overflow-hidden h-full">
          <div className="flex items-center justify-between border-b border-border px-5 py-3">
            <div className="flex items-center gap-2">
              <Box className="h-4 w-4 text-muted-foreground" />
              <h3 className="text-sm font-semibold tracking-tight">Routing Topology</h3>
            </div>
            <span className="text-[11px] font-medium text-muted-foreground uppercase tracking-wider">
              {providers.filter((p: ProviderUsage) => p.share_pct > 0).length} providers · {chains.length} chains
            </span>
          </div>
          <div className="flex-1 flex flex-col justify-center relative">
            <RoutingTopology providers={providers} chains={chains} providerCatalog={providerCatalog} />
          </div>
        </div>

        {/* Insights */}
        <UsageInsightsCard data={data} />
        
        {/* Activity chart */}
        <div className="flex flex-col rounded-xl border border-border bg-background shadow-sm overflow-hidden h-[380px]">
          <div className="flex items-center justify-between border-b border-border px-5 py-3">
            <h3 className="text-sm font-semibold tracking-tight">Usage Trends</h3>
            <span className="text-[11px] font-medium text-muted-foreground uppercase tracking-wider">Requests over time</span>
          </div>
          <div className="flex-1 px-2 pb-3 pt-4 min-h-0">
            <ActivityChart series={series} />
          </div>
        </div>

        {/* Recent Requests */}
        <div className="flex flex-col rounded-xl border border-border bg-background shadow-sm overflow-hidden h-[380px]">
          <div className="flex items-center justify-between border-b border-border px-5 py-3 bg-muted shrink-0">
            <div className="flex items-center gap-2">
              <TerminalSquare className="h-4 w-4 text-muted-foreground" />
              <h3 className="text-sm font-semibold tracking-tight">Recent Requests</h3>
            </div>
          </div>
          {!recent.length ? (
            <div className="flex flex-1 flex-col items-center justify-center gap-3 bg-background">
              <Activity className="h-6 w-6 text-muted-foreground opacity-30" />
              <p className="text-xs font-medium text-muted-foreground">No active requests</p>
            </div>
          ) : (
            <div className="flex-1 overflow-y-auto">
              <table className="w-full border-collapse text-xs">
                <thead className="sticky top-0 z-10 bg-popover backdrop-blur-sm bg-opacity-90">
                  <tr className="border-b border-border">
                    <th className="w-6 px-3 py-2.5" />
                    <th className="px-3 py-2.5 text-left font-medium text-muted-foreground uppercase tracking-wider text-[10px]">Model</th>
                    <th className="px-3 py-2.5 text-right font-medium text-muted-foreground uppercase tracking-wider text-[10px]">Tokens</th>
                    <th className="px-3 py-2.5 text-right font-medium text-muted-foreground uppercase tracking-wider text-[10px]">Time</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border">
                  {recent.map((r: RecentActivity) => <RecentRow key={r.id} row={r} />)}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>

      {/* Token Savings breakdown */}
      {savings && (
        <TokenSavingsBreakdown savings={savings} totalRequests={summary.total_requests} insights={data} period={period} />
      )}

      {/* Breakdowns container to keep grid alignment */}
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
        <ModelUsageTable models={models} />
        <ProviderBreakdown providers={providers} />
      </div>
    </div>
  );
}

// ─── Routing Topology ────────────────────────────────────────────────────────
// Radial hub-and-spoke topology: the Router sits at the center with live
// providers and configured chains orbiting around it. Active sources (real
// provider+model traffic) get animated flow links; dormant chains get a faint
// static hint. Chains expand to reveal their steps in a detail strip below.

interface TopoSource {
  key: string;
  kind: "provider" | "chain";
  label: string;
  sublabel: string;
  color: string;
  icon?: string;
  share: number;
  active: boolean;
  chain?: Chain;
}

function RoutingTopology({ providers, chains, providerCatalog }: { providers: ProviderUsage[]; chains: Chain[]; providerCatalog: Provider[] }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());

  const providerColors = useMemo(() => {
    const m = new Map<string, string>();
    providerCatalog.forEach((p) => m.set(p.id, p.color));
    return m;
  }, [providerCatalog]);

  // Set of provider ids that carried real traffic this period — used to mark a
  // chain as "active" when one of its steps routed live requests.
  const activeProviderIds = useMemo(() => {
    const s = new Set<string>();
    providers.forEach((p) => { if (p.share_pct > 0) s.add(p.provider); });
    return s;
  }, [providers]);

  const sources = useMemo<TopoSource[]>(() => {
    const provs = providers
      .filter((p) => p.share_pct > 0)
      .sort((a, b) => b.share_pct - a.share_pct)
      .map<TopoSource>((p) => ({
        key: `provider:${p.provider}`,
        kind: "provider",
        label: p.display_name || p.provider,
        sublabel: `${p.share_pct.toFixed(1)}% traffic`,
        color: p.color || "var(--color-ink-400)",
        icon: `/providers/${p.provider}.png`,
        share: p.share_pct,
        active: true,
      }));
    const chs = chains.map<TopoSource>((c) => ({
      key: `chain:${c.id}`,
      kind: "chain",
      label: c.name,
      sublabel: `${c.steps.length} step${c.steps.length !== 1 ? "s" : ""} · ${displayStrategy(c.strategy)}`,
      color: "var(--color-accent-500)",
      share: 0,
      // A chain only "connects" when one of its steps used a provider with live
      // traffic, matching the rule that links appear for real provider+model use.
      active: c.steps.some((s) => activeProviderIds.has(s.provider)),
      chain: c,
    }));
    return [...provs, ...chs];
  }, [providers, chains, activeProviderIds]);

  useLayoutEffect(() => {
    const c = containerRef.current;
    if (c) setWidth(c.clientWidth);
  }, []);

  useEffect(() => {
    const c = containerRef.current;
    if (!c) return;
    const ro = new ResizeObserver(() => setWidth(c.clientWidth));
    ro.observe(c);
    return () => ro.disconnect();
  }, []);

  const toggle = (key: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key); else next.add(key);
      return next;
    });
  };

  const n = sources.length;
  // Height scales gently with node count so orbiting nodes have room to breathe.
  const height = Math.max(280, Math.min(480, 200 + Math.max(0, n - 2) * 46));

  // Radial hub-and-spoke placement: the Router sits dead center and sources
  // orbit it on an ellipse. Starting the sweep at 180° puts the first node on
  // the left, so 1–2 nodes read as a clean left/right split while larger counts
  // wrap fully around the hub.
  const placed = useMemo(() => {
    const w = width || 600;
    const cx = w / 2;
    const cy = height / 2;
    const rx = Math.max(130, Math.min(w * 0.34, cx - 108));
    const ry = Math.max(84, cy - 62);
    return sources.map((s, i) => {
      const theta = Math.PI + (i * 2 * Math.PI) / Math.max(1, n);
      return { ...s, x: cx + rx * Math.cos(theta), y: cy + ry * Math.sin(theta) };
    });
  }, [sources, width, height, n]);

  const cx = (width || 600) / 2;
  const cy = height / 2;

  if (sources.length === 0) {
    return (
      <div className="m-4 flex h-[200px] flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border bg-muted">
        <TrendingUp className="h-6 w-6 text-muted-foreground opacity-30" />
        <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">No routing activity</p>
      </div>
    );
  }

  const expandedChains = placed.filter((s) => s.kind === "chain" && expanded.has(s.key) && s.chain);

  return (
    <div className="relative p-4">
      <style>{`
        @keyframes topoFlow { to { stroke-dashoffset: -24; } }
        .topo-flow { animation: topoFlow 1s linear infinite; }
      `}</style>

      <div ref={containerRef} className="relative w-full" style={{ height }}>
        {/* Connector layer */}
        {width > 0 && (
          <svg className="pointer-events-none absolute inset-0" width={width} height={height} viewBox={`0 0 ${width} ${height}`} fill="none">
            {/* Hub rings for a subtle radial anchor */}
            <circle cx={cx} cy={cy} r={44} stroke="var(--border)" strokeWidth={1} strokeOpacity={0.5} />
            <circle cx={cx} cy={cy} r={70} stroke="var(--border)" strokeWidth={1} strokeDasharray="2 6" strokeOpacity={0.35} />
            {placed.map((s) => {
              const d = `M ${s.x} ${s.y} L ${cx} ${cy}`;
              const high = s.active && (s.kind === "chain" || s.share > 20);
              return s.active ? (
                <g key={s.key}>
                  <path d={d} stroke={s.color} strokeWidth={high ? 2.5 : 1.5} strokeOpacity={high ? 0.25 : 0.15} strokeLinecap="round" />
                  <path d={d} stroke={s.color} strokeWidth={high ? 2.5 : 1.5} strokeDasharray="4 8" strokeLinecap="round" strokeOpacity={high ? 0.9 : 0.55} className="topo-flow" />
                </g>
              ) : (
                // Dormant chain / no live traffic — subtle static hint, no flow.
                <path key={s.key} d={d} stroke="var(--color-on-surface-variant)" strokeWidth={1} strokeDasharray="1 6" strokeLinecap="round" strokeOpacity={0.25} />
              );
            })}
          </svg>
        )}

        {/* Router hub (center) */}
        <div
          className="absolute z-20 flex -translate-x-1/2 -translate-y-1/2 flex-col items-center gap-1 rounded-2xl border border-border bg-popover px-4 py-3 shadow-lg"
          style={{ left: cx, top: cy }}
        >
          <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-muted ring-1 ring-border">
            <img src="/flamegate-favicon.svg" alt="Router" className="h-5 w-5" />
          </div>
          <div className="text-center">
            <div className="text-[13px] font-bold leading-tight tracking-tight text-foreground">Router</div>
            <div className="text-[9px] font-medium uppercase tracking-wider text-muted-foreground">{n} route{n !== 1 ? "s" : ""}</div>
          </div>
        </div>

        {/* Source nodes (orbit) */}
        {width > 0 && placed.map((s) => (
          <div key={s.key} className="absolute z-10 -translate-x-1/2 -translate-y-1/2" style={{ left: s.x, top: s.y }}>
            <RadialNode source={s} expanded={expanded.has(s.key)} onToggle={() => toggle(s.key)} />
          </div>
        ))}
      </div>

      {/* Expanded chain details (below the orbit so the layout stays stable) */}
      {expandedChains.length > 0 && (
        <div className="mt-3 space-y-2 border-t border-border pt-3">
          {expandedChains.map((s) => (
            <ChainDetail key={s.key} chain={s.chain!} providerColors={providerColors} />
          ))}
        </div>
      )}
    </div>
  );
}

function RadialNode({ source, expanded, onToggle }: { source: TopoSource; expanded: boolean; onToggle: () => void }) {
  const isChain = source.kind === "chain";
  const hasFallback = !!(source.chain?.fallback_provider && source.chain?.fallback_model);
  return (
    <div
      role={isChain ? "button" : undefined}
      onClick={isChain ? onToggle : undefined}
      className={`flex w-[190px] items-center gap-2 rounded-xl border bg-popover px-2.5 py-2 shadow-sm transition-all hover:border-muted-foreground hover:shadow-md ${
        source.active ? "border-border" : "border-dashed border-border opacity-80"
      } ${isChain ? "cursor-pointer select-none" : ""} ${expanded ? "ring-2 ring-accent-500/40" : ""}`}
    >
      {isChain ? (
        <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md bg-accent-500/15 text-accent-600 dark:text-accent-400">
          <Layers className="h-3 w-3" />
        </span>
      ) : (
        <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md bg-muted ring-1 ring-border" style={{ boxShadow: `inset 3px 0 0 ${source.color}` }}>
          {source.icon && <img src={source.icon} alt="" className="h-3.5 w-3.5 object-contain" onError={(e) => { (e.target as HTMLImageElement).style.display = "none"; }} />}
        </span>
      )}
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1">
          {isChain && <span className="font-mono text-[9px] text-muted-foreground">chain:</span>}
          <span className="truncate text-xs font-semibold text-foreground">{source.label}</span>
        </div>
        <span className="block truncate text-[9px] font-medium uppercase tracking-wider text-muted-foreground">{source.sublabel}</span>
      </div>
      {isChain ? (
        <>
          {hasFallback && (
            <span className="flex h-4 items-center rounded bg-amber-500/10 px-1 text-amber-600 dark:text-amber-400">
              <Shield className="h-2.5 w-2.5" />
            </span>
          )}
          <ChevronRight className={`h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform ${expanded ? "rotate-90" : ""}`} />
        </>
      ) : (
        <div className="h-1.5 w-10 shrink-0 overflow-hidden rounded-full bg-muted">
          <div className="h-full rounded-full" style={{ width: `${Math.max(4, source.share)}%`, backgroundColor: source.color }} />
        </div>
      )}
    </div>
  );
}

function ChainDetail({ chain, providerColors }: { chain: Chain; providerColors: Map<string, string> }) {
  const hasFallback = !!(chain.fallback_provider && chain.fallback_model);
  return (
    <div className="rounded-lg border border-border bg-muted/50 px-3 py-2.5">
      <div className="mb-2 flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
        <Layers className="h-3 w-3 text-accent-500" />
        <span className="font-mono normal-case text-foreground">chain:{chain.name}</span>
      </div>
      <div className="flex flex-wrap items-center gap-x-1.5 gap-y-2">
        {chain.steps.map((step, i) => {
          const color = providerColors.get(step.provider) || "var(--border)";
          return (
            <div key={i} className="flex items-center gap-1.5">
              {i > 0 && <ArrowRight className="h-3 w-3 text-muted-foreground opacity-50" strokeWidth={2} />}
              <div className="flex items-center gap-1.5 rounded-md border border-border bg-background py-1 pl-1 pr-2 shadow-sm">
                <span className="flex h-4 w-4 shrink-0 items-center justify-center rounded" style={{ backgroundColor: `${color}22` }}>
                  <img src={`/providers/${step.provider}.png`} alt="" className="h-3 w-3 object-contain" onError={(e) => { (e.target as HTMLImageElement).style.display = "none"; }} />
                </span>
                <div className="flex min-w-0 flex-col">
                  <span className="truncate font-mono text-[10px] font-medium leading-tight text-foreground">{step.model}</span>
                  <span className="truncate text-[8px] uppercase tracking-wider text-muted-foreground">{step.provider}</span>
                </div>
              </div>
            </div>
          );
        })}
        {hasFallback && (
          <div className="flex items-center gap-1.5">
            <ArrowRight className="h-3 w-3 text-amber-500/60" strokeWidth={2} />
            <div className="flex items-center gap-1.5 rounded-md border border-amber-300/40 bg-amber-500/5 py-1 pl-1 pr-2 shadow-sm dark:border-amber-500/20 dark:bg-amber-500/10">
              <span className="flex h-4 w-4 shrink-0 items-center justify-center rounded bg-amber-500/10 text-amber-500">
                <Shield className="h-2.5 w-2.5" />
              </span>
              <div className="flex min-w-0 flex-col">
                <span className="truncate font-mono text-[10px] font-medium leading-tight text-amber-700 dark:text-amber-400">{chain.fallback_model}</span>
                <span className="truncate text-[8px] uppercase tracking-wider text-amber-600/70 dark:text-amber-400/70">fallback</span>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

const isRoundRobinStrategy = (strategy: string) =>
  strategy === "round_robin" || strategy === "round-robin";

const displayStrategy = (strategy: string) =>
  isRoundRobinStrategy(strategy) ? "round-robin" : strategy;

// ─── Insights Component ───────────────────────────────────────────────────────

function UsageInsightsCard({ data }: { data: any }) {
  const { providers, summary } = data;
  const activeProviders = providers.filter((p: any) => p.share_pct > 0);
  
  const tokenRatio = summary.prompt_tokens > 0 
    ? summary.completion_tokens / summary.prompt_tokens
    : 0;

  return (
    <div className="flex flex-col rounded-xl border border-border bg-background shadow-sm overflow-hidden">
      <div className="flex items-center justify-between border-b border-border px-5 py-3 bg-muted">
        <h3 className="text-sm font-semibold tracking-tight">Insights</h3>
      </div>
      <div className="p-5 flex flex-col gap-6">
        
        {/* Token Efficiency - Elevated design */}
        <div className="flex flex-col">
          <h4 className="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase mb-3">Efficiency Multiplier</h4>
          <div className="flex items-end gap-3">
            <span className="text-4xl font-light tabular-nums leading-none text-foreground tracking-tighter">
              {tokenRatio.toFixed(2)}<span className="text-2xl text-muted-foreground">x</span>
            </span>
            <div className="pb-1">
              {summary.prompt_tokens > 0 && (
                <p className="text-[11px] text-muted-foreground font-medium">
                  {fmtNum(summary.completion_tokens)} out / {fmtNum(summary.prompt_tokens)} in
                </p>
              )}
            </div>
          </div>
        </div>

        <div className="h-px w-full bg-border" />

        {/* Provider Distribution - Sleek Stacked Bar */}
        <div>
          <h4 className="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase mb-4">Traffic Distribution</h4>
          
          <div className="flex h-2.5 w-full overflow-hidden rounded-full bg-muted mb-4">
            {activeProviders.map((p: any) => (
              <div 
                key={p.provider} 
                className="h-full transition-all"
                style={{ width: `${p.share_pct}%`, backgroundColor: p.color || "var(--color-chart-1)" }} 
                title={`${p.provider_name}: ${p.share_pct.toFixed(1)}%`}
              />
            ))}
          </div>

          <div className="grid grid-cols-2 gap-y-3 gap-x-2">
            {activeProviders.slice(0, 4).map((p: any) => (
              <div key={p.provider} className="flex items-center justify-between text-xs pr-2 border-r border-border last:border-r-0 even:border-r-0">
                <div className="flex items-center gap-2 truncate">
                  <span className="h-1.5 w-1.5 rounded-full shrink-0" style={{ backgroundColor: p.color || "var(--color-chart-1)" }} />
                  <span className="text-muted-foreground truncate font-medium">{p.provider_name || p.provider}</span>
                </div>
                <span className="font-semibold tabular-nums ml-2">{p.share_pct.toFixed(0)}%</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

// ─── Activity Chart ──────────────────────────────────────────────────────────

function ActivityChart({ series }: { series: SeriesPoint[] }) {
  if (!series.length) {
    return <div className="flex h-full items-center justify-center text-xs font-medium uppercase tracking-wider text-muted-foreground">No data</div>;
  }

  return (
    <ResponsiveContainer width="100%" height="100%">
      <AreaChart data={series} margin={{ top: 10, right: 0, left: -20, bottom: 0 }}>
        <defs>
          <linearGradient id="usageFill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor="var(--color-chart-1)" stopOpacity={0.15} />
            <stop offset="95%" stopColor="var(--color-chart-1)" stopOpacity={0} />
          </linearGradient>
        </defs>
        {/* Removed harsh grid, kept only subtle horizontal lines */}
        <CartesianGrid vertical={false} stroke="var(--border)" opacity={0.3} />
        <XAxis 
          dataKey="label" 
          tick={{ fontSize: 10, fill: "var(--color-on-surface-variant)", fontWeight: 500 }} 
          tickLine={false} 
          axisLine={false} 
          dy={10}
        />
        <YAxis 
          tick={{ fontSize: 10, fill: "var(--color-on-surface-variant)", fontWeight: 500 }} 
          tickLine={false} 
          axisLine={false} 
          tickFormatter={(v: number) => fmtNum(v)} 
          width={60} 
        />
        <Tooltip
          contentStyle={{ fontSize: 12, background: "var(--color-surface-container)", border: "1px solid var(--border)", borderRadius: 6, boxShadow: "0 4px 6px -1px rgb(0 0 0 / 0.1)" }}
          itemStyle={{ color: "var(--text)", fontWeight: 600 }}
          formatter={(value: number) => [fmtNum(value), "Requests"]}
          labelStyle={{ color: "var(--color-on-surface-variant)", marginBottom: 4 }}
        />
        <Area 
          type="monotone" 
          dataKey="count" 
          stroke="var(--color-chart-1)" 
          strokeWidth={2} 
          fill="url(#usageFill)" 
          activeDot={{ r: 4, strokeWidth: 0, fill: "var(--color-chart-1)" }}
        />
      </AreaChart>
    </ResponsiveContainer>
  );
}

// ─── Recent Row ──────────────────────────────────────────────────────────────

function RecentRow({ row }: { row: RecentActivity }) {
  const success = row.latency_ms > 0;
  const hasSavings = (row.slim_bytes_saved ?? 0) > 0 || row.caveman_active || row.terse_active;
  return (
    <tr className="transition-colors hover:bg-muted group">
      <td className="w-6 px-3 py-2.5">
        <span className={`block h-1.5 w-1.5 rounded-full ${success ? "bg-[color:var(--color-success)]" : "bg-red-500"}`} />
      </td>
      <td className="px-3 py-2.5">
        <div className="flex flex-col gap-1">
          <div className="flex items-center gap-1.5">
            <span className="font-mono text-[11px] font-semibold text-foreground">{row.model || "—"}</span>
          </div>
          <div className="flex items-center gap-1.5">
            {row.provider && (
              <>
                <ProviderIcon provider={row.provider} className="h-3 w-3" />
                <span className="text-[9px] uppercase tracking-wider font-medium text-muted-foreground">{row.provider}</span>
              </>
            )}
            {hasSavings && (
              <span className="flex items-center gap-1">
                {(row.slim_bytes_saved ?? 0) > 0 && (
                  <span className="text-[9px] font-bold text-teal-600 dark:text-teal-400 uppercase tracking-widest" title={`RTK saved ${fmtBytes(row.slim_bytes_saved ?? 0)}`}>
                    [RTK]
                  </span>
                )}
                {row.caveman_active && (
                  <span className="text-[9px] font-bold text-purple-600 dark:text-purple-400 uppercase tracking-widest" title="Caveman compression">
                    [CVMN]
                  </span>
                )}
                {row.terse_active && (
                  <span className="text-[9px] font-bold text-indigo-600 dark:text-indigo-400 uppercase tracking-widest" title="Terse compression">
                    [TRSE]
                  </span>
                )}
              </span>
            )}
          </div>
        </div>
      </td>
      <td className="px-3 py-2.5 text-right tabular-nums text-[11px] font-medium text-muted-foreground">
        <span className="text-foreground">{fmtNum(Math.round(row.tokens * 0.6))}</span> <span className="opacity-40">in</span><br/>
        <span className="text-foreground">{fmtNum(Math.round(row.tokens * 0.4))}</span> <span className="opacity-40">out</span>
      </td>
      <td className="px-3 py-2.5 text-right text-[10px] font-medium text-muted-foreground whitespace-nowrap">
        {relTime(row.created_at)}
      </td>
    </tr>
  );
}

function ProviderIcon({ provider, className = "h-5 w-5" }: { provider: string, className?: string }) {
  const [errored, setErrored] = useState(false);
  if (errored) {
    return (
      <div className={`flex shrink-0 items-center justify-center rounded bg-muted border border-border text-[8px] font-bold text-muted-foreground uppercase ${className}`}>
        {provider.slice(0, 2)}
      </div>
    );
  }
  return <img src={`/providers/${provider}.png`} alt={provider} onError={() => setErrored(true)} className={`shrink-0 rounded object-contain grayscale opacity-80 mix-blend-multiply dark:mix-blend-screen ${className}`} />;
}

// ─── Token Savings Breakdown moved to ../components/SavingsBreakdown.tsx ──

// ─── Model Usage Table ──────────────────────────────────────────────────────

type SortKey = "provider" | "model" | "requests" | "prompt" | "completion" | "cost";

function ModelUsageTable({ models }: { models: ModelUsage[] }) {
  const [search, setSearch] = useState("");
  const [sortKey, setSortKey] = useState<SortKey>("requests");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc");

  const toggleSort = (key: SortKey) => {
    if (sortKey === key) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(key);
      setSortDir("desc");
    }
  };

  const filtered = useMemo(() => {
    const q = search.toLowerCase();
    let rows = models;
    if (q) {
      rows = rows.filter(
        (m) => m.model.toLowerCase().includes(q) || m.provider_name.toLowerCase().includes(q) || m.provider.toLowerCase().includes(q),
      );
    }
    return [...rows].sort((a, b) => {
      const dir = sortDir === "asc" ? 1 : -1;
      switch (sortKey) {
        case "provider": return dir * a.provider_name.localeCompare(b.provider_name);
        case "model": return dir * a.model.localeCompare(b.model);
        case "requests": return dir * (a.total_requests - b.total_requests);
        case "prompt": return dir * (a.prompt_tokens - b.prompt_tokens);
        case "completion": return dir * (a.completion_tokens - b.completion_tokens);
        case "cost": return dir * (a.cost_usd - b.cost_usd);
        default: return 0;
      }
    });
  }, [models, search, sortKey, sortDir]);

  const SortIcon = ({ col }: { col: SortKey }) => {
    if (sortKey !== col) return <ArrowUpDown className="ml-1 inline h-3 w-3 opacity-30" />;
    return sortDir === "asc" ? <ArrowUp className="ml-1 inline h-3 w-3" /> : <ArrowDown className="ml-1 inline h-3 w-3" />;
  };

  const th = (col: SortKey, label: string, align: "left" | "right" = "left") => (
    <th
      className={`cursor-pointer select-none px-4 py-3 font-semibold uppercase tracking-wider text-[10px] text-muted-foreground transition-colors hover:text-foreground ${align === "right" ? "text-right" : "text-left"}`}
      onClick={() => toggleSort(col)}
    >
      {label}
      <SortIcon col={col} />
    </th>
  );

  return (
    <div className="flex flex-col rounded-xl border border-border bg-background shadow-sm overflow-hidden">
      <div className="flex flex-col gap-3 border-b border-border px-5 py-3 bg-muted sm:flex-row sm:items-center sm:justify-between">
        <h3 className="text-sm font-semibold tracking-tight">Model Usage</h3>
        <div className="relative">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search models…"
            className="rounded-lg border border-border bg-background py-1.5 pl-8 pr-3 text-xs placeholder:text-muted-foreground focus:border-foreground focus:outline-none transition-colors w-48"
          />
        </div>
      </div>
      {filtered.length === 0 ? (
        <div className="py-12 text-center text-xs font-medium text-muted-foreground">
          {models.length === 0 ? "No model data for this period" : "No models match your search"}
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead className="bg-background">
              <tr className="border-b border-border">
                {th("model", "Model")}
                {th("requests", "Req", "right")}
                {th("prompt", "In", "right")}
                {th("completion", "Out", "right")}
                {th("cost", "Cost", "right")}
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {filtered.map((m) => (
                <tr key={`${m.provider}/${m.model}`} className="transition-colors hover:bg-muted">
                  <td className="px-4 py-3">
                    <div className="flex flex-col gap-1">
                      <span className="font-mono text-[11px] font-semibold">{m.model}</span>
                      <div className="flex items-center gap-1.5">
                        <ProviderIcon provider={m.provider} className="h-3 w-3" />
                        <span className="text-[9px] uppercase tracking-wider text-muted-foreground">{m.provider_name}</span>
                      </div>
                    </div>
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums font-medium">{m.total_requests.toLocaleString()}</td>
                  <td className="px-4 py-3 text-right tabular-nums text-muted-foreground">{fmtNum(m.prompt_tokens)}</td>
                  <td className="px-4 py-3 text-right tabular-nums text-muted-foreground">{fmtNum(m.completion_tokens)}</td>
                  <td className="px-4 py-3 text-right tabular-nums font-medium text-foreground">${m.cost_usd.toFixed(4)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// ─── Provider Breakdown ─────────────────────────────────────────────────────

function ProviderBreakdown({ providers }: { providers: ProviderUsage[] }) {
  const active = providers.filter((p) => p.total_requests > 0).sort((a,b) => b.total_requests - a.total_requests);
  
  return (
    <div className="flex flex-col rounded-xl border border-border bg-background shadow-sm overflow-hidden">
      <div className="border-b border-border px-5 py-4 bg-muted">
        <h3 className="text-sm font-semibold tracking-tight">Provider Breakdown</h3>
      </div>
      {active.length === 0 ? (
        <div className="py-12 text-center text-xs font-medium text-muted-foreground">No provider data</div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead className="bg-background">
              <tr className="border-b border-border">
                <th className="px-4 py-3 text-left font-semibold uppercase tracking-wider text-[10px] text-muted-foreground">Provider</th>
                <th className="px-4 py-3 text-right font-semibold uppercase tracking-wider text-[10px] text-muted-foreground">Req</th>
                <th className="px-4 py-3 text-right font-semibold uppercase tracking-wider text-[10px] text-muted-foreground">Share</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {active.map((p) => (
                <tr key={p.provider} className="transition-colors hover:bg-muted">
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-3">
                      <ProviderIcon provider={p.provider} />
                      <span className="font-medium text-foreground">{p.display_name}</span>
                    </div>
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums font-medium">{p.total_requests.toLocaleString()}</td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-3">
                      <span className="w-8 text-right tabular-nums font-medium text-foreground">{p.share_pct.toFixed(0)}%</span>
                      <div className="h-1.5 w-16 overflow-hidden rounded-full bg-muted">
                        <div className="h-full rounded-full transition-all" style={{ width: `${Math.max(2, p.share_pct)}%`, backgroundColor: p.color || "var(--text)" }} />
                      </div>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

function fmtNum(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return n.toLocaleString();
}

function fmtBytes(n: number): string {
  if (n >= 1_048_576) return `${(n / 1_048_576).toFixed(1)} MB`;
  if (n >= 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${n} B`;
}

function relTime(iso: string): string {
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return "—";
  const diff = Date.now() - t;
  const s = Math.floor(diff / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h`;
  return `${Math.floor(h / 24)}d`;
}
