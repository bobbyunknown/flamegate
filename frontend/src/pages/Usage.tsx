import { useState, useMemo, useEffect, useRef, useLayoutEffect } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity, DollarSign, Zap, RefreshCw, TrendingUp, Clock, TerminalSquare, ArrowUpDown, ArrowUp, ArrowDown, Search,
  Layers, ChevronRight, Shield, ArrowRight, Flame
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
const LIVE_ACTIVE_WINDOW_MS = 4500;

export function UsagePage() {
  const [period, setPeriod] = useState("today");
  const [liveEventMap, setLiveEventMap] = useState<Record<string, number>>({});
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

  // Listen to SSE live usage events for real-time traffic reactivity
  useEffect(() => {
    const scheduleRefresh = () => {
      if (refreshTimer.current != null) return;
      refreshTimer.current = window.setTimeout(() => {
        refreshTimer.current = null;
        qc.invalidateQueries({ queryKey: ["usage-insights", period] });
        qc.invalidateQueries({ queryKey: ["usage-models", period] });
      }, USAGE_REFRESH_DEBOUNCE_MS);
    };

    return connectUsageStream((ev) => {
      if (ev?.provider) {
        setLiveEventMap((prev) => ({
          ...prev,
          [ev.provider]: Date.now(),
        }));
      }
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
        description="Monitor request flow, real-time routing topology, and provider metrics."
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
        : insights.data ? (
          <UsageContent
            data={insights.data}
            models={modelUsage.data?.models ?? []}
            chains={chains.data?.chains ?? []}
            providerCatalog={providerCatalog.data?.providers ?? []}
            liveEventMap={liveEventMap}
            period={period}
          />
        ) : null}
    </>
  );
}

function UsageContent({
  data,
  models,
  chains,
  providerCatalog,
  liveEventMap,
  period,
}: {
  data: any;
  models: ModelUsage[];
  chains: Chain[];
  providerCatalog: Provider[];
  liveEventMap: Record<string, number>;
  period: string;
}) {
  const { summary, savings, providers, recent, series } = data;

  return (
    <div className="space-y-8 pb-12">
      {/* Precision Stat Grid */}
      <div className="grid grid-cols-2 lg:grid-cols-5 gap-4">
        <StatCard label="REQUESTS" value={fmtNum(summary.total_requests)} icon={Activity} />
        <StatCard label="INPUT TOKENS" value={fmtNum(summary.prompt_tokens)} icon={Zap} />
        <StatCard label="OUTPUT TOKENS" value={fmtNum(summary.completion_tokens)} icon={TrendingUp} />
        <StatCard label="EST. COST" value={`$${summary.cost_usd.toFixed(2)}`} icon={DollarSign} />
        
        <div className="flex flex-col justify-between p-5 rounded-xl border border-border bg-card shadow-xs transition-colors hover:bg-muted/40 relative overflow-hidden group col-span-2 lg:col-span-1">
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
        {/* FlameGate Redesigned Routing Topology */}
        <div className="flex flex-col rounded-xl border border-border bg-card shadow-xs overflow-hidden h-full">
          <RoutingTopology
            providers={providers}
            chains={chains}
            providerCatalog={providerCatalog}
            liveEventMap={liveEventMap}
            summary={summary}
          />
        </div>

        {/* Insights */}
        <UsageInsightsCard data={data} />
        
        {/* Activity chart */}
        <div className="flex flex-col rounded-xl border border-border bg-card shadow-xs overflow-hidden h-[380px]">
          <div className="flex items-center justify-between border-b border-border px-5 py-3 bg-muted/40">
            <div className="flex items-center gap-2">
              <TrendingUp className="h-4 w-4 text-muted-foreground" />
              <h3 className="text-sm font-semibold tracking-tight">Usage Trends</h3>
            </div>
            <span className="text-[11px] font-medium text-muted-foreground uppercase tracking-wider">Requests over time</span>
          </div>
          <div className="flex-1 px-2 pb-3 pt-4 min-h-0">
            <ActivityChart series={series} />
          </div>
        </div>

        {/* Recent Requests */}
        <div className="flex flex-col rounded-xl border border-border bg-card shadow-xs overflow-hidden h-[380px]">
          <div className="flex items-center justify-between border-b border-border px-5 py-3 bg-muted/40 shrink-0">
            <div className="flex items-center gap-2">
              <TerminalSquare className="h-4 w-4 text-muted-foreground" />
              <h3 className="text-sm font-semibold tracking-tight">Recent Requests</h3>
            </div>
            <span className="text-[11px] font-medium text-muted-foreground uppercase tracking-wider">
              {recent.length} recent
            </span>
          </div>
          {!recent.length ? (
            <div className="flex flex-1 flex-col items-center justify-center gap-3 bg-card">
              <Activity className="h-6 w-6 text-muted-foreground opacity-30" />
              <p className="text-xs font-medium text-muted-foreground">No active requests</p>
            </div>
          ) : (
            <div className="flex-1 overflow-y-auto">
              <table className="w-full border-collapse text-xs">
                <thead className="sticky top-0 z-10 bg-muted/90 backdrop-blur-xs">
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

      {/* Breakdowns container */}
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
        <ModelUsageTable models={models} />
        <ProviderBreakdown providers={providers} />
      </div>
    </div>
  );
}

// ─── FlameGate Redesigned Routing Topology ────────────────────────────────────

interface TopoSource {
  key: string;
  kind: "provider" | "chain";
  id: string;
  label: string;
  sublabel: string;
  color: string;
  icon?: string;
  share: number;
  requests: number;
  tokens: number;
  live: boolean;
  chain?: Chain;
}

function RoutingTopology({
  providers,
  chains,
  providerCatalog,
  liveEventMap,
  summary,
}: {
  providers: ProviderUsage[];
  chains: Chain[];
  providerCatalog: Provider[];
  liveEventMap: Record<string, number>;
  summary: any;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [now, setNow] = useState(Date.now());

  // Tick clock every second for live animation expiration
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);

  const providerColors = useMemo(() => {
    const m = new Map<string, string>();
    providerCatalog.forEach((p) => m.set(p.id, p.color));
    return m;
  }, [providerCatalog]);

  const sources = useMemo<TopoSource[]>(() => {
    const provs = providers
      .filter((p) => p.total_requests > 0 || p.share_pct > 0)
      .sort((a, b) => b.total_requests - a.total_requests)
      .map<TopoSource>((p) => {
        const lastActive = liveEventMap[p.provider] ?? 0;
        const isLive = now - lastActive < LIVE_ACTIVE_WINDOW_MS;
        return {
          key: `provider:${p.provider}`,
          kind: "provider",
          id: p.provider,
          label: p.display_name || p.provider,
          sublabel: `${p.share_pct.toFixed(1)}% share · ${fmtNum(p.total_requests)} reqs`,
          color: p.color || "var(--color-accent-500)",
          icon: `/providers/${p.provider}.png`,
          share: p.share_pct,
          requests: p.total_requests,
          tokens: p.prompt_tokens + p.completion_tokens,
          live: isLive,
        };
      });

    const chs = chains.map<TopoSource>((c) => {
      const isAnyStepLive = c.steps.some(
        (s) => now - (liveEventMap[s.provider] ?? 0) < LIVE_ACTIVE_WINDOW_MS
      );
      return {
        key: `chain:${c.id}`,
        kind: "chain",
        id: c.id,
        label: c.name,
        sublabel: `${c.steps.length} step${c.steps.length !== 1 ? "s" : ""} · ${displayStrategy(c.strategy)}`,
        color: "var(--color-accent-500)",
        share: 0,
        requests: 0,
        tokens: 0,
        live: isAnyStepLive,
        chain: c,
      };
    });

    return [...provs, ...chs];
  }, [providers, chains, liveEventMap, now]);

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
  const height = Math.max(300, Math.min(480, 220 + Math.max(0, n - 2) * 44));

  const placed = useMemo(() => {
    const w = width || 600;
    const cx = w / 2;
    const cy = height / 2;
    const rx = Math.max(140, Math.min(w * 0.35, cx - 110));
    const ry = Math.max(88, cy - 64);
    return sources.map((s, i) => {
      const theta = Math.PI + (i * 2 * Math.PI) / Math.max(1, n);
      return { ...s, x: cx + rx * Math.cos(theta), y: cy + ry * Math.sin(theta) };
    });
  }, [sources, width, height, n]);

  const cx = (width || 600) / 2;
  const cy = height / 2;
  const anyLiveActive = sources.some((s) => s.live);

  const expandedChains = placed.filter(
    (s) => s.kind === "chain" && expanded.has(s.key) && s.chain
  );

  return (
    <>
      {/* Topology Header */}
      <div className="flex items-center justify-between border-b border-border px-5 py-3 bg-muted/40">
        <div className="flex items-center gap-2">
          <Layers className="h-4 w-4 text-primary" />
          <h3 className="text-sm font-semibold tracking-tight">Routing Topology</h3>
        </div>
        <div className="flex items-center gap-2">
          {anyLiveActive ? (
            <Badge variant="warning" className="gap-1 animate-pulse">
              <Flame className="h-3 w-3 text-orange-500 fill-orange-500/30" />
              Live Routing Flow
            </Badge>
          ) : (
            <Badge variant="outline" className="gap-1.5 text-muted-foreground font-normal">
              <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 shadow-xs" />
              Connected · Standby
            </Badge>
          )}
          <span className="text-[11px] font-medium text-muted-foreground uppercase tracking-wider hidden sm:inline">
            {providers.filter((p) => p.total_requests > 0).length} active · {chains.length} chains
          </span>
        </div>
      </div>

      {sources.length === 0 ? (
        <div className="m-6 flex h-[220px] flex-col items-center justify-center gap-3 rounded-xl border border-dashed border-border bg-muted/30">
          <Layers className="h-8 w-8 text-muted-foreground opacity-30" />
          <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">No active provider accounts</p>
        </div>
      ) : (
        <div className="relative p-4 flex-1 flex flex-col justify-center">
          <style>{`
            @keyframes flameFlow {
              from { stroke-dashoffset: 24; }
              to { stroke-dashoffset: 0; }
            }
            .flame-flow-active {
              animation: flameFlow 0.8s linear infinite;
            }
          `}</style>

          <div ref={containerRef} className="relative w-full" style={{ height }}>
            {/* SVG Connector layer */}
            {width > 0 && (
              <svg className="pointer-events-none absolute inset-0" width={width} height={height} viewBox={`0 0 ${width} ${height}`} fill="none">
                <defs>
                  <radialGradient id="hubAura" cx="50%" cy="50%" r="50%">
                    <stop offset="0%" stopColor="#f97316" stopOpacity="0.15" />
                    <stop offset="100%" stopColor="#f97316" stopOpacity="0" />
                  </radialGradient>
                  <linearGradient id="activeBeam" x1="0%" y1="0%" x2="100%" y2="0%">
                    <stop offset="0%" stopColor="#f97316" />
                    <stop offset="100%" stopColor="#ea580c" />
                  </linearGradient>
                </defs>

                {/* Hub background aura */}
                <circle cx={cx} cy={cy} r={80} fill="url(#hubAura)" />
                <circle cx={cx} cy={cy} r={46} stroke="var(--border)" strokeWidth={1} strokeOpacity={0.6} />
                <circle cx={cx} cy={cy} r={72} stroke="var(--border)" strokeWidth={1} strokeDasharray="3 6" strokeOpacity={0.3} />

                {/* Connection lines */}
                {placed.map((s) => {
                  const d = `M ${s.x} ${s.y} L ${cx} ${cy}`;
                  if (s.live) {
                    return (
                      <g key={s.key}>
                        {/* Glow halo */}
                        <path d={d} stroke="#f97316" strokeWidth={4} strokeOpacity={0.3} strokeLinecap="round" />
                        {/* Active animated pulsing beam */}
                        <path
                          d={d}
                          stroke="url(#activeBeam)"
                          strokeWidth={2.5}
                          strokeDasharray="6 8"
                          strokeLinecap="round"
                          className="flame-flow-active"
                        />
                      </g>
                    );
                  }

                  // Standby connection (Idle): Solid or crisp dotted line WITHOUT perpetual animation
                  return (
                    <g key={s.key}>
                      <path
                        d={d}
                        stroke="var(--border)"
                        strokeWidth={1.5}
                        strokeDasharray={s.requests > 0 ? undefined : "3 5"}
                        strokeLinecap="round"
                        strokeOpacity={s.requests > 0 ? 0.7 : 0.35}
                      />
                    </g>
                  );
                })}
              </svg>
            )}

            {/* FlameGate Core Router Hub (Center) */}
            <div
              className={`absolute z-20 flex -translate-x-1/2 -translate-y-1/2 flex-col items-center gap-1.5 rounded-2xl border bg-card px-4 py-3.5 shadow-md transition-all ${
                anyLiveActive
                  ? "border-orange-500/50 ring-4 ring-orange-500/10 shadow-orange-500/10"
                  : "border-border"
              }`}
              style={{ left: cx, top: cy }}
            >
              <div className="relative flex h-10 w-10 items-center justify-center rounded-xl bg-muted/80 ring-1 ring-border shadow-xs">
                <img src="/flamegate-favicon.svg" alt="FlameGate" className="h-6 w-6" />
                {anyLiveActive && (
                  <span className="absolute -top-1 -right-1 flex h-3 w-3">
                    <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-orange-400 opacity-75" />
                    <span className="relative inline-flex rounded-full h-3 w-3 bg-orange-500" />
                  </span>
                )}
              </div>
              <div className="text-center">
                <div className="text-xs font-bold leading-tight tracking-tight text-foreground flex items-center justify-center gap-1">
                  FlameGate
                </div>
                <div className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                  {n} route{n !== 1 ? "s" : ""} · {summary.avg_latency_ms > 0 ? `${summary.avg_latency_ms}ms` : "<1ms"}
                </div>
              </div>
            </div>

            {/* Orbiting Nodes (Providers & Chains) */}
            {width > 0 &&
              placed.map((s) => (
                <div key={s.key} className="absolute z-10 -translate-x-1/2 -translate-y-1/2" style={{ left: s.x, top: s.y }}>
                  <RadialNode source={s} expanded={expanded.has(s.key)} onToggle={() => toggle(s.key)} />
                </div>
              ))}
          </div>

          {/* Expanded Chain Pipeline Detail */}
          {expandedChains.length > 0 && (
            <div className="mt-4 space-y-2 border-t border-border pt-3">
              {expandedChains.map((s) => (
                <ChainDetail key={s.key} chain={s.chain!} providerColors={providerColors} />
              ))}
            </div>
          )}
        </div>
      )}
    </>
  );
}

function RadialNode({ source, expanded, onToggle }: { source: TopoSource; expanded: boolean; onToggle: () => void }) {
  const isChain = source.kind === "chain";
  const hasFallback = !!(source.chain?.fallback_provider && source.chain?.fallback_model);

  return (
    <div
      role={isChain ? "button" : undefined}
      onClick={isChain ? onToggle : undefined}
      className={`group flex w-[200px] items-center gap-2.5 rounded-xl border bg-card px-3 py-2.5 shadow-xs transition-all hover:border-foreground/40 hover:shadow-md ${
        source.live
          ? "border-orange-500/60 ring-2 ring-orange-500/20 bg-orange-500/5 dark:bg-orange-950/20"
          : source.requests > 0
          ? "border-border hover:border-border/80"
          : "border-dashed border-border/70 opacity-75"
      } ${isChain ? "cursor-pointer select-none" : ""}`}
    >
      {isChain ? (
        <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
          <Layers className="h-3.5 w-3.5" />
        </span>
      ) : (
        <span
          className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-muted ring-1 ring-border"
          style={{ boxShadow: `inset 3px 0 0 ${source.color}` }}
        >
          {source.icon && (
            <img
              src={source.icon}
              alt=""
              className="h-4 w-4 object-contain"
              onError={(e) => {
                (e.target as HTMLImageElement).style.display = "none";
              }}
            />
          )}
        </span>
      )}

      <div className="min-w-0 flex-1">
        <div className="flex items-center justify-between gap-1">
          <span className="truncate text-xs font-semibold text-foreground">{source.label}</span>
          {source.live ? (
            <span className="flex h-1.5 w-1.5 shrink-0 rounded-full bg-orange-500 animate-ping" />
          ) : (
            <span className="text-[9px] font-mono text-muted-foreground">{source.share > 0 ? `${source.share.toFixed(0)}%` : ""}</span>
          )}
        </div>
        <span className="block truncate text-[10px] text-muted-foreground font-normal">{source.sublabel}</span>
      </div>

      {isChain ? (
        <div className="flex items-center gap-1">
          {hasFallback && (
            <span className="flex h-4 items-center rounded bg-amber-500/10 px-1 text-amber-600 dark:text-amber-400" title="Fallback configured">
              <Shield className="h-2.5 w-2.5" />
            </span>
          )}
          <ChevronRight className={`h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform ${expanded ? "rotate-90" : ""}`} />
        </div>
      ) : (
        <div className="h-1.5 w-8 shrink-0 overflow-hidden rounded-full bg-muted">
          <div
            className="h-full rounded-full transition-all"
            style={{
              width: `${Math.max(4, source.share)}%`,
              backgroundColor: source.live ? "#f97316" : source.color,
            }}
          />
        </div>
      )}
    </div>
  );
}

function ChainDetail({ chain, providerColors }: { chain: Chain; providerColors: Map<string, string> }) {
  const hasFallback = !!(chain.fallback_provider && chain.fallback_model);
  return (
    <div className="rounded-xl border border-border bg-muted/30 px-3.5 py-3">
      <div className="mb-2.5 flex items-center justify-between text-xs">
        <div className="flex items-center gap-1.5 font-medium text-foreground">
          <Layers className="h-3.5 w-3.5 text-primary" />
          <span className="font-mono text-[11px] font-semibold">{chain.name}</span>
        </div>
        <Badge variant="outline" className="text-[10px] uppercase tracking-wider">
          {displayStrategy(chain.strategy)}
        </Badge>
      </div>

      <div className="flex flex-wrap items-center gap-x-2 gap-y-2">
        {chain.steps.map((step, i) => {
          const color = providerColors.get(step.provider) || "var(--border)";
          return (
            <div key={i} className="flex items-center gap-2">
              {i > 0 && <ArrowRight className="h-3 w-3 text-muted-foreground opacity-50" strokeWidth={2} />}
              <div className="flex items-center gap-2 rounded-lg border border-border bg-card py-1 pl-1.5 pr-2.5 shadow-xs">
                <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-md" style={{ backgroundColor: `${color}22` }}>
                  <img
                    src={`/providers/${step.provider}.png`}
                    alt=""
                    className="h-3.5 w-3.5 object-contain"
                    onError={(e) => {
                      (e.target as HTMLImageElement).style.display = "none";
                    }}
                  />
                </span>
                <div className="flex min-w-0 flex-col">
                  <span className="truncate font-mono text-[10px] font-semibold leading-tight text-foreground">{step.model}</span>
                  <span className="truncate text-[8px] uppercase tracking-wider text-muted-foreground">{step.provider}</span>
                </div>
              </div>
            </div>
          );
        })}
        {hasFallback && (
          <div className="flex items-center gap-2">
            <ArrowRight className="h-3 w-3 text-amber-500/60" strokeWidth={2} />
            <div className="flex items-center gap-2 rounded-lg border border-amber-500/20 bg-amber-500/10 py-1 pl-1.5 pr-2.5 shadow-xs">
              <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-md bg-amber-500/20 text-amber-500">
                <Shield className="h-3 w-3" />
              </span>
              <div className="flex min-w-0 flex-col">
                <span className="truncate font-mono text-[10px] font-semibold leading-tight text-amber-600 dark:text-amber-400">
                  {chain.fallback_model}
                </span>
                <span className="truncate text-[8px] uppercase tracking-wider text-amber-600/80 dark:text-amber-400/80">
                  {chain.fallback_provider} (fallback)
                </span>
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
    <div className="flex flex-col rounded-xl border border-border bg-card shadow-xs overflow-hidden">
      <div className="flex items-center justify-between border-b border-border px-5 py-3 bg-muted/40">
        <h3 className="text-sm font-semibold tracking-tight">Insights</h3>
      </div>
      <div className="p-5 flex flex-col gap-6">
        
        {/* Token Efficiency */}
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

        {/* Provider Distribution */}
        <div>
          <h4 className="text-[10px] font-semibold tracking-widest text-muted-foreground uppercase mb-4">Traffic Distribution</h4>
          
          <div className="flex h-2.5 w-full overflow-hidden rounded-full bg-muted mb-4">
            {activeProviders.map((p: any) => (
              <div 
                key={p.provider} 
                className="h-full transition-all"
                style={{ width: `${p.share_pct}%`, backgroundColor: p.color || "var(--color-chart-1)" }} 
                title={`${p.provider_name || p.provider}: ${p.share_pct.toFixed(1)}%`}
              />
            ))}
          </div>

          <div className="grid grid-cols-2 gap-y-3 gap-x-2">
            {activeProviders.slice(0, 4).map((p: any) => (
              <div key={p.provider} className="flex items-center justify-between text-xs pr-2 border-r border-border last:border-r-0 even:border-r-0">
                <div className="flex items-center gap-2 truncate">
                  <span className="h-1.5 w-1.5 rounded-full shrink-0" style={{ backgroundColor: p.color || "var(--color-chart-1)" }} />
                  <span className="text-muted-foreground truncate font-medium">{p.display_name || p.provider}</span>
                </div>
                <span className="font-semibold tabular-nums ml-2 text-foreground">{p.share_pct.toFixed(0)}%</span>
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
        <CartesianGrid vertical={false} stroke="var(--border)" opacity={0.3} />
        <XAxis 
          dataKey="label" 
          tick={{ fontSize: 10, fill: "var(--color-ink-400)", fontWeight: 500 }} 
          tickLine={false} 
          axisLine={false} 
          dy={10}
        />
        <YAxis 
          tick={{ fontSize: 10, fill: "var(--color-ink-400)", fontWeight: 500 }} 
          tickLine={false} 
          axisLine={false} 
          tickFormatter={(v: number) => fmtNum(v)} 
          width={60} 
        />
        <Tooltip
          contentStyle={{
            fontSize: 12,
            background: "var(--color-ink-900, #1c1b18)",
            border: "1px solid var(--border)",
            borderRadius: 8,
            boxShadow: "0 4px 12px rgba(0,0,0,0.2)"
          }}
          itemStyle={{ color: "var(--color-ink-50, #faf9f7)", fontWeight: 600 }}
          formatter={(value: number) => [fmtNum(value), "Requests"]}
          labelStyle={{ color: "var(--color-ink-400, #a8a59a)", marginBottom: 4 }}
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
    <tr className="transition-colors hover:bg-muted/50 group">
      <td className="w-6 px-3 py-2.5">
        <span className={`block h-1.5 w-1.5 rounded-full ${success ? "bg-emerald-500" : "bg-destructive"}`} />
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
  return (
    <img
      src={`/providers/${provider}.png`}
      alt={provider}
      onError={() => setErrored(true)}
      className={`shrink-0 rounded object-contain grayscale opacity-80 mix-blend-multiply dark:mix-blend-screen ${className}`}
    />
  );
}

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
    <div className="flex flex-col rounded-xl border border-border bg-card shadow-xs overflow-hidden">
      <div className="flex flex-col gap-3 border-b border-border px-5 py-3 bg-muted/40 sm:flex-row sm:items-center sm:justify-between">
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
            <thead className="bg-card">
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
                <tr key={`${m.provider}/${m.model}`} className="transition-colors hover:bg-muted/50">
                  <td className="px-4 py-3">
                    <div className="flex flex-col gap-1">
                      <span className="font-mono text-[11px] font-semibold text-foreground">{m.model}</span>
                      <div className="flex items-center gap-1.5">
                        <ProviderIcon provider={m.provider} className="h-3 w-3" />
                        <span className="text-[9px] uppercase tracking-wider text-muted-foreground">{m.provider_name}</span>
                      </div>
                    </div>
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums font-medium text-foreground">{m.total_requests.toLocaleString()}</td>
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
    <div className="flex flex-col rounded-xl border border-border bg-card shadow-xs overflow-hidden">
      <div className="border-b border-border px-5 py-4 bg-muted/40">
        <h3 className="text-sm font-semibold tracking-tight">Provider Breakdown</h3>
      </div>
      {active.length === 0 ? (
        <div className="py-12 text-center text-xs font-medium text-muted-foreground">No provider data</div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead className="bg-card">
              <tr className="border-b border-border">
                <th className="px-4 py-3 text-left font-semibold uppercase tracking-wider text-[10px] text-muted-foreground">Provider</th>
                <th className="px-4 py-3 text-right font-semibold uppercase tracking-wider text-[10px] text-muted-foreground">Req</th>
                <th className="px-4 py-3 text-right font-semibold uppercase tracking-wider text-[10px] text-muted-foreground">Share</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {active.map((p) => (
                <tr key={p.provider} className="transition-colors hover:bg-muted/50">
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-3">
                      <ProviderIcon provider={p.provider} />
                      <span className="font-medium text-foreground">{p.display_name}</span>
                    </div>
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums font-medium text-foreground">{p.total_requests.toLocaleString()}</td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-3">
                      <span className="w-8 text-right tabular-nums font-medium text-foreground">{p.share_pct.toFixed(0)}%</span>
                      <div className="h-1.5 w-16 overflow-hidden rounded-full bg-muted">
                        <div className="h-full rounded-full transition-all" style={{ width: `${Math.max(2, p.share_pct)}%`, backgroundColor: p.color || "var(--color-chart-1)" }} />
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
