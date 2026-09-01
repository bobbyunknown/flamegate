import { useState, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  DollarSign,
  Database,
  Zap,
  Calendar,
  Clock,
  CheckCircle2,
  Timer,
  TrendingUp,
  AlertTriangle,
  Wallet,
  Sparkles,
} from "lucide-react";
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  ResponsiveContainer,
  Tooltip,
  CartesianGrid,
} from "recharts";
import { api, type UsageInsights, type RecentActivity, type ProviderUsage } from "../lib/api";
import { microsToUSD } from "../lib/format";
import { PageHeader } from "@/components/composite/page-header";
import { Card, CardContent } from "@/components/ui/card";
import { SectionHeader } from "@/components/composite/section-header";
import { StatCard } from "@/components/composite/stat-card";
import { ErrorCard } from "@/components/composite/error-card";
import { Skeleton } from "@/components/ui/skeleton";
import { NativeSelect as Select } from "@/components/composite/native-select";

const periods = [
  { value: "today", label: "Today" },
  { value: "week", label: "Last 7 days" },
  { value: "month", label: "Last 30 days" },
];

export function OverviewPage() {
  const [period, setPeriod] = useState("week");
  const insights = useQuery({
    queryKey: ["usage-insights", period],
    queryFn: () => api.usageInsights(period),
    staleTime: 30_000,
    placeholderData: (previous) => previous,
  });

  const budgets = useQuery({
    queryKey: ["budget-status"],
    queryFn: () => api.budgetStatus(),
    staleTime: 30_000,
    refetchInterval: 60_000,
    placeholderData: (previous) => previous,
  });

  const alerts = (budgets.data?.budgets ?? []).filter((b) => b.pct_used >= b.alert_pct);
  const blocked = alerts.filter((b) => b.pct_used >= 100 && b.hard_cutoff);
  const warnings = alerts.filter((b) => b.pct_used < 100 || !b.hard_cutoff);

  return (
    <>
      {/* ── Plan alerts ──────────────────────────────────────────── */}
      {blocked.length > 0 && (
        <div className="mb-6 flex items-center gap-3 rounded-xl border border-destructive/30 bg-destructive/10 px-4 py-3">
          <AlertTriangle className="h-5 w-5 shrink-0 text-destructive" />
          <div className="flex-1">
            <p className="text-sm font-medium text-foreground">Plan limit reached. Requests blocked.</p>
            <p className="text-xs text-muted-foreground">
              {blocked.map((b) => `${b.scope_name} (${microsToUSD(b.limit_micros)} ${b.period})`).join(", ")}
            </p>
          </div>
          <a
            href="/plans"
            className="shrink-0 rounded-lg border border-destructive/30 px-3 py-1.5 text-xs font-medium text-destructive transition-colors hover:bg-destructive/20"
          >
            Manage
          </a>
        </div>
      )}
      {warnings.length > 0 && blocked.length === 0 && (
        <div className="mb-6 flex items-center gap-3 rounded-xl border border-amber-500/30 bg-amber-500/10 px-4 py-3">
          <Wallet className="h-5 w-5 shrink-0 text-amber-500" />
          <div className="flex-1">
            <p className="text-sm font-medium text-foreground">Plan alert</p>
            <p className="text-xs text-muted-foreground">
              {warnings.map((b) => `${b.scope_name}: ${b.pct_used.toFixed(0)}% used`).join(", ")}
            </p>
          </div>
          <a
            href="/plans"
            className="shrink-0 rounded-lg border border-amber-500/30 px-3 py-1.5 text-xs font-medium text-amber-500 transition-colors hover:bg-amber-500/20"
          >
            Manage
          </a>
        </div>
      )}

      <PageHeader
        title="Overview"
        icon={Activity}
        description="Usage, routing metrics, and performance across all AI providers."
        action={
          <div className="flex items-center gap-2 rounded-xl border border-border/80 bg-card/80 px-3 py-1.5 shadow-sm backdrop-blur-sm">
            <Calendar className="h-4 w-4 text-muted-foreground" />
            <Select
              value={period}
              onChange={(e) => setPeriod(e.target.value)}
              className="border-0 bg-transparent px-0 py-0 h-auto text-sm font-medium text-foreground focus:ring-0"
            >
              {periods.map((p) => (
                <option key={p.value} value={p.value} className="bg-popover text-foreground">
                  {p.label}
                </option>
              ))}
            </Select>
          </div>
        }
      />

      {insights.isLoading ? (
        <OverviewSkeleton />
      ) : insights.isError ? (
        <ErrorCard message="Failed to load usage data. Is the backend running?" />
      ) : (
        insights.data ? <InsightsDashboard data={insights.data} /> : null
      )}
    </>
  );
}

