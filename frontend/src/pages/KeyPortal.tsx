import { useState, useEffect, useMemo } from "react";
import { useSearch } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import {
  AreaChart, Area, BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer,
  CartesianGrid, PieChart, Pie, Cell,
} from "recharts";
import { fetchKeyUsage, fetchKeyUsageById, APIError, BASE_URL, type KeyUsageData } from "../lib/api"
import { useBranding } from "../contexts/BrandingContext";
import {
  AlertTriangle, CheckCircle2, Activity, ArrowDownRight, ArrowUpRight, DollarSign,
  LogOut, Layers, Key, Zap, Send, ChevronDown, Radio, TrendingUp, Coins, Calendar,
  Trophy, Infinity as InfinityIcon,
} from "lucide-react";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect as Select } from "@/components/composite/native-select";
import { Spinner } from "@/components/ui/spinner";
import { ErrorCard } from "@/components/composite/error-card";
import { Badge } from "@/components/ui/badge";
import { SegmentedControl } from "@/components/composite/tab-bar";


// Chart palette pulled from the design-system CSS variables so the portal
// matches the admin dashboard in both light and dark themes.
const C_INPUT = "var(--color-chart-1)";
const C_OUTPUT = "var(--color-chart-2)";
const C_COST = "var(--color-chart-3)";
const C_REQ = "var(--color-chart-5)";

