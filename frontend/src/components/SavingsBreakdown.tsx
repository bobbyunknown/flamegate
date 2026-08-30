import { useState } from "react";
import { Scissors, Coins, FileText, Gauge, Sparkles, Layers } from "lucide-react";
import type { ClientSaving, TokenSavings, UsageInsights } from "../lib/api";
import { SavingsCardShareButton } from "./SavingsCard";

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

function fmtUSD(n: number): string {
  if (n > 0 && n < 0.01) return "<$0.01";
  return `$${n.toFixed(2)}`;
}

// prettyClient turns an internal client label into a readable name. Generic
// labels (any client detected from a User-Agent) pass through title-cased.
function prettyClient(id: string): string {
  if (!id || id === "unknown") return "Unknown";
  const known: Record<string, string> = {
    "claude-code": "Claude Code",
    "kilo-code": "Kilo Code",
    "roo-code": "Roo Code",
    cursor: "Cursor",
    codex: "Codex",
    cline: "Cline",
    copilot: "Copilot",
    opencode: "OpenCode",
    droid: "Droid",
    aider: "Aider",
  };
  if (known[id]) return known[id];
  return id
    .replace(/[-_.]+/g, " ")
    .replace(/\bsdk\b/i, "SDK")
    .split(" ")
    .map((w) => (w ? w[0].toUpperCase() + w.slice(1) : w))
    .join(" ");
}

// clientIcon maps a client id to an icon asset in /providers. Falls back to
// undefined so the avatar renders initials instead.
function clientIcon(id: string): string | undefined {
  const map: Record<string, string> = {
    "claude-code": "claude",
    "kilo-code": "kilocode",
    "roo-code": "roo",
    cursor: "cursor",
    codex: "codex",
    cline: "cline",
    copilot: "copilot",
    opencode: "opencode",
    droid: "droid",
    kiro: "kiro",
    qoder: "qoder",
    commandcode: "commandcode",
  };
  const file = map[id];
  return file ? `/providers/${file}.png` : undefined;
}

function ClientAvatar({ id, className = "h-6 w-6" }: { id: string; className?: string }) {
  const [errored, setErrored] = useState(false);
  const src = clientIcon(id);
  if (!src || errored) {
    const name = prettyClient(id);
    const initials = name.split(" ").slice(0, 2).map((w) => w[0]).join("").toUpperCase();
    return (
      <span className={`flex shrink-0 items-center justify-center rounded-md bg-muted text-[9px] font-bold text-muted-foreground ring-1 ring-border ${className}`}>
        {initials}
      </span>
    );
  }
  return (
    <span className={`flex shrink-0 items-center justify-center rounded-md bg-white p-0.5 ring-1 ring-black/5 dark:bg-black/20 dark:ring-white/10 ${className}`}>
      <img src={src} alt={prettyClient(id)} className="h-full w-full rounded object-contain" onError={() => setErrored(true)} />
    </span>
  );
}

// StatTile renders a hero metric with a soft colored accent.
function StatTile({ label, value, unit, icon: Icon, tint }: {
  label: string; value: string; unit?: string; icon: any; tint: string;
}) {
  return (
    <div className="relative flex flex-col gap-2 overflow-hidden rounded-xl border border-border bg-popover p-3.5">
      <div className="absolute inset-0 opacity-[0.07]" style={{ background: `radial-gradient(120% 120% at 100% 0%, ${tint} 0%, transparent 60%)` }} />
      <div className="relative flex items-center gap-1.5">
        <Icon className="h-3.5 w-3.5" style={{ color: tint }} />
        <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{label}</span>
      </div>
      <div className="relative flex items-baseline gap-1">
        <span className="text-xl font-light tabular-nums leading-none text-foreground">{value}</span>
        {unit && <span className="text-[11px] font-medium text-muted-foreground">{unit}</span>}
      </div>
    </div>
  );
}

