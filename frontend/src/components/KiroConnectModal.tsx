import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Shield, Building2, FileUp, KeyRound, ArrowLeft } from "lucide-react";
import { api, type DeviceCode } from "../lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ErrorBanner } from "@/components/composite/error-banner";
import { Modal } from "@/components/composite/modal";
import { useToast } from "./Toast";

type Method = "builder-id" | "idc" | "import" | "api-key";


// KiroConnectModal mirrors 9router's "Connect Kiro" flow: pick an auth method,
// then either run an AWS SSO OIDC device authorization (Builder ID / IAM
// Identity Center) or paste a refresh token exported from the Kiro IDE.
export function KiroConnectModal({ onClose }: { onClose: () => void }) {
  const [method, setMethod] = useState<Method | null>(null);

  // Close on Escape.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <Modal 
      open={true} 
      onClose={onClose} 
      title={
        <div className="flex items-center gap-2">
          {method && (
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={() => setMethod(null)}
              aria-label="Go back"
            >
              <ArrowLeft className="h-4 w-4" />
            </Button>
          )}
          <span>Connect Kiro</span>
        </div>
      }
      maxWidth="max-w-lg"
    >
      {!method && <MethodSelect onSelect={setMethod} />}
      {method === "builder-id" && <DeviceFlow method="builder-id" onClose={onClose} />}
      {method === "idc" && <IDCFlow onClose={onClose} />}
      {method === "import" && <ImportFlow onClose={onClose} />}
      {method === "api-key" && <APIKeyFlow onClose={onClose} />}
    </Modal>
  );
}

function MethodSelect({ onSelect }: { onSelect: (m: Method) => void }) {
  return (
    <div className="space-y-3 px-6 py-5">
      <p className="text-sm text-muted-foreground">Choose your authentication method:</p>

      <MethodCard
        icon={Shield}
        title="AWS Builder ID"
        description="Recommended for most users. Free AWS account required."
        onClick={() => onSelect("builder-id")}
      />
      <MethodCard
        icon={Building2}
        title="AWS IAM Identity Center"
        description="For enterprise users with custom AWS IAM Identity Center."
        onClick={() => onSelect("idc")}
      />
      <MethodCard
        icon={FileUp}
        title="Import Token"
        description="Paste refresh token from Kiro IDE."
        onClick={() => onSelect("import")}
      />
      <MethodCard
        icon={KeyRound}
        title="API Key"
        description="Paste a headless CodeWhisperer API key. No refresh required."
        onClick={() => onSelect("api-key")}
      />
    </div>
  );
}


function MethodCard({
  icon: Icon,
  title,
  description,
  onClick,
}: {
  icon: typeof Shield;
  title: string;
  description: string;
  onClick: () => void;
}) {
  return (
    <Button
      variant="ghost"
      onClick={onClick}
      className="flex w-full h-auto items-start gap-3 justify-start rounded-xl border border-border p-4 text-left font-normal hover:bg-ink-50 dark:hover:bg-ink-800"
    >
      <span className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-accent-100 text-accent-700 dark:bg-accent-800/40 dark:text-accent-200">
        <Icon className="h-[18px] w-[18px]" />
      </span>
      <div className="min-w-0">
        <h3 className="text-sm font-semibold">{title}</h3>
        <p className="mt-0.5 text-sm text-muted-foreground">{description}</p>
      </div>
    </Button>
  );
}

// IDCFlow collects the IAM Identity Center start URL + region, then runs the
// device flow against that organization's SSO.
function IDCFlow({ onClose }: { onClose: () => void }) {
  const [startUrl, setStartUrl] = useState("");
  const [region, setRegion] = useState("us-east-1");
  const [confirmed, setConfirmed] = useState(false);
  const [error, setError] = useState("");

  if (confirmed) {
    return <DeviceFlow method="idc" startUrl={startUrl.trim()} region={region.trim()} onClose={onClose} />;
  }

  return (
    <div className="space-y-4 px-6 py-5">
      <div className="flex flex-col gap-1.5"><span className="text-xs font-medium text-on-surface-variant">IDC Start URL</span>
        <Input
          value={startUrl}
          onChange={(e) => setStartUrl(e.target.value)}
          placeholder="https://your-org.awsapps.com/start"
          className="font-mono"
        />
      </div>
      <div className="flex flex-col gap-1.5"><span className="text-xs font-medium text-on-surface-variant">AWS Region</span>
        <Input
          value={region}
          onChange={(e) => setRegion(e.target.value)}
          placeholder="us-east-1"
          className="font-mono"
        />
      </div>
      {error && <ErrorBanner message={error} />}
      <Button
        className="w-full"
        onClick={() => {
          if (!startUrl.trim()) {
            setError("Please enter your IDC start URL");
            return;
          }
          setError("");
          setConfirmed(true);
        }}
      >
        Continue
      </Button>
    </div>
  );
}