export function KeyPortalPage() {
  const { branding, logoSrc } = useBranding();
  const searchParams = useSearch({ strict: false })
  const params = new URLSearchParams(searchParams as any);
  const activeId = params.get("id") || "";
  const activeKey = params.get("key") || "";
  const [apiKeyInput, setApiKeyInput] = useState(activeKey || activeId);
  const [selectedModel, setSelectedModel] = useState("");
  const [testPrompt, setTestPrompt] = useState("Say hello in one sentence");
  const [testResponse, setTestResponse] = useState<any>(null);
  const [isTesting, setIsTesting] = useState(false);

  const authValue = activeId || activeKey;
  const isIdMode = !!activeId;

  const handleLogin = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const val = apiKeyInput.trim();
    if (val) {
      if (val.startsWith("sk-")) window.location.search = new URLSearchParams({ key: val }).toString();
      else window.location.search = new URLSearchParams({ id: val }).toString();
    }
  };

  const handleLogout = () => {
    window.history.replaceState(null, "", window.location.pathname);
    setApiKeyInput("");
  };

  const { data, isLoading, isError, error, dataUpdatedAt } = useQuery({
    queryKey: ["key-usage", authValue, isIdMode],
    queryFn: () => (isIdMode ? fetchKeyUsageById(authValue) : fetchKeyUsage(authValue)),
    enabled: !!authValue,
    retry: false,
    refetchInterval: 30000,
  });

  useEffect(() => {
    if (data?.allowed_models?.length && !selectedModel) {
      setSelectedModel(data.allowed_models[0]);
    }
  }, [data]);

  if (!authValue) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background p-4 md:p-8">
        <div className="w-full max-w-md animate-[page-in_0.3s_ease-out]">
          <Card className="p-8 md:p-10 text-center shadow-float border-0 ring-1 ring-border">
            <div className="mx-auto mb-6 flex h-16 w-16 items-center justify-center rounded-2xl bg-muted ring-1 ring-inset ring-border">
              <img src={logoSrc} alt={branding.name || "FlameGate"} className="h-8 object-contain" />
            </div>
            <h1 className="mb-2 text-2xl font-display tracking-tight text-foreground">Portal Access</h1>
            <p className="mb-8 text-sm text-muted-foreground">
              {branding.tagline || "Enter your Key or Portal ID to monitor your real-time usage and budgets."}
            </p>

            <form onSubmit={handleLogin} className="space-y-5 text-left">
              <div className="space-y-1.5">
                <label className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">
                  Identifier
                </label>
                <div className="relative">
                  <div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3 text-muted-foreground">
                    <Key size={16} />
                  </div>
                  <Input
                    type="password"
                    value={apiKeyInput}
                    onChange={(e) => setApiKeyInput(e.target.value)}
                    placeholder="sk-... or key_..."
                    className="pl-10 h-11 bg-background"
                    autoFocus
                  />
                </div>
              </div>
              <Button
                type="submit"
                className="w-full h-11 text-base font-medium shadow-sm transition-all hover:-translate-y-px"
                disabled={!apiKeyInput.trim()}
              >
                View Dashboard
              </Button>
            </form>
          </Card>
        </div>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <Spinner />
      </div>
    );
  }

  if (isError) {
    let msg = "Authentication failed or server error.";
    if (error instanceof APIError) msg = error.message;
    return (
      <div className="flex min-h-screen items-center justify-center bg-background p-4">
        <div className="w-full max-w-md space-y-4 text-center animate-[page-in_0.3s_ease-out]">
          <ErrorCard message={msg} />
          <Button variant="ghost" onClick={handleLogout} className="rounded-xl">
            Return to Login
          </Button>
        </div>
      </div>
    );
  }

  const d = data!;

  return (
    <div className="min-h-screen bg-background p-4 md:p-10 animate-[page-in_0.4s_ease-out]">
      <div className="mx-auto max-w-[1040px] space-y-8">

        {/* ── Header ─────────────────────────────────────────────────── */}
        <header className="flex flex-col gap-6 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-5">
            <div className="flex h-16 w-16 shrink-0 items-center justify-center rounded-2xl bg-popover border border-border shadow-sm">
              <img src={logoSrc} alt={branding.name || "FlameGate"} className="h-8 object-contain" />
            </div>
            <div>
              <h1 className="text-2xl md:text-3xl font-display font-semibold text-foreground tracking-tight">Usage Dashboard</h1>
              <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1">
                <span className="text-sm text-muted-foreground">Monitoring for <strong className="font-medium text-foreground">{d.key_name}</strong></span>
                <span className="hidden h-1 w-1 rounded-full bg-[var(--border-strong)] sm:inline-block" />
                <span className="font-mono text-[13px] text-muted-foreground tracking-tight">ID: {d.key_id}</span>
              </div>
            </div>
          </div>
          <div className="flex items-center gap-3 self-start sm:self-auto">
            <LiveIndicator updatedAt={dataUpdatedAt} />
            <Button variant="ghost" onClick={handleLogout} className="shrink-0 hover:bg-popover rounded-xl px-4 py-2 border-border shadow-sm">
              <LogOut size={16} /> <span className="ml-1 font-medium">Disconnect</span>
            </Button>
          </div>
        </header>

        {/* ── Overview: Allocations + 30-day KPIs ────────────────────── */}
        <OverviewSection d={d} />

        {/* ── Usage Trend (multi-metric) ─────────────────────────────── */}
        {d.daily && d.daily.length > 0 && <TrendSection daily={d.daily} />}

        {/* ── Composition + Highlights ───────────────────────────────── */}
        {d.daily && d.daily.length > 0 && <InsightsSection d={d} />}

        {/* ── Model breakdown ────────────────────────────────────────── */}
        {d.models && d.models.length > 0 && <ModelSection models={d.models} />}

        {/* ── Authorized routes ──────────────────────────────────────── */}
        {d.allowed_models && d.allowed_models.length > 0 && (
          <section className="space-y-4">
            <SectionTitle title="Authorized Routes" icon={<Key size={17} />} count={d.allowed_models.length} />
            <Card className="p-6 md:p-7">
              <div className="flex flex-wrap gap-2.5">
                {d.allowed_models.map((m) => (
                  <div key={m} className="flex items-center gap-2 rounded-full border border-border bg-muted/60 px-3.5 py-1.5 text-sm text-foreground transition-colors hover:bg-muted">
                    <CheckCircle2 size={15} className="text-accent-500" />
                    <span className="font-mono text-[13px] tracking-tight">{m}</span>
                  </div>
                ))}
              </div>
            </Card>
          </section>
        )}

        {/* ── Playground (only when authed with a live key) ──────────── */}
        {!isIdMode && activeKey && d.allowed_models && d.allowed_models.length > 0 && (
          <PlaygroundSection
            allowedModels={d.allowed_models}
            activeKey={activeKey}
            selectedModel={selectedModel}
            setSelectedModel={setSelectedModel}
            testPrompt={testPrompt}
            setTestPrompt={setTestPrompt}
            testResponse={testResponse}
            setTestResponse={setTestResponse}
            isTesting={isTesting}
            setIsTesting={setIsTesting}
          />
        )}

        <footer className="pt-2 pb-8 text-center text-xs text-muted-foreground">
          Metrics reflect the last 30 days · refreshed automatically every 30 seconds
        </footer>
      </div>
    </div>
  );
}

