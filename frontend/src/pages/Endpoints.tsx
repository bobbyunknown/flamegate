import { Field } from "@/components/composite/native-select";
import { useEffect, useState, useRef, useCallback } from "react";
import { Link } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Copy,
  Check,
  Loader2,
  ArrowUpRight,
  KeyRound,
} from "lucide-react";
import { api, BASE_URL, type TailscaleEnableResult } from "../lib/api"
import { PageHeader } from "@/components/composite/page-header";
import { Card } from "@/components/ui/card";
import { CardHeader } from "@/components/composite/card-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";

// Polling intervals (ms).
const STATUS_POLL_FAST = 5000;
const STATUS_POLL_SLOW = 30000;
const PING_INTERVAL = 2000;
const PING_TIMEOUT = 5000;
const PING_MAX_MS = 300000;
const REACHABLE_MISS_THRESHOLD = 5;

// ---------------------------------------------------------------------------
// Brand SVG logos
// ---------------------------------------------------------------------------

function CloudflareLogo({ className = "" }: { className?: string }) {
  return (
    <svg role="img" viewBox="0 0 24 24" className={className} fill="currentColor" aria-label="Cloudflare">
      <path d="M16.5088 16.8447c.1475-.5068.0908-.9707-.1553-1.3154-.2246-.3164-.6045-.499-1.0615-.5205l-8.6592-.1123a.1559.1559 0 0 1-.1333-.0713c-.0283-.042-.0351-.0986-.021-.1553.0278-.084.1123-.1484.2036-.1562l8.7359-.1123c1.0351-.0489 2.1601-.8868 2.5537-1.9136l.499-1.3013c.0215-.0561.0293-.1128.0147-.168-.5625-2.5463-2.835-4.4453-5.5499-4.4453-2.5039 0-4.6284 1.6177-5.3876 3.8614-.4927-.3658-1.1187-.5625-1.794-.499-1.2026.119-2.1665 1.083-2.2861 2.2856-.0283.31-.0069.6128.0635.894C1.5683 13.171 0 14.7754 0 16.752c0 .1748.0142.3515.0352.5273.0141.083.0844.1475.1689.1475h15.9814c.0909 0 .1758-.0645.2032-.1553l.12-.4268zm2.7568-5.5634c-.0771 0-.1611 0-.2383.0112-.0566 0-.1054.0415-.127.0976l-.3378 1.1744c-.1475.5068-.0918.9707.1543 1.3164.2256.3164.6055.498 1.0625.5195l1.8437.1133c.0557 0 .1055.0263.1329.0703.0283.043.0351.1074.0214.1562-.0283.084-.1132.1485-.204.1553l-1.921.1123c-1.041.0488-2.1582.8867-2.5527 1.914l-.1406.3585c-.0283.0713.0215.1416.0986.1416h6.5977c.0771 0 .1474-.0489.169-.126.1122-.4082.1757-.837.1757-1.2803 0-2.6025-2.125-4.727-4.7344-4.727" />
    </svg>
  );
}