function OverviewSkeleton() {
  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-28 rounded-xl" />
        ))}
      </div>
      <Skeleton className="h-72 rounded-xl" />
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <Skeleton className="h-64 rounded-xl" />
        <Skeleton className="h-64 rounded-xl" />
      </div>
    </div>
  );
}

function InsightsDashboard({ data }: { data: UsageInsights }) {
  const { summary, providers, recent, series } = data;

  const totalTokens = summary.prompt_tokens + summary.completion_tokens;
  const costUSD = summary.cost_usd;
  const avgLatency = summary.avg_latency_ms;
  const successRate = summary.success_rate;

  return (
    <div className="space-y-6">
      {/* ── Metric cards ─────────────────────────────────────────── */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard
          label="Total Requests"
          value={summary.total_requests.toLocaleString()}
          icon={Activity}
          iconTone="accent"
        />
        <StatCard
          label="Est. Cost"
          value={`$${costUSD.toFixed(2)}`}
          icon={DollarSign}
          iconTone="warning"
        />
        <StatCard
          label="Total Tokens"
          value={compact(totalTokens)}
          icon={Database}
          iconTone="accent"
        />
        <StatCard
          label="Avg Latency"
          value={avgLatency > 0 ? `${avgLatency}ms` : summary.total_requests > 0 ? "<1ms" : "—"}
          icon={Timer}
          iconTone="neutral"
        />
      </div>

      {/* ── Secondary KPIs ───────────────────────────────────────── */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard
          label="Prompt Tokens"
          value={compact(summary.prompt_tokens)}
          icon={Zap}
          iconTone="accent"
        />
        <StatCard
          label="Completion Tokens"
          value={compact(summary.completion_tokens)}
          icon={TrendingUp}
          iconTone="accent"
        />
        <StatCard
          label="Success Rate"
          value={successRate != null ? `${(successRate * 100).toFixed(1)}%` : summary.total_requests > 0 ? "100%" : "100%"}
          icon={CheckCircle2}
          iconTone="accent"
        />
        <StatCard
          label="Cached Tokens"
          value={compact(summary.cached_tokens)}
          icon={Clock}
          iconTone="neutral"
        />
      </div>

      {/* ── Activity chart ───────────────────────────────────────── */}
      <Card className="border-border/80 bg-card/80 backdrop-blur-sm shadow-sm">
        <SectionHeader
          title="Activity over time"
          description="Request volume in the selected time window."
          icon={Activity}
          iconTone="accent"
        />
        <CardContent className="px-6 pb-6 pt-0">
          <ActivityChart series={series} />
        </CardContent>
      </Card>

      {/* ── Breakdown grid ───────────────────────────────────────── */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <Card className="border-border/80 bg-card/80 backdrop-blur-sm shadow-sm">
          <SectionHeader
            title="Provider distribution"
            description="Share of total requests across providers."
            icon={Database}
            iconTone="accent"
          />
          <CardContent className="px-6 pb-6 pt-0">
            <ProviderBreakdown providers={providers} totalRequests={summary.total_requests} />
          </CardContent>
        </Card>

        <Card className="border-border/80 bg-card/80 backdrop-blur-sm shadow-sm">
          <SectionHeader
            title="Token breakdown"
            description="Input vs. output and cache efficiency."
            icon={Zap}
            iconTone="accent"
          />
          <CardContent className="px-6 pb-6 pt-0">
            <TokenBreakdown summary={summary} />
          </CardContent>
        </Card>

        <Card className="lg:col-span-2 border-border/80 bg-card/80 backdrop-blur-sm shadow-sm">
          <SectionHeader
            title="Recent activity"
            description="Latest requests processed through the gateway."
            icon={Clock}
            iconTone="neutral"
            action={
              recent.length > 0 ? (
                <span className="inline-flex items-center rounded-md border border-border bg-muted/60 px-2 py-1 text-xs font-medium text-muted-foreground">
                  Last {recent.length} requests
                </span>
              ) : undefined
            }
          />
          <RecentActivityTable recent={recent} providers={providers} />
        </Card>
      </div>
    </div>
  );
}

/* ── Custom Chart Tooltip ────────────────────────────────────────── */

interface TooltipProps {
  active?: boolean;
  payload?: Array<{ value: number; payload: { label: string; count: number } }>;
  label?: string;
}

function CustomChartTooltip({ active, payload, label }: TooltipProps) {
  if (!active || !payload || !payload.length) return null;
  const count = payload[0].value ?? 0;

  return (
    <div className="rounded-lg border border-border/80 bg-popover/95 px-3 py-2 text-xs shadow-xl backdrop-blur-md">
      <div className="font-medium text-muted-foreground mb-1">{label}</div>
      <div className="flex items-center gap-2">
        <span className="size-2 rounded-full bg-primary animate-pulse" />
        <span className="text-foreground font-semibold tabular-nums">
          {count.toLocaleString()} {count === 1 ? "request" : "requests"}
        </span>
      </div>
    </div>
  );
}

/* ── Activity sparkline / area chart ─────────────────────────────── */

function ActivityChart({ series }: { series: UsageInsights["series"] }) {
  if (!series || series.length === 0) {
    return (
      <div className="flex h-52 items-center justify-center rounded-lg border border-dashed border-border/60 text-sm text-muted-foreground">
        No activity recorded for this period.
      </div>
    );
  }

  // Format label ticks to avoid cramped timestamps
  const formattedSeries = useMemo(() => {
    return series.map((item) => ({
      ...item,
      displayLabel: item.label,
    }));
  }, [series]);

  return (
    <div className="relative h-56 w-full pt-2">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={formattedSeries} margin={{ top: 10, right: 10, left: -20, bottom: 0 }}>
          <defs>
            <linearGradient id="flameAreaGradient" x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor="#ff5540" stopOpacity={0.35} />
              <stop offset="95%" stopColor="#ff5540" stopOpacity={0.0} />
            </linearGradient>
          </defs>
          <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="var(--border)" opacity={0.35} />
          <XAxis
            dataKey="label"
            axisLine={false}
            tickLine={false}
            tick={{ fontSize: 11, fill: "var(--color-muted-foreground)" }}
            interval="preserveStartEnd"
            minTickGap={30}
          />
          <YAxis
            axisLine={false}
            tickLine={false}
            tick={{ fontSize: 11, fill: "var(--color-muted-foreground)" }}
            allowDecimals={false}
          />
          <Tooltip content={<CustomChartTooltip />} cursor={{ stroke: "var(--color-primary)", strokeWidth: 1, strokeDasharray: "4 4" }} />
          <Area
            type="monotone"
            dataKey="count"
            stroke="#ff5540"
            strokeWidth={2.5}
            fillOpacity={1}
            fill="url(#flameAreaGradient)"
            activeDot={{ r: 5, fill: "#ff5540", stroke: "#ffffff", strokeWidth: 2 }}
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}

/* ── Provider breakdown ──────────────────────────────────────────── */

const PROVIDER_COLORS: Record<string, string> = {
  antigravity: "#3b82f6",
  google: "#4285F4",
  cline: "#8b5cf6",
  openai: "#10a37f",
  anthropic: "#d97706",
  mimo: "#f97316",
  xiaomi: "#f97316",
  deepseek: "#0ea5e9",
};

function getProviderColor(providerSlug: string, customColor?: string): string {
  if (customColor && customColor.startsWith("#")) return customColor;
  const lower = providerSlug.toLowerCase();
  for (const [key, color] of Object.entries(PROVIDER_COLORS)) {
    if (lower.includes(key)) return color;
  }
  return "#ff5540";
}

function ProviderBreakdown({
  providers,
  totalRequests,
}: {
  providers: ProviderUsage[];
  totalRequests: number;
}) {
  if (!providers || providers.length === 0) {
    return (
      <div className="flex h-48 items-center justify-center rounded-lg border border-dashed border-border/60 text-sm text-muted-foreground">
        No provider usage recorded yet.
      </div>
    );
  }

  const calculatedTotal = totalRequests > 0 ? totalRequests : providers.reduce((sum, p) => sum + p.total_requests, 0);

  return (
    <div className="space-y-4 pt-1">
      {providers.map((p) => {
        const color = getProviderColor(p.provider, p.color);
        const percent = calculatedTotal > 0 ? (p.total_requests / calculatedTotal) * 100 : 0;

        return (
          <div key={p.provider} className="group rounded-lg border border-border/50 bg-muted/20 p-3 hover:bg-muted/40 transition-colors">
            <div className="flex items-center justify-between text-sm mb-2">
              <div className="flex items-center gap-2.5">
                <SmallProviderIcon p={p} />
                <span className="font-semibold text-foreground">{p.display_name}</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="rounded-md border border-border bg-card px-2 py-0.5 text-xs font-semibold tabular-nums text-foreground">
                  {p.total_requests.toLocaleString()} req
                </span>
              </div>
            </div>

            {/* Modern sleek progress track */}
            <div className="h-2 w-full bg-muted/80 rounded-full relative overflow-hidden">
              <div
                className="h-full rounded-full transition-all duration-500 shadow-sm"
                style={{
                  width: `${Math.max(percent, 2)}%`,
                  backgroundColor: color,
                }}
              />
            </div>

            <div className="flex items-center justify-between text-xs text-muted-foreground mt-2">
              <span className="font-medium">{percent.toFixed(1)}% of total traffic</span>
              <span className="font-semibold tabular-nums text-foreground/90">${p.cost_usd.toFixed(4)}</span>
            </div>
          </div>
        );
      })}
    </div>
  );
}

/* ── Token breakdown ─────────────────────────────────────────────── */

function TokenBreakdown({ summary }: { summary: UsageInsights["summary"] }) {
  const prompt = summary.prompt_tokens;
  const completion = summary.completion_tokens;
  const cached = summary.cached_tokens;
  const total = prompt + completion;
  const cacheHits = summary.cache_hits;

  if (total === 0 && cached === 0) {
    return (
      <div className="flex h-48 flex-col items-center justify-center rounded-lg border border-dashed border-border/60 text-sm text-muted-foreground">
        <Sparkles className="size-6 text-muted-foreground/40 mb-2" />
        No token data recorded yet.
      </div>
    );
  }

  const promptPct = total > 0 ? (prompt / total) * 100 : 0;
  const completionPct = total > 0 ? (completion / total) * 100 : 0;

  return (
    <div className="flex flex-col justify-between h-full space-y-4 pt-1">
      {/* Top Total Header */}
      <div className="rounded-lg border border-border/50 bg-muted/20 p-3">
        <div className="flex items-baseline justify-between">
          <div>
            <p className="text-xs uppercase tracking-wider text-muted-foreground font-medium">Total Processed</p>
            <p className="text-3xl font-bold tracking-tight text-foreground tabular-nums mt-0.5">{compact(total)}</p>
          </div>
          <div className="text-right">
            <span className="inline-flex items-center gap-1 rounded-md border border-emerald-500/20 bg-emerald-500/10 px-2 py-0.5 text-xs font-semibold text-emerald-400">
              <Clock className="size-3" />
              {cacheHits.toLocaleString()} Cache Hits
            </span>
          </div>
        </div>

        {/* Visual segmented bar */}
        <div className="mt-3 flex h-2 w-full overflow-hidden rounded-full bg-muted/80">
          <div
            className="h-full bg-primary transition-all duration-500"
            style={{ width: `${promptPct}%` }}
            title={`Prompt: ${promptPct.toFixed(1)}%`}
          />
          <div
            className="h-full bg-amber-400 transition-all duration-500"
            style={{ width: `${completionPct}%` }}
            title={`Completion: ${completionPct.toFixed(1)}%`}
          />
        </div>
      </div>

      {/* Rows breakdown */}
      <div className="space-y-2.5">
        <div className="flex items-center justify-between rounded-lg border border-border/40 bg-card/40 p-2.5 text-xs">
          <div className="flex items-center gap-2">
            <span className="size-2.5 rounded-full bg-primary" />
            <span className="text-muted-foreground font-medium">Prompt tokens</span>
          </div>
          <div className="flex items-center gap-2 font-semibold text-foreground tabular-nums">
            <span>{prompt.toLocaleString()}</span>
            <span className="text-muted-foreground font-normal">({promptPct.toFixed(0)}%)</span>
          </div>
        </div>

        <div className="flex items-center justify-between rounded-lg border border-border/40 bg-card/40 p-2.5 text-xs">
          <div className="flex items-center gap-2">
            <span className="size-2.5 rounded-full bg-amber-400" />
            <span className="text-muted-foreground font-medium">Completion tokens</span>
          </div>
          <div className="flex items-center gap-2 font-semibold text-foreground tabular-nums">
            <span>{completion.toLocaleString()}</span>
            <span className="text-muted-foreground font-normal">({completionPct.toFixed(0)}%)</span>
          </div>
        </div>

        <div className="flex items-center justify-between rounded-lg border border-border/40 bg-card/40 p-2.5 text-xs">
          <div className="flex items-center gap-2">
            <span className="size-2.5 rounded-full bg-sky-400" />
            <span className="text-muted-foreground font-medium">Cached tokens</span>
          </div>
          <div className="flex items-center gap-2 font-semibold text-foreground tabular-nums">
            <span>{cached.toLocaleString()}</span>
            <span className="text-emerald-400 font-medium">Saved</span>
          </div>
        </div>
      </div>
    </div>
  );
}

/* ── Recent activity table ───────────────────────────────────────── */

function RecentActivityTable({
  recent,
  providers,
}: {
  recent: RecentActivity[];
  providers: ProviderUsage[];
}) {
  if (!recent || recent.length === 0) {
    return (
      <div className="px-6 py-12 text-center text-sm text-muted-foreground">
        No recent activity. Send a request through the proxy to see live metrics here.
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border/80 text-left text-xs uppercase tracking-wider text-muted-foreground bg-muted/20">
            <th className="px-6 py-3 font-semibold">Model</th>
            <th className="px-4 py-3 font-semibold">Provider</th>
            <th className="px-4 py-3 text-right font-semibold">Tokens</th>
            <th className="px-4 py-3 text-right font-semibold">Cost</th>
            <th className="px-4 py-3 text-right font-semibold">Latency</th>
            <th className="px-6 py-3 text-right font-semibold">Status</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border/60">
          {recent.map((row) => (
            <tr key={row.id} className="group hover:bg-muted/30 transition-colors">
              <td className="px-6 py-3">
                <span className="font-mono text-xs font-semibold text-foreground">{row.model}</span>
              </td>
              <td className="px-4 py-3 text-xs text-muted-foreground">
                <div className="flex items-center gap-2">
                  <SmallProviderIcon p={providers.find((p) => p.provider === row.provider)} />
                  <span className="font-medium text-foreground/90 capitalize">{row.provider}</span>
                </div>
              </td>
              <td className="px-4 py-3 text-right font-mono text-xs text-foreground tabular-nums font-medium">
                {row.tokens.toLocaleString()}
              </td>
              <td className="px-4 py-3 text-right font-mono text-xs text-muted-foreground tabular-nums">
                ${row.cost_usd.toFixed(4)}
              </td>
              <td className="px-4 py-3 text-right font-mono text-xs text-muted-foreground tabular-nums">
                {row.latency_ms}ms
              </td>
              <td className="px-6 py-3 text-right">
                {row.cache_hit ? (
                  <span className="inline-flex items-center gap-1 rounded-md border border-emerald-500/20 bg-emerald-500/10 px-2 py-0.5 text-xs font-semibold text-emerald-400">
                    <span className="size-1.5 rounded-full bg-emerald-400 animate-pulse" />
                    Cache Hit
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1 rounded-md border border-border/80 bg-muted/40 px-2 py-0.5 text-xs text-muted-foreground">
                    Direct
                  </span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/* ── Helpers ─────────────────────────────────────────────────────── */

function compact(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}

function SmallProviderIcon({ p }: { p?: { display_name: string; icon: string; color: string } }) {
  const [errored, setErrored] = useState(false);
  if (!p) return <div className="size-4 shrink-0 rounded bg-muted" />;
  if (errored || !p.icon) {
    return (
      <div
        className="flex size-4 shrink-0 items-center justify-center rounded text-[8px] font-bold text-white shadow-xs"
        style={{ backgroundColor: p.color || "#ff5540" }}
      >
        {p.display_name.slice(0, 1).toUpperCase()}
      </div>
    );
  }
  return (
    <img
      src={p.icon}
      alt={p.display_name}
      onError={() => setErrored(true)}
      className="size-4 shrink-0 object-contain rounded"
    />
  );
}
