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
        <div className="mb-6 flex items-center gap-3 border border-error-container/50 bg-error-container/10 px-4 py-3">
          <AlertTriangle className="h-5 w-5 shrink-0 text-error" />
          <div className="flex-1">
            <p className="text-sm font-medium text-on-surface">Plan limit reached. Requests blocked.</p>
            <p className="text-xs text-on-surface-variant">
              {blocked.map((b) => `${b.scope_name} (${microsToUSD(b.limit_micros)} ${b.period})`).join(", ")}
            </p>
          </div>
          <a
            href="/plans"
            className="shrink-0 border border-error-container px-3 py-1.5 text-xs font-medium text-error transition-colors hover:bg-error-container"
          >
            Manage
          </a>
        </div>
      )}
      {warnings.length > 0 && blocked.length === 0 && (
        <div className="mb-6 flex items-center gap-3 border border-warning/30 bg-warning/10 px-4 py-3">
          <Wallet className="h-5 w-5 shrink-0 text-warning" />
          <div className="flex-1">
            <p className="text-sm font-medium text-on-surface">Plan alert</p>
            <p className="text-xs text-on-surface-variant">
              {warnings.map((b) => `${b.scope_name}: ${b.pct_used.toFixed(0)}% used`).join(", ")}
            </p>
          </div>
          <a
            href="/plans"
            className="shrink-0 border border-warning/30 px-3 py-1.5 text-xs font-medium text-warning transition-colors hover:bg-warning/20"
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
          <div className="flex items-center gap-2 border border-outline-variant bg-surface-container-high px-3 py-2">
            <Calendar className="h-4 w-4 text-on-surface-variant" />
            <Select
              value={period}
              onChange={(e) => setPeriod(e.target.value)}
              className="border-0 bg-transparent px-0 py-0 h-auto text-sm font-medium"
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

// OverviewSkeleton mirrors the InsightsDashboard layout so the page shows its
// shape while data loads, avoiding a blank spinner and layout shift on arrival.
function OverviewSkeleton() {
  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Card key={i} className="p-4">
            <Skeleton className="h-4 w-24" />
            <Skeleton className="mt-3 h-8 w-20" />
          </Card>
        ))}
      </div>
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <Card className="lg:col-span-2 p-6">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="mt-4 h-48 w-full" />
        </Card>
        <Card className="p-6">
          <Skeleton className="h-4 w-24" />
          <div className="mt-4 space-y-3">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-8 w-full" />
            ))}
          </div>
        </Card>
      </div>
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <Card className="p-6">
          <Skeleton className="h-4 w-28" />
          <Skeleton className="mt-4 h-10 w-32" />
          <div className="mt-4 space-y-2">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-4 w-full" />
            ))}
          </div>
        </Card>
        <Card className="lg:col-span-2 p-6">
          <Skeleton className="h-4 w-32" />
          <div className="mt-4 space-y-2">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-6 w-full" />
            ))}
          </div>
        </Card>
      </div>
    </div>
  );
}