// ─── Overview: budget allocations + 30-day KPI cards ──────────────────────
function OverviewSection({ d }: { d: KeyUsageData }) {
  const daily = d.daily ?? [];
  const t = useMemo(() => aggregate(daily), [daily]);
  const totalTokens = t.prompt + t.completion;
  const inputPct = totalTokens ? Math.round((t.prompt / totalTokens) * 100) : 0;
  const activeDays = daily.filter((x) => x.requests > 0).length;
  const avgPerReq = t.requests ? Math.round(totalTokens / t.requests) : 0;

  return (
    <section>
      <div className="grid grid-cols-1 lg:grid-cols-12 gap-5">
        {/* Allocations */}
        <div className="lg:col-span-5">
          <Card className="h-full p-7 md:p-8 flex flex-col">
            <div className="mb-6 flex items-center justify-between">
              <h2 className="text-xs font-semibold tracking-widest text-muted-foreground uppercase">Allocations</h2>
              {d.budgets && d.budgets.length > 0 && (
                <Badge variant={d.budgets.some((b) => b.alert) ? "danger" : "neutral"}>
                  {d.budgets.length} limit{d.budgets.length === 1 ? "" : "s"}
                </Badge>
              )}
            </div>

            {d.budgets && d.budgets.length > 0 ? (
              <div className="flex-1 flex flex-col justify-center space-y-9">
                {d.budgets.map((b, i) => (
                  <div key={i}>
                    <div className="mb-5 flex items-center justify-between">
                      <div className="flex items-center gap-3">
                        <span className={`h-2.5 w-2.5 rounded-full ${b.alert ? "bg-[color:var(--color-danger)] shadow-[0_0_10px_var(--color-danger)]" : "bg-accent-500 shadow-[0_0_10px_var(--color-accent-500)]"}`} />
                        <h3 className="text-xl font-display font-semibold tracking-tight text-foreground">
                          {b.period === "total" ? "All-Time" : b.period.charAt(0).toUpperCase() + b.period.slice(1)} Limit
                        </h3>
                      </div>
                      {b.alert && (
                        <Badge variant="destructive">
                          <span className="flex items-center gap-1.5"><AlertTriangle size={13} /> Exceeded</span>
                        </Badge>
                      )}
                    </div>
                    <div className="space-y-6">
                      {b.limit_tokens > 0 && (
                        <BudgetProgress label="Tokens" used={b.tokens_used} limit={b.limit_tokens} pct={b.tokens_pct_used} alert={b.alert} remaining={b.tokens_remaining} format={formatTokens} />
                      )}
                      {b.limit_usd > 0 && (
                        <BudgetProgress label="Spend" used={b.spent_usd} limit={b.limit_usd} pct={b.usd_pct_used} alert={b.alert} remaining={b.usd_remaining} format={(v: number) => `$${v.toFixed(2)}`} />
                      )}
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <div className="flex-1 flex flex-col items-center justify-center text-center py-6">
                <div className="mb-5 flex h-16 w-16 items-center justify-center rounded-full bg-accent-50 text-accent-600 ring-4 ring-accent-50/50 dark:bg-accent-900/20 dark:ring-accent-900/30">
                  <InfinityIcon size={30} strokeWidth={1.75} />
                </div>
                <h3 className="text-xl font-display font-semibold text-foreground">Unrestricted</h3>
                <p className="mt-2 text-sm text-muted-foreground max-w-xs">This key has no configured budget limits and can be used indefinitely.</p>
              </div>
            )}
          </Card>
        </div>

        {/* 30-day KPI cards */}
        <div className="lg:col-span-7">
          <div className="mb-3 flex items-center justify-between px-1">
            <h2 className="text-xs font-semibold tracking-widest text-muted-foreground uppercase">Last 30 Days</h2>
            <span className="text-xs text-muted-foreground">{activeDays} active day{activeDays === 1 ? "" : "s"}</span>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <KpiCard icon={Activity} label="Requests" value={formatNumber(t.requests)} sub={`${avgPerReq ? formatTokens(avgPerReq) : 0} tokens / req`} color={C_REQ} data={daily} dataKey="requests" sparkId="sp-req" />
            <KpiCard icon={DollarSign} label="Total Cost" value={`$${t.cost.toFixed(4)}`} sub={activeDays ? `~$${(t.cost / activeDays).toFixed(2)} / day` : "no spend yet"} color={C_COST} accent data={daily} dataKey="cost_usd" sparkId="sp-cost" />
            <KpiCard icon={ArrowDownRight} label="Input Tokens" value={formatTokens(t.prompt)} sub={`${inputPct}% of tokens`} color={C_INPUT} data={daily} dataKey="prompt_tokens" sparkId="sp-in" />
            <KpiCard icon={ArrowUpRight} label="Output Tokens" value={formatTokens(t.completion)} sub={`${100 - inputPct}% of tokens`} color={C_OUTPUT} data={daily} dataKey="completion_tokens" sparkId="sp-out" />
          </div>
          <div className="mt-4 flex items-center gap-2 rounded-xl border border-border bg-muted/40 px-4 py-2.5 text-xs text-muted-foreground">
            <Calendar size={14} className="text-muted-foreground" />
            This month so far:
            <strong className="font-medium text-foreground">{formatNumber(d.current_period.total_requests)}</strong> requests ·
            <strong className="font-medium text-foreground">${d.current_period.cost_usd.toFixed(4)}</strong> spent
          </div>
        </div>
      </div>
    </section>
  );
}

// ─── Usage trend chart with metric toggle ─────────────────────────────────
type Metric = "tokens" | "requests" | "cost";

function TrendSection({ daily }: { daily: NonNullable<KeyUsageData["daily"]> }) {
  const [metric, setMetric] = useState<Metric>("tokens");
  const chartData = useMemo(() => daily.map((dp) => ({ ...dp, label: dp.date.slice(5) })), [daily]);
  const t = useMemo(() => aggregate(daily), [daily]);

  const headline =
    metric === "tokens" ? formatTokens(t.prompt + t.completion) + " tokens"
      : metric === "requests" ? formatNumber(t.requests) + " requests"
        : "$" + t.cost.toFixed(4);

  return (
    <section className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <SectionTitle title="Usage Trend" icon={<TrendingUp size={17} />} />
        <SegmentedControl<Metric>
          value={metric}
          onChange={setMetric}
          options={[
            { value: "tokens", label: "Tokens" },
            { value: "requests", label: "Requests" },
            { value: "cost", label: "Cost" },
          ]}
        />
      </div>

      <Card className="p-6 md:p-7">
        <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
          <div>
            <p className="text-xs font-medium uppercase tracking-widest text-muted-foreground">30-day total</p>
            <p className="mt-1 text-2xl font-display font-semibold tabular-nums tracking-tight text-foreground">{headline}</p>
          </div>
          {metric === "tokens" && (
            <div className="flex items-center gap-4 text-xs">
              <LegendDot color={C_INPUT} label="Input" />
              <LegendDot color={C_OUTPUT} label="Output" />
            </div>
          )}
        </div>

        <div className="h-[300px] w-full">
          <ResponsiveContainer width="100%" height="100%">
            {metric === "requests" ? (
              <BarChart data={chartData} margin={{ top: 10, right: 8, left: -18, bottom: 0 }}>
                <CartesianGrid vertical={false} stroke="var(--border)" strokeDasharray="4 4" opacity={0.6} />
                <XAxis dataKey="label" tick={axisTick} tickLine={false} axisLine={false} dy={12} minTickGap={24} />
                <YAxis tick={axisTick} tickLine={false} axisLine={false} tickFormatter={formatNumber} width={56} />
                <Tooltip cursor={{ fill: "var(--bg-subtle)", opacity: 0.5 }} content={<ChartTooltip metric={metric} />} />
                <Bar dataKey="requests" fill={C_REQ} radius={[6, 6, 0, 0]} maxBarSize={38} name="Requests" />
              </BarChart>
            ) : metric === "cost" ? (
              <AreaChart data={chartData} margin={{ top: 10, right: 8, left: -8, bottom: 0 }}>
                <defs>
                  <linearGradient id="costFill" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor={C_COST} stopOpacity={0.3} />
                    <stop offset="95%" stopColor={C_COST} stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid vertical={false} stroke="var(--border)" strokeDasharray="4 4" opacity={0.6} />
                <XAxis dataKey="label" tick={axisTick} tickLine={false} axisLine={false} dy={12} minTickGap={24} />
                <YAxis tick={axisTick} tickLine={false} axisLine={false} tickFormatter={(v) => `$${v}`} width={56} />
                <Tooltip content={<ChartTooltip metric={metric} />} />
                <Area type="monotone" dataKey="cost_usd" stroke={C_COST} strokeWidth={2.5} fill="url(#costFill)" name="Cost" />
              </AreaChart>
            ) : (
              <AreaChart data={chartData} margin={{ top: 10, right: 8, left: -18, bottom: 0 }}>
                <defs>
                  <linearGradient id="inFill" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor={C_INPUT} stopOpacity={0.28} />
                    <stop offset="95%" stopColor={C_INPUT} stopOpacity={0} />
                  </linearGradient>
                  <linearGradient id="outFill" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor={C_OUTPUT} stopOpacity={0.28} />
                    <stop offset="95%" stopColor={C_OUTPUT} stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid vertical={false} stroke="var(--border)" strokeDasharray="4 4" opacity={0.6} />
                <XAxis dataKey="label" tick={axisTick} tickLine={false} axisLine={false} dy={12} minTickGap={24} />
                <YAxis tick={axisTick} tickLine={false} axisLine={false} tickFormatter={formatTokens} width={56} />
                <Tooltip content={<ChartTooltip metric={metric} />} />
                <Area type="monotone" dataKey="prompt_tokens" stackId="1" stroke={C_INPUT} strokeWidth={2.5} fill="url(#inFill)" name="Input" />
                <Area type="monotone" dataKey="completion_tokens" stackId="1" stroke={C_OUTPUT} strokeWidth={2.5} fill="url(#outFill)" name="Output" />
              </AreaChart>
            )}
          </ResponsiveContainer>
        </div>
      </Card>
    </section>
  );
}

// ─── Token composition donut + activity highlights ────────────────────────
function InsightsSection({ d }: { d: KeyUsageData }) {
  const daily = d.daily ?? [];
  const t = aggregate(daily);
  const totalTokens = t.prompt + t.completion;
  const inputPct = totalTokens ? Math.round((t.prompt / totalTokens) * 100) : 0;
  const busiest = daily.reduce<null | NonNullable<KeyUsageData["daily"]>[number]>(
    (max, dp) => (dp.requests > (max?.requests ?? -1) ? dp : max), null,
  );
  const avgPerReq = t.requests ? Math.round(totalTokens / t.requests) : 0;
  const pieData = [
    { name: "Input", value: t.prompt, color: C_INPUT },
    { name: "Output", value: t.completion, color: C_OUTPUT },
  ];

  return (
    <section className="grid grid-cols-1 gap-5 lg:grid-cols-2">
      {/* Token composition */}
      <div className="space-y-4">
        <SectionTitle title="Token Composition" icon={<Coins size={17} />} />
        <Card className="p-6 md:p-7">
          {totalTokens > 0 ? (
            <div className="flex items-center gap-6">
              <div className="relative h-[150px] w-[150px] shrink-0">
                <ResponsiveContainer width="100%" height="100%">
                  <PieChart>
                    <Pie data={pieData} dataKey="value" nameKey="name" innerRadius={52} outerRadius={72} paddingAngle={2} stroke="none">
                      {pieData.map((entry) => <Cell key={entry.name} fill={entry.color} />)}
                    </Pie>
                    <Tooltip content={<ChartTooltip metric="tokens" simple />} />
                  </PieChart>
                </ResponsiveContainer>
                <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
                  <span className="text-lg font-display font-semibold tabular-nums text-foreground">{formatTokens(totalTokens)}</span>
                  <span className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">Tokens</span>
                </div>
              </div>
              <div className="flex-1 space-y-4">
                <CompositionRow color={C_INPUT} label="Input" value={formatTokens(t.prompt)} pct={inputPct} />
                <CompositionRow color={C_OUTPUT} label="Output" value={formatTokens(t.completion)} pct={100 - inputPct} />
                <div className="border-t border-border pt-3 text-xs text-muted-foreground">
                  Avg <strong className="font-medium text-foreground">{formatTokens(avgPerReq)}</strong> tokens per request
                </div>
              </div>
            </div>
          ) : (
            <p className="py-8 text-center text-sm text-muted-foreground">No token usage recorded yet.</p>
          )}
        </Card>
      </div>

      {/* Highlights */}
      <div className="space-y-4">
        <SectionTitle title="Highlights" icon={<Trophy size={17} />} />
        <Card className="p-6 md:p-7">
          <div className="grid grid-cols-2 gap-x-6 gap-y-6">
            <Highlight label="Busiest Day" value={busiest && busiest.requests > 0 ? busiest.date.slice(5) : "—"} sub={busiest && busiest.requests > 0 ? `${formatNumber(busiest.requests)} requests` : "no activity"} />
            <Highlight label="Models Used" value={String(d.models?.length ?? 0)} sub="in last 30 days" />
            <Highlight label="Avg / Request" value={formatTokens(avgPerReq)} sub="tokens" />
            <Highlight label="This Month" value={`$${d.current_period.cost_usd.toFixed(2)}`} sub={`${formatNumber(d.current_period.total_requests)} requests`} />
          </div>
        </Card>
      </div>
    </section>
  );
}

// ─── Per-model breakdown ──────────────────────────────────────────────────
function ModelSection({ models }: { models: NonNullable<KeyUsageData["models"]> }) {
  const sorted = useMemo(() => [...models].sort((a, b) => b.total_requests - a.total_requests), [models]);
  const totals = useMemo(() => sorted.reduce(
    (acc, m) => ({
      requests: acc.requests + m.total_requests,
      prompt: acc.prompt + m.prompt_tokens,
      completion: acc.completion + m.completion_tokens,
      cost: acc.cost + m.cost_usd,
    }),
    { requests: 0, prompt: 0, completion: 0, cost: 0 },
  ), [sorted]);

  return (
    <section className="space-y-4">
      <SectionTitle title="Model Breakdown" icon={<Layers size={17} />} count={sorted.length} />
      <Card className="overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-muted/50 border-b border-border">
              <tr className="text-[11px] uppercase tracking-wide text-muted-foreground">
                <th className="px-6 py-4 text-left font-semibold">Model</th>
                <th className="px-6 py-4 text-left font-semibold w-[26%]">Requests</th>
                <th className="px-6 py-4 text-right font-semibold">Input</th>
                <th className="px-6 py-4 text-right font-semibold">Output</th>
                <th className="px-6 py-4 text-right font-semibold">Avg / Req</th>
                <th className="px-6 py-4 text-right font-semibold">Cost</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {sorted.map((m, i) => {
                const share = totals.requests ? (m.total_requests / totals.requests) * 100 : 0;
                const avg = m.total_requests ? Math.round((m.prompt_tokens + m.completion_tokens) / m.total_requests) : 0;
                return (
                  <tr key={i} className="group transition-colors hover:bg-muted/30">
                    <td className="px-6 py-4">
                      <div className="flex items-center gap-3.5">
                        <ProviderIcon provider={m.provider} />
                        <div className="flex min-w-0 flex-col">
                          <div className="flex items-center gap-2">
                            <span className="font-semibold text-foreground text-[15px] truncate">{m.model}</span>
                            {i === 0 && totals.requests > 0 && <Badge variant="accent" title="Most used model">Top</Badge>}
                          </div>
                          <span className="text-[13px] text-muted-foreground capitalize truncate">{m.provider}</span>
                        </div>
                      </div>
                    </td>
                    <td className="px-6 py-4">
                      <div className="flex items-center gap-3">
                        <span className="tabular-nums text-foreground font-medium w-12">{formatNumber(m.total_requests)}</span>
                        <div className="h-1.5 flex-1 min-w-[48px] overflow-hidden rounded-full bg-muted">
                          <div className="h-full rounded-full bg-accent-500 transition-all" style={{ width: `${Math.max(share, 2)}%` }} />
                        </div>
                        <span className="tabular-nums text-xs text-muted-foreground w-9 text-right">{share.toFixed(0)}%</span>
                      </div>
                    </td>
                    <td className="px-6 py-4 text-right tabular-nums font-mono text-[13px] text-muted-foreground">{formatTokens(m.prompt_tokens)}</td>
                    <td className="px-6 py-4 text-right tabular-nums font-mono text-[13px] text-muted-foreground">{formatTokens(m.completion_tokens)}</td>
                    <td className="px-6 py-4 text-right tabular-nums font-mono text-[13px] text-muted-foreground">{formatTokens(avg)}</td>
                    <td className={`px-6 py-4 text-right tabular-nums font-semibold text-[15px] ${m.cost_usd > 0 ? "text-foreground" : "text-muted-foreground"}`}>${m.cost_usd.toFixed(4)}</td>
                  </tr>
                );
              })}
            </tbody>
            {sorted.length > 1 && (
              <tfoot className="border-t-2 border-border bg-muted/30">
                <tr className="text-[13px]">
                  <td className="px-6 py-4 font-semibold text-foreground">Total · {sorted.length} models</td>
                  <td className="px-6 py-4 tabular-nums font-semibold text-foreground">{formatNumber(totals.requests)}</td>
                  <td className="px-6 py-4 text-right tabular-nums font-mono text-muted-foreground">{formatTokens(totals.prompt)}</td>
                  <td className="px-6 py-4 text-right tabular-nums font-mono text-muted-foreground">{formatTokens(totals.completion)}</td>
                  <td className="px-6 py-4" />
                  <td className="px-6 py-4 text-right tabular-nums font-semibold text-foreground">${totals.cost.toFixed(4)}</td>
                </tr>
              </tfoot>
            )}
          </table>
        </div>
      </Card>
    </section>
  );
}

// ─── Playground ───────────────────────────────────────────────────────────
function PlaygroundSection({
  allowedModels, activeKey, selectedModel, setSelectedModel, testPrompt, setTestPrompt,
  testResponse, setTestResponse, isTesting, setIsTesting,
}: {
  allowedModels: string[]; activeKey: string; selectedModel: string; setSelectedModel: (v: string) => void;
  testPrompt: string; setTestPrompt: (v: string) => void; testResponse: any; setTestResponse: (v: any) => void;
  isTesting: boolean; setIsTesting: (v: boolean) => void;
}) {
  return (
    <section className="space-y-4">
      <SectionTitle title="Playground" icon={<Zap size={17} />} />
      <Card className="p-6 md:p-8">
        <div className="space-y-6">
          <div>
            <label className="mb-2 block text-xs font-semibold uppercase tracking-widest text-muted-foreground">Model</label>
            <div className="relative">
              <Select
                value={selectedModel}
                onChange={(e) => setSelectedModel(e.target.value)}
                className="h-9 rounded-lg pr-8"
              >
                {allowedModels.map((m) => <option key={m} value={m}>{m}</option>)}
              </Select>
              <ChevronDown size={18} className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
            </div>
          </div>

          <div>
            <textarea
              value={testPrompt}
              onChange={(e) => setTestPrompt(e.target.value)}
              rows={3}
              className="w-full rounded-lg border border-input bg-transparent px-3 py-2 text-sm text-foreground shadow-sm transition-colors placeholder:text-muted-foreground hover:border-[var(--border-strong)] focus:border-accent-500 focus:outline-none focus:ring-2 focus:ring-accent-500/20"
              placeholder="Enter your message..."
            />
          </div>

          <Button
            onClick={() => {
              setIsTesting(true);
              setTestResponse(null);
              fetch(`${BASE_URL}/v1/chat/completions`, {
                method: "POST",
                headers: { "Content-Type": "application/json", Authorization: `Bearer ${activeKey}` },
                body: JSON.stringify({ model: selectedModel, messages: [{ role: "user", content: testPrompt }], stream: false }),
              })
                .then((r) => r.json())
                .then((json) => {
                  if (json.error) setTestResponse({ error: json.error.message || "Request failed" });
                  else setTestResponse(json);
                })
                .catch((err) => setTestResponse({ error: err.message }))
                .finally(() => setIsTesting(false));
            }}
            disabled={!selectedModel || !testPrompt.trim() || isTesting}
            className="w-full h-12 text-base font-medium shadow-sm transition-all hover:-translate-y-px"
          >
            {isTesting ? <><Spinner /> Testing...</> : <><Send size={16} /> Send Message</>}
          </Button>

          {testResponse && (
            <div className="mt-2 space-y-3">
              <div className="flex items-center justify-between">
                <h4 className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">Response</h4>
                {testResponse.error && <Badge variant="destructive">Error</Badge>}
              </div>
              <div className="rounded-xl border border-border bg-muted p-4 text-sm">
                {testResponse.error ? (
                  <p className="font-medium text-[color:var(--color-danger)]">{testResponse.error}</p>
                ) : (
                  <div className="space-y-2">
                    <p className="whitespace-pre-wrap text-foreground">{testResponse.choices?.[0]?.message?.content || "No response content"}</p>
                    {testResponse.usage && (
                      <div className="mt-4 flex flex-wrap gap-4 border-t border-border pt-4 text-xs text-muted-foreground">
                        <span>Prompt: {testResponse.usage.prompt_tokens?.toLocaleString()}</span>
                        <span>Completion: {testResponse.usage.completion_tokens?.toLocaleString()}</span>
                        <span>Total: {testResponse.usage.total_tokens?.toLocaleString()}</span>
                      </div>
                    )}
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
      </Card>
    </section>
  );
}

// ─── Small presentational helpers ─────────────────────────────────────────

function SectionTitle({ title, icon, count }: { title: string; icon?: React.ReactNode; count?: number }) {
  return (
    <div className="flex items-center gap-2.5 pl-1">
      {icon && <div className="text-muted-foreground">{icon}</div>}
      <h2 className="text-sm font-semibold tracking-widest text-muted-foreground uppercase">{title}</h2>
      {count != null && (
        <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] font-semibold text-muted-foreground tabular-nums">{count}</span>
      )}
    </div>
  );
}

function LiveIndicator({ updatedAt }: { updatedAt?: number }) {
  const [, force] = useState(0);
  useEffect(() => {
    const t = setInterval(() => force((n) => n + 1), 15000);
    return () => clearInterval(t);
  }, []);
  const label = updatedAt ? relativeTime(updatedAt) : "live";
  return (
    <div className="hidden items-center gap-2 rounded-xl border border-border bg-popover px-3 py-2 shadow-sm sm:flex" title="Auto-refreshes every 30s">
      <span className="relative flex h-2 w-2">
        <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-accent-500 opacity-60" />
        <span className="relative inline-flex h-2 w-2 rounded-full bg-accent-500" />
      </span>
      <Radio size={13} className="text-muted-foreground" />
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
    </div>
  );
}

function KpiCard({
  icon: Icon, label, value, sub, color, accent, data, dataKey, sparkId,
}: {
  icon: any; label: string; value: string; sub: string; color: string; accent?: boolean;
  data: any[]; dataKey: string; sparkId: string;
}) {
  return (
    <div className="flex flex-col justify-between rounded-2xl border border-border bg-popover p-5 shadow-sm ring-1 ring-inset ring-white/50 dark:ring-0">
      <div className="flex items-center gap-2.5">
        <span className="flex h-8 w-8 items-center justify-center rounded-lg border border-border bg-muted/60 text-muted-foreground" style={accent ? { color, borderColor: color + "40" } : undefined}>
          <Icon size={16} strokeWidth={2} />
        </span>
        <p className="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">{label}</p>
      </div>
      <div className="mt-3">
        <p className={`text-3xl font-display font-semibold tracking-tight tabular-nums ${accent ? "text-accent-600 dark:text-accent-400" : "text-foreground"}`}>{value}</p>
        <p className="mt-1 text-xs text-muted-foreground">{sub}</p>
      </div>
      <div className="-mx-1 mt-3 h-9">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={data} margin={{ top: 2, right: 2, left: 2, bottom: 0 }}>
            <defs>
              <linearGradient id={sparkId} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={color} stopOpacity={0.35} />
                <stop offset="100%" stopColor={color} stopOpacity={0} />
              </linearGradient>
            </defs>
            <Area type="monotone" dataKey={dataKey} stroke={color} strokeWidth={1.75} fill={`url(#${sparkId})`} isAnimationActive={false} />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}

function BudgetProgress({ label, used, limit, pct, alert, remaining, format }: {
  label: string; used: number; limit: number; pct: number; alert: boolean; remaining: number; format: (v: number) => string;
}) {
  if (limit <= 0) return null;
  const safePct = Math.min(Math.max(pct, 0), 100);
  const isWarning = safePct > 80 && !alert;
  const barColor = alert
    ? "bg-[color:var(--color-danger)] shadow-[0_0_10px_var(--color-danger)]"
    : isWarning ? "bg-[color:var(--color-warning)] shadow-[0_0_10px_var(--color-warning)]" : "bg-accent-500 shadow-[0_0_10px_var(--color-accent-500)]";
  return (
    <div>
      <div className="mb-2.5 flex items-end justify-between">
        <span className="text-sm font-medium text-foreground">{label}</span>
        <div className="text-right">
          <span className="text-[17px] font-display font-semibold text-foreground tabular-nums tracking-tight">{format(used)}</span>
          <span className="ml-1.5 text-sm font-medium text-muted-foreground">/ {format(limit)}</span>
        </div>
      </div>
      <div className="h-2.5 w-full overflow-hidden rounded-full bg-muted ring-1 ring-inset ring-border">
        <div className={`h-full rounded-full ${barColor} transition-all duration-1000 ease-out`} style={{ width: `${safePct}%` }} />
      </div>
      <div className="mt-2 flex items-center justify-between text-xs text-muted-foreground">
        <span className="tabular-nums">{safePct.toFixed(1)}% used</span>
        <span className="tabular-nums">{format(remaining)} left</span>
      </div>
    </div>
  );
}

function CompositionRow({ color, label, value, pct }: { color: string; label: string; value: string; pct: number }) {
  return (
    <div>
      <div className="mb-1.5 flex items-center justify-between">
        <span className="flex items-center gap-2 text-sm font-medium text-foreground">
          <span className="h-2.5 w-2.5 rounded-sm" style={{ background: color }} />
          {label}
        </span>
        <span className="text-sm tabular-nums text-muted-foreground"><strong className="font-medium text-foreground">{value}</strong> · {pct}%</span>
      </div>
      <div className="h-2 w-full overflow-hidden rounded-full bg-muted">
        <div className="h-full rounded-full transition-all" style={{ width: `${pct}%`, background: color }} />
      </div>
    </div>
  );
}

function Highlight({ label, value, sub }: { label: string; value: string; sub: string }) {
  return (
    <div>
      <p className="text-[11px] font-semibold uppercase tracking-widest text-muted-foreground">{label}</p>
      <p className="mt-1.5 text-2xl font-display font-semibold tabular-nums tracking-tight text-foreground">{value}</p>
      <p className="mt-0.5 text-xs text-muted-foreground">{sub}</p>
    </div>
  );
}

function LegendDot({ color, label }: { color: string; label: string }) {
  return (
    <span className="flex items-center gap-1.5 text-muted-foreground">
      <span className="h-2 w-2 rounded-full" style={{ background: color }} />
      {label}
    </span>
  );
}

function ChartTooltip({ active, payload, label, metric, simple }: any) {
  if (!active || !payload?.length) return null;
  const fmt = (v: number) => (metric === "cost" ? `$${v.toFixed(4)}` : metric === "requests" ? formatNumber(v) : formatTokens(v));
  const nameOf = (key: string) => (key === "prompt_tokens" || key === "Input" ? "Input" : key === "completion_tokens" || key === "Output" ? "Output" : key === "requests" ? "Requests" : key === "cost_usd" ? "Cost" : key);
  return (
    <div className="rounded-xl border border-border bg-popover px-3.5 py-2.5 shadow-md">
      {!simple && label && <p className="mb-1.5 text-xs font-medium text-muted-foreground">{label}</p>}
      <div className="space-y-1">
        {payload.map((p: any, i: number) => (
          <div key={i} className="flex items-center gap-2 text-sm">
            <span className="h-2 w-2 rounded-full" style={{ background: p.color || p.payload?.color }} />
            <span className="text-muted-foreground">{nameOf(p.name ?? p.dataKey)}</span>
            <span className="ml-auto font-semibold tabular-nums text-foreground">{fmt(p.value)}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function ProviderIcon({ provider, className }: { provider: string; className?: string }) {
  const [errored, setErrored] = useState(false);
  const sizeClass = className || "h-10 w-10";
  if (errored) {
    return (
      <div className={`flex shrink-0 items-center justify-center rounded-xl bg-popover border border-border shadow-sm text-[11px] font-bold text-muted-foreground uppercase tracking-wider ${sizeClass}`}>
        {provider.slice(0, 2)}
      </div>
    );
  }
  return (
    <div className={`flex shrink-0 items-center justify-center rounded-xl bg-popover border border-border shadow-sm ${sizeClass} p-1.5`}>
      <img src={`/providers/${provider}.png`} alt={provider} onError={() => setErrored(true)} className="h-full w-full object-contain" />
    </div>
  );
}

// ─── Utilities ────────────────────────────────────────────────────────────

const axisTick = { fontSize: 12, fill: "var(--text-muted)", fontFamily: "var(--font-sans)" } as const;

function aggregate(daily: NonNullable<KeyUsageData["daily"]>) {
  return daily.reduce(
    (acc, dp) => ({
      requests: acc.requests + dp.requests,
      prompt: acc.prompt + dp.prompt_tokens,
      completion: acc.completion + dp.completion_tokens,
      cost: acc.cost + dp.cost_usd,
    }),
    { requests: 0, prompt: 0, completion: 0, cost: 0 },
  );
}

function formatTokens(n: number): string {
  if (n >= 1_000_000_000) return `${(n / 1_000_000_000).toFixed(1)}B`;
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return n.toLocaleString();
}

function formatNumber(n: number): string {
  return Math.round(n).toLocaleString();
}

function relativeTime(ts: number): string {
  const s = Math.max(0, Math.floor((Date.now() - ts) / 1000));
  if (s < 10) return "just now";
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  return `${Math.floor(m / 60)}h ago`;
}
