import { useState } from "react";
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
} from "lucide-react";
import { BarChart, Bar, XAxis, YAxis, ResponsiveContainer, Tooltip } from "recharts";
import { api, type UsageInsights, type RecentActivity, type ProviderUsage } from "../lib/api";
import { microsToUSD } from "../lib/format";
import { PageHeader } from "@/components/composite/page-header";
import { Card } from "@/components/ui/card";
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
        description="Usage and performance across all providers."
        action={
          <div className="flex items-center gap-2 rounded-xl border border-border bg-muted/60 px-3 py-2">
            <Calendar className="h-4 w-4 text-muted-foreground" />
            <Select
              value={period}
              onChange={(e) => setPeriod(e.target.value)}
              className="border-0 bg-transparent px-0 py-0 h-auto text-sm font-medium text-foreground"
            >
              {periods.map((p) => (
                <option key={p.value} value={p.value}>
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
          <Skeleton key={i} className="h-28" />
        ))}
      </div>
      <Skeleton className="h-64" />
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <Skeleton className="h-64" />
        <Skeleton className="h-64" />
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
        />
        <StatCard
          label="Est. Cost"
          value={`$${costUSD.toFixed(2)}`}
          icon={DollarSign}
        />
        <StatCard
          label="Total Tokens"
          value={compact(totalTokens)}
          icon={Database}
        />
        <StatCard
          label="Avg Latency"
          value={avgLatency > 0 ? `${avgLatency}ms` : summary.total_requests > 0 ? "<1ms" : "—"}
          icon={Timer}
        />
      </div>

      {/* ── Secondary KPIs ───────────────────────────────────────── */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard
          label="Prompt Tokens"
          value={compact(summary.prompt_tokens)}
          icon={Zap}
        />
        <StatCard
          label="Completion Tokens"
          value={compact(summary.completion_tokens)}
          icon={TrendingUp}
        />
        <StatCard
          label="Success Rate"
          value={successRate != null ? `${(successRate * 100).toFixed(1)}%` : summary.total_requests > 0 ? "—" : "100%"}
          icon={CheckCircle2}
        />
        <StatCard
          label="Cached Tokens"
          value={compact(summary.cached_tokens)}
          icon={Clock}
        />
      </div>

      {/* ── Activity chart ───────────────────────────────────────── */}
      <Card>
        <SectionHeader
          title="Activity over time"
          description="Request volume in the selected time window."
          icon={Activity}
        />
        <ActivityChart series={series} />
      </Card>

      {/* ── Breakdown grid ───────────────────────────────────────── */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <Card>
          <SectionHeader
            title="Provider distribution"
            description="Share of total requests across providers."
            icon={Database}
          />
          <ProviderBreakdown providers={providers} />
        </Card>

        <Card>
          <SectionHeader
            title="Token breakdown"
            description="Input vs. output tokens."
            icon={Zap}
          />
          <TokenBreakdown summary={summary} />
        </Card>

        <Card className="lg:col-span-2">
          <SectionHeader
            title="Recent activity"
            description="Latest requests through the proxy."
            icon={Clock}
            action={
              recent.length > 0 ? (
                <span className="text-xs text-muted-foreground">
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

/* ── Activity sparkline chart ────────────────────────────────────── */

function ActivityChart({ series }: { series: UsageInsights["series"] }) {
  if (series.length === 0) {
    return (
      <div className="flex h-48 items-center justify-center text-sm text-muted-foreground">
        No activity recorded for this period.
      </div>
    );
  }

  return (
    <div className="activity-chart relative h-48">
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-0 opacity-[0.05]"
        style={{
          backgroundImage:
            "linear-gradient(to right, #333 1px, transparent 1px), linear-gradient(to bottom, #333 1px, transparent 1px)",
          backgroundSize: "40px 40px",
        }}
      />
      <ResponsiveContainer width="100%" height="100%">
        <BarChart data={series} barCategoryGap="20%">
          <XAxis
            dataKey="label"
            axisLine={false}
            tickLine={false}
            tick={{ fontSize: 11, fill: "var(--color-ink-400)" }}
            interval="preserveStartEnd"
          />
          <YAxis
            axisLine={false}
            tickLine={false}
            tick={{ fontSize: 11, fill: "var(--color-ink-400)" }}
            width={36}
          />
          <Tooltip
            cursor={{ fill: "rgba(255, 255, 255, 0.05)" }}
            contentStyle={{
              background: "var(--color-ink-900, #1c1b18)",
              border: "1px solid var(--border)",
              borderRadius: "8px",
              fontSize: "12px",
              padding: "6px 10px",
              color: "var(--color-ink-50, #faf9f7)",
            }}
          />
          <Bar
            dataKey="count"
            fill="var(--color-chart-1)"
            radius={[4, 4, 0, 0]}
          />
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

/* ── Provider breakdown ──────────────────────────────────────────── */

function ProviderBreakdown({ providers }: { providers: ProviderUsage[] }) {
  if (providers.length === 0) {
    return (
      <div className="py-8 text-center text-sm text-muted-foreground">
        No provider usage yet.
      </div>
    );
  }

  const maxRequests = Math.max(...providers.map((p) => p.total_requests));

  return (
    <div className="divide-y divide-border">
      {providers.map((p) => (
        <div key={p.provider} className="py-3 first:pt-0 last:pb-0">
          <div className="flex items-center justify-between text-sm mb-2">
            <div className="flex items-center gap-2">
              <SmallProviderIcon p={p} />
              <span className="font-medium text-foreground">{p.display_name}</span>
            </div>
            <span className="font-mono text-xs tabular-nums text-foreground">
              {p.total_requests.toLocaleString()} req
            </span>
          </div>
          <div className="h-1.5 bg-muted rounded-full relative overflow-hidden">
            <div
              className="absolute inset-y-0 left-0 bg-primary rounded-full transition-all duration-500"
              style={{ width: `${maxRequests > 0 ? (p.total_requests / maxRequests) * 100 : 0}%`, backgroundColor: p.color || "var(--color-chart-1)" }}
            />
          </div>
          <div className="flex items-center justify-between text-xs text-muted-foreground mt-2 font-mono">
            <span>
              {((p.total_requests / (maxRequests || 1)) * 100).toFixed(0)}% of max
            </span>
            <span>${p.cost_usd.toFixed(2)}</span>
          </div>
        </div>
      ))}
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

  if (total === 0) {
    return (
      <div className="py-8 text-center text-sm text-muted-foreground">
        No token data recorded yet.
      </div>
    );
  }

  const rows = [
    { label: "Prompt tokens", value: prompt },
    { label: "Completion tokens", value: completion },
    { label: "Cached tokens", value: cached },
  ];

  return (
    <div className="flex flex-col h-full">
      <div className="pb-6 mb-6 border-b border-border">
        <div className="flex items-baseline gap-2">
          <span className="font-display text-4xl font-semibold tracking-tight text-foreground">{compact(total)}</span>
          <span className="text-xs uppercase tracking-wider text-muted-foreground">tokens</span>
        </div>
      </div>

      <div className="space-y-4 flex-1">
        {rows.map((row) => (
          <div key={row.label} className="flex items-center justify-between">
            <span className="text-sm text-muted-foreground">{row.label}</span>
            <div className="flex items-center gap-4">
              <span className="text-sm font-mono tabular-nums text-foreground">
                {row.value.toLocaleString()}
              </span>
              <span className="w-10 text-right text-xs font-mono text-muted-foreground">
                {total > 0 ? `${((row.value / total) * 100).toFixed(0)}%` : "0%"}
              </span>
            </div>
          </div>
        ))}
      </div>

      <div className="pt-4 mt-6 border-t border-border">
        <div className="flex items-center justify-between">
          <span className="text-sm text-muted-foreground">Cache hits</span>
          <span className="text-sm font-mono tabular-nums text-foreground">{cacheHits.toLocaleString()}</span>
        </div>
      </div>
    </div>
  );
}

/* ── Recent activity table ───────────────────────────────────────── */

function RecentActivityTable({ recent, providers }: { recent: RecentActivity[], providers: ProviderUsage[] }) {
  if (recent.length === 0) {
    return (
      <div className="px-6 py-10 text-center text-sm text-muted-foreground">
        No recent activity. Make a request through the proxy to see it here.
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border text-left text-xs uppercase tracking-wider text-muted-foreground">
            <th className="px-6 py-3 font-medium">Model</th>
            <th className="px-4 py-3 font-medium">Provider</th>
            <th className="px-4 py-3 text-right font-medium">Tokens</th>
            <th className="px-4 py-3 text-right font-medium">Cost</th>
            <th className="px-4 py-3 text-right font-medium">Latency</th>
            <th className="px-6 py-3 text-right font-medium">Cache</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {recent.map((row) => (
            <tr
              key={row.id}
              className="group hover:bg-muted/40 transition-colors"
            >
              <td className="px-6 py-3">
                <span className="font-mono text-xs text-foreground">{row.model}</span>
              </td>
              <td className="px-4 py-3 text-xs text-muted-foreground">
                <div className="flex items-center gap-2">
                  <SmallProviderIcon p={providers.find((p) => p.provider === row.provider)} />
                  {row.provider}
                </div>
              </td>
              <td className="px-4 py-3 text-right font-mono text-xs text-foreground">
                {row.tokens.toLocaleString()}
              </td>
              <td className="px-4 py-3 text-right font-mono text-xs text-muted-foreground">
                ${row.cost_usd.toFixed(4)}
              </td>
              <td className="px-4 py-3 text-right font-mono text-xs text-muted-foreground">
                {row.latency_ms}ms
              </td>
              <td className="px-6 py-3 text-right">
                {row.cache_hit ? (
                  <span className="inline-flex items-center gap-1.5 text-xs text-emerald-500 font-medium">
                    <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
                    Hit
                  </span>
                ) : null}
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
  if (!p) return <div className="h-4 w-4 shrink-0 bg-muted" />;
  if (errored || !p.icon) {
    return (
      <div
        className="flex h-4 w-4 shrink-0 items-center justify-center text-[8px] font-bold text-white rounded"
        style={{ backgroundColor: p.color || "var(--color-ink-400)" }}
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
      className="h-4 w-4 shrink-0 object-contain rounded"
    />
  );
}