// DeviceFlow registers an SSO OIDC client, starts device authorization, shows
// the user code + verification link, and polls until the account is created.
function DeviceFlow({
  method,
  startUrl,
  region,
  onClose,
}: {
  method: "builder-id" | "idc";
  startUrl?: string;
  region?: string;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const toast = useToast();
  const [dc, setDc] = useState<DeviceCode | null>(null);
  const [status, setStatus] = useState<"starting" | "waiting" | "done" | "error">("starting");
  const [error, setError] = useState("");
  const pollRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const startedRef = useRef(false);

  useEffect(() => {
    if (startedRef.current) return;
    startedRef.current = true;

    const start = async () => {
      try {
        const res = await api.kiroDeviceStart({ method, start_url: startUrl, region });
        setDc(res);
        setStatus("waiting");
        poll(res.device_code, res.interval);
      } catch (e) {
        setError((e as Error).message);
        setStatus("error");
        toast.error("Couldn't start Kiro authorization", (e as Error).message);
      }
    };

    const poll = (deviceCode: string, interval: number) => {
      pollRef.current = setTimeout(async () => {
        try {
          const res = await api.kiroDevicePoll(deviceCode);
          if (res.status === "complete") {
            setStatus("done");
            qc.invalidateQueries({ queryKey: ["accounts"] });
            toast.success("Kiro connected", "Account added successfully.");
            setTimeout(onClose, 1200);
            return;
          }
          poll(deviceCode, res.slow_down ? interval + 5 : interval);
        } catch (e) {
          setError((e as Error).message);
          setStatus("error");
          toast.error("Kiro authorization failed", (e as Error).message);
        }
      }, Math.max(1, interval) * 1000);
    };

    start();
    return () => {
      if (pollRef.current) clearTimeout(pollRef.current);
    };
  }, [method, startUrl, region, qc, onClose, toast]);

  if (status === "done") {
    return <div className="px-6 py-6 text-sm">Connected. Refreshing accounts…</div>;
  }
  if (status === "error") {
    return (
      <div className="space-y-3 px-6 py-6">
        <ErrorBanner message={error} />
        <Button variant="ghost" className="w-full" onClick={onClose}>
          Close
        </Button>
      </div>
    );
  }
  if (status === "starting" || !dc) {
    return (
      <div className="px-6 py-10 text-center text-sm text-muted-foreground">
        Starting authorization…
      </div>
    );
  }

  return (
    <div className="space-y-4 px-6 py-5">
      <div className="rounded-xl border border-border bg-muted px-4 py-4 text-center">
        <p className="text-xs text-muted-foreground">Your code</p>
        <p className="mt-1 font-mono text-2xl font-bold tracking-widest">{dc.user_code}</p>
      </div>
      <a
        href={dc.verification_uri_complete || dc.verification_uri}
        target="_blank"
        rel="noopener noreferrer"
        className="block w-full rounded-xl bg-accent-600 px-3 py-2 text-center text-sm font-medium text-white shadow-sm transition-colors hover:bg-accent-700"
      >
        Open verification page
      </a>
      <p className="text-center text-xs text-muted-foreground">
        Enter the code on the AWS page, then approve. Waiting for authorization…
      </p>
    </div>
  );
}

// APIKeyFlow takes a long-lived CodeWhisperer API key, validates it (by
// resolving its profile upstream), and stores it as a headless connection.
// Unlike the OAuth flows there is no refresh token; the key is used as-is.
function APIKeyFlow({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [apiKey, setApiKey] = useState("");
  const [region, setRegion] = useState("us-east-1");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);

  const submit = async () => {
    if (!apiKey.trim()) {
      setError("Please enter an API key");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api.kiroAPIKey(apiKey.trim(), region.trim() || undefined);
      setDone(true);
      qc.invalidateQueries({ queryKey: ["accounts"] });
      toast.success("Kiro connected", "API key added successfully.");
      setTimeout(onClose, 1200);
    } catch (e) {
      setError((e as Error).message);
      toast.error("API key validation failed", (e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  if (done) {
    return <div className="px-6 py-6 text-sm">Connected. Refreshing accounts…</div>;
  }

  return (
    <div className="space-y-4 px-6 py-5">
      <p className="text-sm text-muted-foreground">
        Paste a headless CodeWhisperer API key. It is validated against your
        AWS profile and used directly, with no refresh.
      </p>
      <div className="flex flex-col gap-1.5"><span className="text-xs font-medium text-on-surface-variant">API Key</span>
        <Input
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
          placeholder="Your CodeWhisperer API key"
          className="font-mono"
        />
      </div>
      <div className="flex flex-col gap-1.5"><span className="text-xs font-medium text-on-surface-variant">AWS Region</span>
        <Input
          value={region}
          onChange={(e) => setRegion(e.target.value)}
          placeholder="us-east-1"
          className="font-mono"
        />
      </div>
      {error && <ErrorBanner message={error} />}
      <Button className="w-full" onClick={submit} disabled={busy || !apiKey.trim()}>
        {busy ? "Validating…" : "Connect with API Key"}
      </Button>
    </div>
  );
}

// ImportFlow takes a refresh token exported from the Kiro IDE and validates it.
function ImportFlow({ onClose }: { onClose: () => void }) {

  const qc = useQueryClient();
  const toast = useToast();
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);

  const submit = async () => {
    if (!token.trim()) {
      setError("Please enter a refresh token");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api.kiroImport(token.trim());
      setDone(true);
      qc.invalidateQueries({ queryKey: ["accounts"] });
      toast.success("Kiro connected", "Token imported successfully.");
      setTimeout(onClose, 1200);
    } catch (e) {
      setError((e as Error).message);
      toast.error("Import failed", (e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  if (done) {
    return <div className="px-6 py-6 text-sm">Connected. Refreshing accounts…</div>;
  }

  return (
    <div className="space-y-4 px-6 py-5">
      <p className="text-sm text-muted-foreground">
        Paste the refresh token from the Kiro IDE. It usually starts with
        <span className="font-mono"> aorAAAAAG…</span>
      </p>
      <div className="flex flex-col gap-1.5"><span className="text-xs font-medium text-on-surface-variant">Refresh Token</span>
        <Input
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder="aorAAAAAG..."
          className="font-mono"
        />
      </div>
      {error && <ErrorBanner message={error} />}
      <Button className="w-full" onClick={submit} disabled={busy || !token.trim()}>
        {busy ? "Importing…" : "Import Token"}
      </Button>
    </div>
  );
}