export function TokenSavingsBreakdown({ savings, totalRequests, insights, period }: { savings: TokenSavings; totalRequests: number; insights: UsageInsights; period: string }) {
  const rules = (savings.rules || []).slice().sort((a, b) => b.bytes_saved - a.bytes_saved);
  const maxBytes = Math.max(...rules.map((r) => r.bytes_saved), 1);
  const totalCavemanPct = totalRequests > 0 ? ((savings.caveman_requests / totalRequests) * 100).toFixed(1) : "0";
  const totalTersePct = totalRequests > 0 ? ((savings.terse_requests / totalRequests) * 100).toFixed(0) : "0";
  // New savers. Old payloads predate these fields, so coalesce missing to 0.
  const headroomTokensSaved = savings.headroom_tokens_saved ?? 0;
  const ponytailRequests = savings.ponytail_requests ?? 0;
  const totalPonytailPct = totalRequests > 0 ? ((ponytailRequests / totalRequests) * 100).toFixed(0) : "0";
  const hasSavings = savings.slim_bytes_saved > 0 || savings.caveman_requests > 0 || savings.terse_requests > 0 || headroomTokensSaved > 0 || ponytailRequests > 0 || rules.length > 0;
  // Prefer the backend's blended USD estimate; fall back to a rough $3/M rate
  // for older payloads that predate the usd_saved field.
  const usdSaved = savings.usd_saved ?? (savings.slim_tokens_saved / 1_000_000) * 3;
  const optimizedRequests = savings.caveman_requests + savings.terse_requests;

  const badges: { label: string; pct: string; color: string }[] = [];
  if (savings.caveman_requests > 0) badges.push({ label: "CVMN", pct: totalCavemanPct, color: "#a855f7" });
  if (savings.terse_requests > 0) badges.push({ label: "TRSE", pct: totalTersePct, color: "#6366f1" });
  if (ponytailRequests > 0) badges.push({ label: "PONY", pct: totalPonytailPct, color: "#14b8a6" });

  return (
    <div className="rounded-xl border border-border bg-card shadow-xs overflow-hidden">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border px-5 py-3 bg-muted/40">
        <div className="flex items-center gap-2">
          <span className="flex h-6 w-6 items-center justify-center rounded-lg bg-primary/10 text-primary">
            <Scissors className="h-3.5 w-3.5" />
          </span>
          <h3 className="text-sm font-semibold tracking-tight">Optimization Engine</h3>
        </div>
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2.5 text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
            {badges.map((b) => (
              <span key={b.label} className="flex items-center gap-1.5">
                <span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: b.color }} />
                {b.label} {b.pct}%
              </span>
            ))}
          </div>
          <SavingsCardShareButton insights={insights} period={period} />
        </div>
      </div>

      <div className="p-5">
        {/* Hero stat tiles */}
        <div className="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
          <StatTile label="Total Saved" value={fmtBytes(savings.slim_bytes_saved)} icon={Sparkles} tint="#10b981" />
          <StatTile label="Tokens Saved" value={fmtNum(savings.slim_tokens_saved)} unit="tok" icon={Coins} tint="#f59e0b" />
          <StatTile label="Est. Value" value={fmtUSD(usdSaved)} unit="est" icon={Gauge} tint="#0ea5e9" />
          <StatTile label="Optimized" value={fmtNum(optimizedRequests)} unit="req" icon={Layers} tint="#a855f7" />
        </div>

        {/* Rules */}
        <div>
          <div className="mb-3 flex items-center gap-1.5 text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
            <FileText className="h-3 w-3" /> Compression Rules
          </div>
          {rules.length === 0 ? (
            <div className="flex items-center justify-center rounded-lg border border-dashed border-border bg-muted/40 py-6 text-xs font-medium text-muted-foreground">
              {!hasSavings
                ? "No optimizations active for this period"
                : "Output optimizations active (no prompt savings yet)"}
            </div>
          ) : (
            <div className="space-y-2.5">
              {rules.map((r, i) => (
                <div key={r.rule} className="group flex items-center gap-3">
                  <div className="flex w-36 shrink-0 items-center gap-2">
                    <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-md bg-muted text-[10px] font-bold tabular-nums text-muted-foreground">{i + 1}</span>
                    <span className="truncate font-mono text-xs font-medium text-foreground" title={r.rule}>{r.rule}</span>
                  </div>
                  <div className="flex-1">
                    <div className="h-2 w-full overflow-hidden rounded-full bg-muted">
                      <div
                        className="h-full rounded-full transition-all"
                        style={{
                          width: `${Math.max(3, (r.bytes_saved / maxBytes) * 100)}%`,
                          background: "linear-gradient(90deg, var(--color-accent-500), #10b981)",
                        }}
                      />
                    </div>
                  </div>
                  <div className="w-20 text-right text-xs font-semibold tabular-nums text-foreground">{fmtBytes(r.bytes_saved)}</div>
                  <div className="hidden w-16 text-right text-[10px] font-medium tabular-nums uppercase text-muted-foreground sm:block">{fmtNum(r.tokens_saved)} tok</div>
                  <div className="w-10 text-right text-[10px] font-medium tabular-nums text-muted-foreground">{r.count}×</div>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Headroom / Ponytail chips */}
        {(headroomTokensSaved > 0 || ponytailRequests > 0) && (
          <div className="mt-5 flex flex-wrap gap-2">
            {headroomTokensSaved > 0 && (
              <div className="flex items-center gap-2 rounded-lg border border-border bg-muted/50 px-3 py-2">
                <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">Headroom</span>
                <span className="text-sm font-medium tabular-nums text-foreground">{fmtNum(headroomTokensSaved)}</span>
                <span className="text-[10px] text-muted-foreground">tokens</span>
              </div>
            )}
            {ponytailRequests > 0 && (
              <div className="flex items-center gap-2 rounded-lg border border-border bg-muted/50 px-3 py-2">
                <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">Ponytail</span>
                <span className="text-sm font-medium tabular-nums text-foreground">{fmtNum(ponytailRequests)}</span>
                <span className="text-[10px] text-muted-foreground">requests</span>
              </div>
            )}
          </div>
        )}

        <ClientBreakdown clients={savings.by_client || []} />
      </div>
    </div>
  );
}

// ClientBreakdown shows which clients benefited from optimization, attributing
// token and estimated dollar savings to each. Generic across any client — it
// renders whatever the backend reports, never locked to specific tools.
function ClientBreakdown({ clients }: { clients: ClientSaving[] }) {
  if (clients.length === 0) return null;
  const sorted = clients.slice().sort((a, b) => b.tokens_saved - a.tokens_saved);
  const maxTokens = Math.max(...sorted.map((c) => c.tokens_saved), 1);
  return (
    <div className="mt-6 border-t border-border pt-5">
      <div className="mb-3 text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
        Savings by Client
      </div>
      <div className="grid gap-2.5 sm:grid-cols-2">
        {sorted.map((c) => (
          <div key={c.client} className="flex items-center gap-3 rounded-xl border border-border bg-popover px-3 py-2.5">
            <ClientAvatar id={c.client} className="h-8 w-8" />
            <div className="min-w-0 flex-1">
              <div className="flex items-center justify-between gap-2">
                <span className="truncate text-xs font-semibold text-foreground" title={prettyClient(c.client)}>{prettyClient(c.client)}</span>
                <span className="shrink-0 text-xs font-semibold tabular-nums text-accent-600 dark:text-accent-400">{fmtUSD(c.usd_saved)}</span>
              </div>
              <div className="mt-1.5 h-1.5 w-full overflow-hidden rounded-full bg-muted">
                <div
                  className="h-full rounded-full"
                  style={{ width: `${Math.max(3, (c.tokens_saved / maxTokens) * 100)}%`, background: "linear-gradient(90deg, var(--color-accent-500), #10b981)" }}
                />
              </div>
              <div className="mt-1 flex items-center justify-between text-[10px] font-medium tabular-nums text-muted-foreground">
                <span>{fmtNum(c.tokens_saved)} tok saved</span>
                <span>{c.requests}× requests</span>
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