function InsightsDashboard({ data }: { data: UsageInsights }) {
  const { summary, providers, recent, series } = data;

  return (
    <div className="space-y-6">
      {/* ── Key metrics ──────────────────────────────────────────── */}
      <div className="stagger-in grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          icon={Activity}
          iconTone="accent"
          label="Requests"
          value={summary.total_requests.toLocaleString()}
        />
        <StatCard
          icon={DollarSign}
          iconTone="warning"
          label="Cost"
          value={`$${summary.cost_usd.toFixed(2)}`}
        />
        <StatCard
          icon={CheckCircle2}
          iconTone="accent"
          label="Success rate"
          value={`${(summary.success_rate * 100).toFixed(1)}%`}
        />
        <StatCard
          icon={Timer}
          iconTone="accent"
          label="Avg TTFT"
          value={`${Math.round(summary.avg_ttft_ms)}ms`}
        />
      </div>

      {/* ── Activity chart + Provider breakdown ──────────────────── */}
      <div className="stagger-in grid grid-cols-1 gap-6 lg:grid-cols-3">
        <Card className="lg:col-span-2 active-glow">
          <SectionHeader
            title="Request volume"
            description="Requests over the selected period."
            icon={TrendingUp}
          />
          <div className="px-6 pb-6">
            <ActivityChart series={series} />
          </div>
        </Card>

        <Card>
          <SectionHeader
            title="Providers"
            description="Usage by provider."
            icon={Database}
          />
          <div className="px-6 pb-6">
            <ProviderBreakdown providers={providers} />
          </div>
        </Card>
      </div>

      {/* ── Token stats + Recent activity ────────────────────────── */}
      <div className="stagger-in grid grid-cols-1 gap-6 lg:grid-cols-3">
        <Card>
          <SectionHeader
            title="Token breakdown"
            description="Input, output, and cached tokens."
            icon={Zap}
          />
          <div className="px-6 pb-6">
            <TokenStats
              prompt={summary.prompt_tokens}
              completion={summary.completion_tokens}
              cached={summary.cached_tokens}
              cacheHits={summary.cache_hits}
            />
          </div>
        </Card>

        <Card className="lg:col-span-2">
          <SectionHeader
            title="Recent activity"
            description="Latest requests through the proxy."
            icon={Clock}
            action={
              recent.length > 0 ? (
                <span className="text-xs text-on-surface-variant">
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
      <div className="flex h-48 items-center justify-center text-sm text-on-surface-variant">
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
            tick={{ fontSize: 11, fill: "var(--color-on-surface-variant)" }}
            interval="preserveStartEnd"
          />
          <YAxis
            axisLine={false}
            tickLine={false}
            tick={{ fontSize: 11, fill: "var(--color-on-surface-variant)" }}
            width={36}
          />
          <Tooltip
            cursor={{ fill: "var(--color-surface-container-high)" }}
            contentStyle={{
              background: "var(--color-surface-container)",
              border: "1px solid var(--color-outline-variant)",
              borderRadius: "0",
              fontSize: "12px",
              padding: "6px 10px",
              color: "var(--color-on-surface)",
            }}
          />
          <Bar
            dataKey="count"
            fill="var(--color-primary-container)"
            radius={[0, 0, 0, 0]}
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
      <div className="py-8 text-center text-sm text-on-surface-variant">
        No provider usage yet.
      </div>
    );
  }

  const maxRequests = Math.max(...providers.map((p) => p.total_requests));

  return (
    <div className="divide-y divide-outline-variant/50">
      {providers.map((p) => (
        <div key={p.provider} className="py-3 first:pt-0 last:pb-0">
          <div className="flex items-center justify-between text-sm mb-2">
            <div className="flex items-center gap-2">
              <SmallProviderIcon p={p} />
              <span className="font-medium text-on-surface">{p.display_name}</span>
            </div>
            <span className="font-mono text-xs tabular-nums text-on-surface">
              {p.total_requests.toLocaleString()} req
            </span>
          </div>
          <div className="h-1 bg-surface-container-high relative overflow-hidden">
            <div
              className="absolute inset-y-0 left-0 bg-primary-container transition-all duration-500"
              style={{ width: `${maxRequests > 0 ? (p.total_requests / maxRequests) * 100 : 0}%`, boxShadow: "0 0 6px rgba(255, 85, 64, 0.45)" }}
            />
          </div>
          <div className="flex items-center justify-between text-xs text-on-surface-variant mt-2 font-mono">
            <span>{compact(p.prompt_tokens + p.completion_tokens)} tks</span>
            <span>${p.cost_usd.toFixed(4)}</span>
          </div>
        </div>
      ))}
    </div>
  );
}

/* ── Token stats (no chart, just clean numbers) ──────────────────── */

function TokenStats({
  prompt,
  completion,
  cached,
  cacheHits,
}: {
  prompt: number;
  completion: number;
  cached: number;
  cacheHits: number;
}) {
  const nonCachedInput = Math.max(prompt - cached, 0);
  const total = prompt + completion;

  if (total === 0) {
    return (
      <div className="py-8 text-center text-sm text-on-surface-variant">
        No token usage recorded yet.
      </div>
    );
  }

  const rows = [
    { label: "Input", value: nonCachedInput },
    { label: "Output", value: completion },
    { label: "Cached", value: cached },
  ];

  return (
    <div className="flex flex-col h-full">
      <div className="pb-6 mb-6 border-b border-outline-variant">
        <div className="flex items-baseline gap-2">
          <span className="font-display text-4xl font-semibold tracking-tight text-on-surface">{compact(total)}</span>
          <span className="text-xs uppercase tracking-wider text-on-surface-variant">tokens</span>
        </div>
      </div>

      <div className="space-y-4 flex-1">
        {rows.map((row) => (
          <div key={row.label} className="flex items-center justify-between">
            <span className="text-sm text-on-surface-variant">{row.label}</span>
            <div className="flex items-center gap-4">
              <span className="text-sm font-mono tabular-nums text-on-surface">
                {row.value.toLocaleString()}
              </span>
              <span className="w-10 text-right text-xs font-mono text-on-surface-variant">
                {total > 0 ? `${((row.value / total) * 100).toFixed(0)}%` : "0%"}
              </span>
            </div>
          </div>
        ))}
      </div>

      <div className="pt-4 mt-6 border-t border-outline-variant">
        <div className="flex items-center justify-between">
          <span className="text-sm text-on-surface-variant">Cache hits</span>
          <span className="text-sm font-mono tabular-nums text-on-surface">{cacheHits.toLocaleString()}</span>
        </div>
      </div>
    </div>
  );
}

/* ── Recent activity table ───────────────────────────────────────── */

function RecentActivityTable({ recent, providers }: { recent: RecentActivity[], providers: ProviderUsage[] }) {
  if (recent.length === 0) {
    return (
      <div className="px-6 py-10 text-center text-sm text-on-surface-variant">
        No recent activity. Make a request through the proxy to see it here.
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-outline-variant text-left text-xs uppercase tracking-wider text-on-surface-variant">
            <th className="px-6 py-3 font-medium">Model</th>
            <th className="px-4 py-3 font-medium">Provider</th>
            <th className="px-4 py-3 text-right font-medium">Tokens</th>
            <th className="px-4 py-3 text-right font-medium">Cost</th>
            <th className="px-4 py-3 text-right font-medium">Latency</th>
            <th className="px-6 py-3 text-right font-medium">Cache</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-outline-variant/50">
          {recent.map((row) => (
            <tr
              key={row.id}
              className="group hover:bg-surface-container-high/50 transition-colors"
            >
              <td className="px-6 py-3">
                <span className="font-mono text-xs text-on-surface">{row.model}</span>
              </td>
              <td className="px-4 py-3 text-xs text-on-surface-variant">
                <div className="flex items-center gap-2">
                  <SmallProviderIcon p={providers.find((p) => p.provider === row.provider)} />
                  {row.provider}
                </div>
              </td>
              <td className="px-4 py-3 text-right font-mono text-xs text-on-surface">
                {row.tokens.toLocaleString()}
              </td>
              <td className="px-4 py-3 text-right font-mono text-xs text-on-surface-variant">
                ${row.cost_usd.toFixed(4)}
              </td>
              <td className="px-4 py-3 text-right font-mono text-xs text-on-surface-variant">
                {row.latency_ms}ms
              </td>
              <td className="px-6 py-3 text-right">
                {row.cache_hit ? (
                  <span className="inline-flex items-center gap-1.5 text-xs text-success font-medium">
                    <span className="h-1.5 w-1.5 bg-current" />
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
  if (!p) return <div className="h-4 w-4 shrink-0 bg-outline-variant" />;
  if (errored || !p.icon) {
    return (
      <div
        className="flex h-4 w-4 shrink-0 items-center justify-center text-[8px] font-bold text-white"
        style={{ backgroundColor: p.color || "var(--color-on-surface-variant)" }}
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
      className="h-4 w-4 shrink-0 object-contain"
    />
  );
}