function TailscaleLogo({ className = "" }: { className?: string }) {
  return (
    <svg role="img" viewBox="0 0 24 24" className={className} fill="currentColor" aria-label="Tailscale">
      <path d="M24 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0zm-9 9a3 3 0 1 1-6 0 3 3 0 0 1 6 0zm0-9a3 3 0 1 1-6 0 3 3 0 0 1 6 0zm6-6a3 3 0 1 1 0-6 3 3 0 0 1 0 6zm0-.5a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5zM3 24a3 3 0 1 1 0-6 3 3 0 0 1 0 6zm0-.5a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5zm18 .5a3 3 0 1 1 0-6 3 3 0 0 1 0 6zm0-.5a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5zM6 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0zm9-9a3 3 0 1 1-6 0 3 3 0 0 1 6 0zm-3 2.5a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5zM6 3a3 3 0 1 1-6 0 3 3 0 0 1 6 0zM3 5.5a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5z" />
    </svg>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export function EndpointsPage() {
  return (
    <>
      <PageHeader
        title="Endpoints"
        description="How your applications connect to FlameGate."
      />
      <div className="space-y-5 sm:space-y-6">
        <PrimaryEndpoint />
        <CredentialNotice />
        <TunnelSection />
      </div>
    </>
  );
}

// ---------------------------------------------------------------------------
// Primary endpoint — hero card
// ---------------------------------------------------------------------------

// Append the /v1 API path to a tunnel base URL so it mirrors the primary
// endpoint and can be used as a drop-in replacement.
function withApiPath(base: string): string {
  const trimmed = base.replace(/\/+$/, "");
  if (!trimmed) return "";
  return trimmed.endsWith("/v1") ? trimmed : `${trimmed}/v1`;
}

function PrimaryEndpoint() {
  const access = useQuery({ queryKey: ["access-settings"], queryFn: () => api.accessSettings() });
  const status = useQuery({
    queryKey: ["tunnel-status"],
    queryFn: () => api.tunnelStatus(),
    refetchInterval: STATUS_POLL_SLOW,
  });

  const localUrl = access.data?.endpoint_url ?? "";

  const tunnel = status.data?.tunnel;
  const tunnelRunning = tunnel?.running ?? false;
  const tunnelBase = tunnel?.publicUrl || tunnel?.tunnelUrl || "";
  const tunnelUrl = tunnelRunning ? withApiPath(tunnelBase) : "";

  return (
    <Card>
      <div className="divide-y divide-border">
        <EndpointRow
          label="Primary endpoint"
          url={localUrl}
          loadingText="Loading…"
          hint="Point your applications at this URL. All providers are accessible through this single endpoint."
        />
        {tunnelUrl && (
          <EndpointRow
            label="Public endpoint · Cloudflare Tunnel"
            url={tunnelUrl}
            icon={<CloudflareLogo className="h-3.5 w-3.5 text-[#F6821F]" />}
            hint="Publicly reachable URL from your Cloudflare tunnel. Use it to reach FlameGate from anywhere."
          />
        )}
      </div>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Copyable endpoint row — shared by the primary + tunnel endpoints
// ---------------------------------------------------------------------------

function EndpointRow({
  label,
  url,
  hint,
  icon,
  loadingText,
}: {
  label: string;
  url: string;
  hint: string;
  icon?: React.ReactNode;
  loadingText?: string;
}) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    if (!url) return;
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      setTimeout(() => setCopied(false), 1800);
    } catch {
      // no-op
    }
  };

  return (
    <div className="px-4 py-4 sm:px-6 sm:py-5">
      <div className="flex items-center gap-2">
        {icon ?? (
          <span className="relative flex h-2 w-2">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-accent-400 opacity-75" style={{ animationDuration: "2s" }} />
            <span className="relative inline-flex h-2 w-2 rounded-full bg-[color:var(--color-success)]" />
          </span>
        )}
        <p className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
          {label}
        </p>
      </div>
      {/* Mobile: stack vertically. Desktop: side-by-side. */}
      <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-stretch">
        <div className="flex min-w-0 flex-1 items-center rounded-lg border border-input bg-background px-3 py-2 sm:px-4 sm:py-2.5">
          <span className="truncate font-mono text-[13px] text-foreground">
            {url || loadingText || ""}
          </span>
        </div>
        <Button
          onClick={copy}
          disabled={!url}
          variant="default"
          size="default"
        >
          {copied ? (
            <>
              <Check className="h-4 w-4" />
              <span>Copied</span>
            </>
          ) : (
            <>
              <Copy className="h-4 w-4" />
              <span>Copy</span>
            </>
          )}
        </Button>
      </div>
      <p className="mt-2.5 text-xs text-muted-foreground sm:mt-3">
        {hint}
      </p>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Credential notice — keep key management centralized
// ---------------------------------------------------------------------------

function CredentialNotice() {
  return (
    <Card className="overflow-hidden">
      <div className="flex flex-col gap-4 px-4 py-4 sm:flex-row sm:items-center sm:justify-between sm:px-6 sm:py-5">
        <div className="flex items-start gap-3">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-secondary-100 text-secondary-700 dark:bg-secondary-900/40 dark:text-secondary-200">
            <KeyRound className="h-5 w-5" />
          </div>
          <div>
            <h2 className="text-sm font-semibold tracking-tight text-foreground">Credentials live in API Keys</h2>
            <p className="mt-1 max-w-2xl text-sm leading-relaxed text-muted-foreground">
              Use this page for connection URLs and tunnels. Create, revoke, copy owner portals, and set model limits from the API Keys page.
            </p>
          </div>
        </div>
        <Button render={<Link to="/keys" />} variant="default" size="default">
          Manage keys
          <ArrowUpRight className="h-4 w-4" />
        </Button>
      </div>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Tunnel section — single card with list
// ---------------------------------------------------------------------------

function TunnelSection() {
  return (
    <Card>
      <CardHeader
        title="Tunnels"
        description="Expose FlameGate to external networks"
      />
      <div className="divide-y divide-border border-t border-border">
        <CloudflareTunnel />
        {/* Tailscale — temporarily disabled, under active development */}
        <TunnelRow
          name="Tailscale"
          description="Private network with HTTPS"
          logo={<TailscaleLogo className="h-5 w-5 text-[#2255CC]/40 dark:text-[#5990FF]/40" />}
          brandColor="bg-[#2255CC]/5 dark:bg-[#5990FF]/5"
          loading={false}
          isRunning={false}
          displayUrl=""
          reachable={null}
          actionsDisabled
          onEnable={() => {}}
          onDisable={() => {}}
          enablePending={false}
          disablePending={false}
        >
          <div className="flex items-center gap-2 rounded-lg border border-amber-300/40 bg-amber-500/5 px-3 py-2 dark:border-amber-500/20 dark:bg-amber-500/10">
            <span className="relative flex h-2 w-2">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-amber-400 opacity-75" />
              <span className="relative inline-flex h-2 w-2 rounded-full bg-amber-500" />
            </span>
            <p className="text-xs font-medium text-amber-700 dark:text-amber-400">
              Under development — this feature is being built and will be available soon.
            </p>
          </div>
        </TunnelRow>
      </div>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Cloudflare tunnel row
// ---------------------------------------------------------------------------

function CloudflareTunnel() {
  const qc = useQueryClient();
  const [loading, setLoading] = useState(false);
  const [reachable, setReachable] = useState<boolean | null>(null);
  const missRef = useRef(0);
  const pingTimerRef = useRef<ReturnType<typeof setInterval> | undefined>(undefined);
  const pollTimerRef = useRef<ReturnType<typeof setInterval> | undefined>(undefined);

  const status = useQuery({
    queryKey: ["tunnel-status"],
    queryFn: () => api.tunnelStatus(),
    refetchInterval: STATUS_POLL_SLOW,
  });

  const tunnel = status.data?.tunnel;
  const download = status.data?.download;
  const tunnelUrl = tunnel?.tunnelUrl || "";
  const publicUrl = tunnel?.publicUrl || "";
  const isRunning = tunnel?.running ?? false;
  const displayUrl = publicUrl || tunnelUrl;

  const pingTunnel = useCallback(async () => {
    const url = publicUrl || tunnelUrl;
    if (!url) return;
    try {
      const ctrl = new AbortController();
      const timer = setTimeout(() => ctrl.abort(), PING_TIMEOUT);
      const res = await fetch(`${url}/healthz`, { mode: "cors", signal: ctrl.signal });
      clearTimeout(timer);
      if (res.ok) {
        setReachable(true);
        missRef.current = 0;
      } else {
        missRef.current++;
        if (missRef.current >= REACHABLE_MISS_THRESHOLD) setReachable(false);
      }
    } catch {
      missRef.current++;
      if (missRef.current >= REACHABLE_MISS_THRESHOLD) setReachable(false);
    }
  }, [publicUrl, tunnelUrl]);

  useEffect(() => {
    if (isRunning && (publicUrl || tunnelUrl)) {
      pingTunnel();
      pingTimerRef.current = setInterval(pingTunnel, PING_INTERVAL);
      const stopAt = Date.now() + PING_MAX_MS;
      const check = setInterval(() => {
        if (Date.now() > stopAt) {
          clearInterval(pingTimerRef.current);
          clearInterval(check);
        }
      }, 10000);
      return () => {
        clearInterval(pingTimerRef.current);
        clearInterval(check);
      };
    } else {
      setReachable(null);
      missRef.current = 0;
    }
  }, [isRunning, publicUrl, tunnelUrl, pingTunnel]);

  useEffect(() => {
    if (loading) {
      pollTimerRef.current = setInterval(
        () => qc.invalidateQueries({ queryKey: ["tunnel-status"] }),
        STATUS_POLL_FAST,
      );
      return () => clearInterval(pollTimerRef.current);
    }
  }, [loading, qc]);

  const enable = useMutation({
    mutationFn: () => api.tunnelEnable(),
    onMutate: () => setLoading(true),
    onSuccess: () => {
      setLoading(false);
      qc.invalidateQueries({ queryKey: ["tunnel-status"] });
      qc.invalidateQueries({ queryKey: ["access-settings"] });
    },
    onError: () => setLoading(false),
  });

  const disable = useMutation({
    mutationFn: () => api.tunnelDisable(),
    onSuccess: () => {
      setReachable(null);
      qc.invalidateQueries({ queryKey: ["tunnel-status"] });
      qc.invalidateQueries({ queryKey: ["access-settings"] });
    },
  });

  return (
    <TunnelRow
      name="Cloudflare Tunnel"
      description="Quick tunnel — no account needed"
      logo={<CloudflareLogo className="h-5 w-5 text-[#F6821F]" />}
      brandColor="bg-[#F6821F]/10 dark:bg-[#F6821F]/15"
      isRunning={isRunning}
      reachable={reachable}
      loading={loading}
      displayUrl={displayUrl}
      subText={
        download?.downloading
          ? `Downloading cloudflared… ${download.progress}%`
          : undefined
      }
      onEnable={() => enable.mutate()}
      onDisable={() => disable.mutate()}
      enablePending={enable.isPending}
      disablePending={disable.isPending}
    />
  );
}

// ---------------------------------------------------------------------------
// Tailscale tunnel row
// ---------------------------------------------------------------------------

export function TailscaleTunnel() {
  const qc = useQueryClient();
  const [loading, setLoading] = useState(false);
  const [reachable, setReachable] = useState<boolean | null>(null);
  const [sudoPassword, setSudoPassword] = useState("");
  const [installLog, setInstallLog] = useState<string[]>([]);
  const [installing, setInstalling] = useState(false);
  const [authUrl, setAuthUrl] = useState<string | null>(null);
  const [showInstall, setShowInstall] = useState(false);
  const missRef = useRef(0);
  const pingTimerRef = useRef<ReturnType<typeof setInterval> | undefined>(undefined);

  const status = useQuery({
    queryKey: ["tunnel-status"],
    queryFn: () => api.tunnelStatus(),
    refetchInterval: STATUS_POLL_SLOW,
  });

  const tsCheck = useQuery({
    queryKey: ["tailscale-check"],
    queryFn: () => api.tailscaleCheck(),
    refetchInterval: STATUS_POLL_SLOW,
  });

  const ts = status.data?.tailscale;
  const isRunning = ts?.running ?? false;
  const isLoggedIn = ts?.loggedIn ?? false;
  const isInstalled = tsCheck.data?.installed ?? false;
  const tunnelUrl = ts?.tunnelUrl || "";

  const pingTailscale = useCallback(async () => {
    if (!tunnelUrl) return;
    try {
      const ctrl = new AbortController();
      const timer = setTimeout(() => ctrl.abort(), PING_TIMEOUT);
      const res = await fetch(`${tunnelUrl}/healthz`, { mode: "cors", signal: ctrl.signal });
      clearTimeout(timer);
      if (res.ok) {
        setReachable(true);
        missRef.current = 0;
      } else {
        missRef.current++;
        if (missRef.current >= REACHABLE_MISS_THRESHOLD) setReachable(false);
      }
    } catch {
      missRef.current++;
      if (missRef.current >= REACHABLE_MISS_THRESHOLD) setReachable(false);
    }
  }, [tunnelUrl]);

  useEffect(() => {
    if (isRunning && tunnelUrl) {
      pingTailscale();
      pingTimerRef.current = setInterval(pingTailscale, PING_INTERVAL);
      return () => clearInterval(pingTimerRef.current);
    } else {
      setReachable(null);
    }
  }, [isRunning, tunnelUrl, pingTailscale]);

  const handleInstall = async () => {
    setInstalling(true);
    setInstallLog([]);
    try {
      const res = await fetch(`${BASE_URL}/api/tunnel/tailscale-install`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ sudoPassword }),
      });
      if (!res.ok || !res.body) {
        setInstallLog((prev) => [...prev, "Failed to start install"]);
        setInstalling(false);
        return;
      }
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split("\n");
        buffer = lines.pop() || "";
        for (const line of lines) {
          if (line.startsWith("event: ")) {
            const event = line.slice(7);
            const dataLine = lines[lines.indexOf(line) + 1];
            if (dataLine?.startsWith("data: ")) {
              try {
                const data = JSON.parse(dataLine.slice(6));
                if (event === "progress") {
                  setInstallLog((prev) => [...prev, data.message]);
                } else if (event === "done") {
                  setInstallLog((prev) => [...prev, "Installation complete!"]);
                  setInstalling(false);
                  qc.invalidateQueries({ queryKey: ["tailscale-check"] });
                } else if (event === "error") {
                  setInstallLog((prev) => [...prev, `Error: ${data.error}`]);
                  setInstalling(false);
                }
              } catch { /* ignore parse errors */ }
            }
          }
        }
      }
    } catch (e) {
      setInstallLog((prev) => [...prev, `Error: ${(e as Error).message}`]);
      setInstalling(false);
    }
  };

  const enable = useMutation({
    mutationFn: () => api.tailscaleEnable(sudoPassword || undefined),
    onMutate: () => setLoading(true),
    onSuccess: (data: TailscaleEnableResult) => {
      setLoading(false);
      if (data.needsLogin && data.authUrl) {
        setAuthUrl(data.authUrl);
        window.open(data.authUrl, "_blank", "width=600,height=700");
      } else if (data.funnelNotEnabled && data.enableUrl) {
        window.open(data.enableUrl, "_blank", "width=600,height=700");
      } else if (data.success) {
        setAuthUrl(null);
        qc.invalidateQueries({ queryKey: ["tunnel-status"] });
        qc.invalidateQueries({ queryKey: ["access-settings"] });
      }
    },
    onError: () => setLoading(false),
  });

  const disable = useMutation({
    mutationFn: () => api.tailscaleDisable(),
    onSuccess: () => {
      setReachable(null);
      qc.invalidateQueries({ queryKey: ["tunnel-status"] });
      qc.invalidateQueries({ queryKey: ["access-settings"] });
    },
  });

  return (
    <TunnelRow
      name="Tailscale"
      description="Private network with HTTPS"
      logo={<TailscaleLogo className="h-5 w-5 text-[#2255CC] dark:text-[#5990FF]" />}
      brandColor="bg-[#2255CC]/10 dark:bg-[#5990FF]/15"
      isRunning={isRunning}
      reachable={reachable}
      loading={loading}
      displayUrl={tunnelUrl}
      onEnable={() => {
        if (!isInstalled) setShowInstall(true);
        else enable.mutate();
      }}
      onDisable={() => disable.mutate()}
      enablePending={enable.isPending}
      disablePending={disable.isPending}
    >
      {!isInstalled && showInstall && (
        <div className="space-y-3">
          <Field label="Sudo password (for installation)">
            <Input
              type="password"
              value={sudoPassword}
              onChange={(e) => setSudoPassword(e.target.value)}
              placeholder="Required for system install"
            />
          </Field>
          <div className="flex gap-2">
            <Button onClick={handleInstall} disabled={installing || !sudoPassword.trim()}>
              {installing ? <Loader2 className="h-4 w-4 animate-spin" /> : "Install"}
            </Button>
            <Button variant="ghost" onClick={() => setShowInstall(false)}>
              Cancel
            </Button>
          </div>
          {installLog.length > 0 && (
            <div className="max-h-36 overflow-y-auto rounded-lg bg-ink-950 p-3 font-mono text-[11px] leading-relaxed text-ink-300">
              {installLog.map((line, i) => (
                <div key={i}>{line}</div>
              ))}
            </div>
          )}
        </div>
      )}

      {isInstalled && authUrl && (
        <div className="rounded-lg border border-accent-200 bg-accent-50 px-3 py-2 dark:border-accent-800/50 dark:bg-accent-800/20">
          <p className="text-xs text-accent-700 dark:text-accent-200">
            Login required —{" "}
            <a href={authUrl} target="_blank" rel="noopener noreferrer" className="font-medium underline">
              authenticate here
            </a>
          </p>
        </div>
      )}

      {isInstalled && !isRunning && isLoggedIn && (
        <div className="max-w-sm">
          <Field label="Sudo password (optional, for TUN mode)">
            <Input
              type="password"
              value={sudoPassword}
              onChange={(e) => setSudoPassword(e.target.value)}
              placeholder="Leave empty for userspace"
            />
          </Field>
        </div>
      )}

      {isInstalled && !isLoggedIn && (
        <p className="text-xs text-muted-foreground">
          Log in to your Tailscale account to enable the funnel.
        </p>
      )}
    </TunnelRow>
  );
}

// ---------------------------------------------------------------------------
// Shared tunnel row component
// ---------------------------------------------------------------------------

function TunnelRow({
  name,
  description,
  logo,
  brandColor,
  isRunning,
  reachable,
  loading,
  displayUrl,
  subText,
  onEnable,
  onDisable,
  enablePending,
  disablePending,
  actionsDisabled,
  children,
}: {
  name: string;
  description: string;
  logo: React.ReactNode;
  brandColor: string;
  isRunning: boolean;
  reachable: boolean | null;
  loading: boolean;
  displayUrl?: string;
  subText?: string;
  onEnable: () => void;
  onDisable: () => void;
  enablePending: boolean;
  disablePending: boolean;
  actionsDisabled?: boolean;
  children?: React.ReactNode;
}) {
  return (
    <div className="flex flex-col px-4 py-4 sm:px-6 sm:py-5">
      <div className="flex items-center gap-4">
        <div className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-xl ${brandColor}`}>
          {logo}
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="text-sm font-semibold text-foreground">{name}</h3>
            {isRunning && <TunnelBadge reachable={reachable} />}
          </div>
          <p className="text-xs text-muted-foreground mt-0.5">{description}</p>
          {isRunning && displayUrl && (
            <div className="mt-1.5 flex items-center gap-1.5">
              <TunnelDot running={isRunning} reachable={reachable} />
              <a
                href={displayUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="font-mono text-xs text-muted-foreground transition-colors hover:text-foreground truncate flex items-center gap-1"
              >
                <span>{displayUrl}</span>
                <ArrowUpRight className="h-3 w-3 shrink-0" />
              </a>
            </div>
          )}
          {subText && (
            <p className="mt-1.5 text-xs text-muted-foreground">{subText}</p>
          )}
        </div>
        <div className="flex shrink-0 items-center pl-2">
          {isRunning ? (
            <Button variant="destructive" onClick={onDisable} disabled={actionsDisabled || disablePending}>
              {disablePending ? <Loader2 className="h-4 w-4 animate-spin" /> : "Disable"}
            </Button>
          ) : (
            <Button onClick={onEnable} disabled={actionsDisabled || loading || enablePending} className={actionsDisabled ? "opacity-50 cursor-not-allowed" : ""}>
              {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : "Enable"}
            </Button>
          )}
        </div>
      </div>
      {children && (
        <div className="mt-4 pt-4 border-t border-border">
          {children}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Shared tunnel UI atoms
// ---------------------------------------------------------------------------

function TunnelDot({
  running,
  reachable,
}: {
  running: boolean;
  reachable: boolean | null;
}) {
  const color = running
    ? reachable === true
      ? "bg-[color:var(--color-success)]"
      : reachable === false
        ? "bg-[color:var(--color-danger)]"
        : "bg-[color:var(--color-warning)] animate-pulse"
    : "bg-ink-300 dark:bg-ink-600";
  return <span className={`block h-2 w-2 rounded-full ${color}`} />;
}

function TunnelBadge({ reachable }: { reachable: boolean | null }) {
  const tone = reachable === true ? "success" : reachable === false ? "danger" : "neutral";
  const label = reachable === true ? "Reachable" : reachable === false ? "Unreachable" : "Checking…";
  return <Badge variant={tone}>{label}</Badge>;
}
