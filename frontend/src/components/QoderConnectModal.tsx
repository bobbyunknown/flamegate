import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ExternalLink, RefreshCw, CheckCircle2, AlertTriangle } from "lucide-react";
import { api, type DeviceCode } from "../lib/api";
import { Button } from "@/components/ui/button";
import { ErrorBanner } from "@/components/composite/error-banner";
import { Modal } from "@/components/composite/modal";
import { useToast } from "./Toast";

// QoderConnectModal implements the Qoder PKCE device-token flow:
//   1. Backend generates PKCE pair + nonce + machine_id.
//   2. We open https://qoder.com/device/selectAccounts?challenge=... in the browser.
//   3. We poll openapi.qoder.sh until the user authorizes and we get a token.
//
// The flow mirrors Kiro's device-code UX but is simpler — no method picker,
// no user code to type. The user just picks their Qoder account in the popup.
export function QoderConnectModal({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [dc, setDc] = useState<DeviceCode | null>(null);
  const [status, setStatus] = useState<"idle" | "starting" | "waiting" | "done" | "error">("idle");
  const [error, setError] = useState("");
  const [elapsed, setElapsed] = useState(0);
  const pollRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const startedRef = useRef(false);

  // Close on Escape.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  // Cleanup timers on unmount.
  useEffect(() => {
    return () => {
      if (pollRef.current) clearTimeout(pollRef.current);
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, []);

  const start = async () => {
    if (startedRef.current) return;
    startedRef.current = true;
    setStatus("starting");
    setError("");

    try {
      const res = await api.qoderDeviceStart();
      setDc(res);
      setStatus("waiting");

      // Start elapsed timer.
      timerRef.current = setInterval(() => setElapsed((e) => e + 1), 1000);

      // Begin polling.
      poll(res.device_code, res.interval);
    } catch (e) {
      setError((e as Error).message);
      setStatus("error");
      startedRef.current = false;
      toast.error("Couldn't start Qoder authorization", (e as Error).message);
    }
  };

  const poll = (deviceCode: string, interval: number) => {
    pollRef.current = setTimeout(async () => {
      try {
        const res = await api.qoderDevicePoll(deviceCode);
        if (res.status === "complete") {
          setStatus("done");
          if (timerRef.current) clearInterval(timerRef.current);
          qc.invalidateQueries({ queryKey: ["accounts"] });
          toast.success("Qoder connected", "Account added successfully.");
          setTimeout(onClose, 1400);
          return;
        }
        poll(deviceCode, res.slow_down ? interval + 5 : interval);
      } catch (e) {
        setError((e as Error).message);
        setStatus("error");
        if (timerRef.current) clearInterval(timerRef.current);
        toast.error("Qoder authorization failed", (e as Error).message);
      }
    }, Math.max(1, interval) * 1000);
  };

  const formatElapsed = (s: number) => {
    const m = Math.floor(s / 60);
    const sec = s % 60;
    return `${m}:${sec.toString().padStart(2, "0")}`;
  };

  return (
    <Modal open={true} onClose={onClose} title="Connect Qoder" maxWidth="max-w-md">
      {status === "idle" && <IdleState onStart={start} />}
      {status === "starting" && <StartingState />}
      {status === "waiting" && dc && (
        <WaitingState
          dc={dc}
          elapsed={elapsed}
          formatElapsed={formatElapsed}
        />
      )}
      {status === "done" && <DoneState />}
      {status === "error" && (
        <ErrorState error={error} onRetry={start} onClose={onClose} />
      )}
    </Modal>
  );
}

function IdleState({ onStart }: { onStart: () => void }) {
  return (
    <div className="space-y-5">
      <p className="text-sm text-muted-foreground">
        Connect your Qoder account to route requests through Qoder's API.
        No API key needed — you'll sign in with your Qoder account.
      </p>

      <div className="rounded-xl border border-border bg-muted p-4">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
          How it works
        </h3>
        <ol className="space-y-2.5">
          <Step num={1} text="We generate a secure PKCE challenge locally" />
          <Step num={2} text="A popup opens for you to pick your Qoder account" />
          <Step num={3} text="Once authorized, your token is encrypted and stored" />
        </ol>
      </div>

      <Button onClick={onStart} className="w-full">
        <ExternalLink className="h-4 w-4" />
        Open Qoder sign-in
      </Button>
    </div>
  );
}

function Step({ num, text }: { num: number; text: string }) {
  return (
    <li className="flex items-start gap-2.5">
      <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-accent-100 text-[10px] font-bold text-accent-700 dark:bg-accent-800/40 dark:text-accent-200">
        {num}
      </span>
      <span className="text-sm text-foreground">{text}</span>
    </li>
  );
}

function StartingState() {
  return (
    <div className="flex flex-col items-center gap-4 py-6">
      <div className="h-10 w-10 rounded-full border-2 border-accent-500 border-t-transparent animate-spin" />
      <p className="text-sm text-muted-foreground">Generating secure challenge…</p>
    </div>
  );
}

function WaitingState({
  dc,
  elapsed,
  formatElapsed,
}: {
  dc: DeviceCode;
  elapsed: number;
  formatElapsed: (s: number) => string;
}) {
  const verificationUrl = dc.verification_uri_complete || dc.verification_uri;

  return (
    <div className="space-y-5">
      {/* Progress indicator */}
      <div className="flex items-center justify-between rounded-xl border border-border bg-muted px-4 py-3">
        <div className="flex items-center gap-2.5">
          <div className="h-8 w-8 rounded-full border-2 border-accent-500 border-t-transparent animate-spin" />
          <div>
            <p className="text-sm font-medium text-foreground">Waiting for authorization</p>
            <p className="text-xs text-muted-foreground">Pick your account in the popup</p>
          </div>
        </div>
        <span className="font-mono text-sm tabular-nums text-muted-foreground">
          {formatElapsed(elapsed)}
        </span>
      </div>

      {/* Action button */}
      <a
        href={verificationUrl}
        target="_blank"
        rel="noopener noreferrer"
        className="block w-full rounded-xl bg-accent-600 px-3 py-2.5 text-center text-sm font-medium text-white shadow-sm transition-colors hover:bg-accent-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/60"
      >
        <span className="inline-flex items-center gap-2">
          <ExternalLink className="h-4 w-4" />
          Open Qoder account picker
        </span>
      </a>

      {/* Timeout hint */}
      <p className="text-center text-xs text-muted-foreground">
        The link expires in 5 minutes. Complete the sign-in in the other tab.
      </p>

      {/* Expired hint — uses warning semantic token */}
      {elapsed > 240 && (
        <div className="flex items-start gap-2 rounded-lg border border-[color:var(--color-warning)]/30 bg-[color:var(--color-warning)]/10 px-3 py-2">
          <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[color:var(--color-warning)]" />
          <p className="text-xs text-[color:var(--color-warning)]">
            Taking a while? Make sure the popup isn't blocked by your browser.
          </p>
        </div>
      )}
    </div>
  );
}

function DoneState() {
  return (
    <div className="flex flex-col items-center gap-3 py-6">
      <div className="flex h-12 w-12 items-center justify-center rounded-full bg-accent-100 dark:bg-accent-800/40">
        <CheckCircle2 className="h-6 w-6 text-[color:var(--color-success)]" />
      </div>
      <p className="text-sm font-medium text-foreground">Connected!</p>
      <p className="text-xs text-muted-foreground">Refreshing accounts…</p>
    </div>
  );
}

function ErrorState({
  error,
  onRetry,
  onClose,
}: {
  error: string;
  onRetry: () => void;
  onClose: () => void;
}) {
  return (
    <div className="space-y-4">
      <ErrorBanner message={error} />

      <div className="flex gap-3">
        <Button variant="ghost" className="flex-1" onClick={onRetry}>
          <RefreshCw className="h-4 w-4" />
          Retry
        </Button>
        <Button variant="ghost" className="flex-1" onClick={onClose}>
          Close
        </Button>
      </div>
    </div>
  );
}
