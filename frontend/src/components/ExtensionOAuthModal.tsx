import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
  ExternalLink,
  RefreshCw,
  CheckCircle2,
  Copy,
  Check,
  KeyRound,
  ShieldCheck,
} from "lucide-react";
import { api } from "../lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ErrorBanner } from "@/components/composite/error-banner";
import { Modal } from "@/components/composite/modal";
import { useToast } from "./Toast";

interface ExtensionOAuthModalProps {
  open: boolean;
  slug: string;
  providerName: string;
  onClose: () => void;
  onSuccess?: () => void;
}

export function ExtensionOAuthModal({
  open,
  slug,
  providerName,
  onClose,
  onSuccess,
}: ExtensionOAuthModalProps) {
  const qc = useQueryClient();
  const toast = useToast();

  const [step, setStep] = useState<
    "idle" | "starting" | "waiting" | "exchanging" | "done" | "error"
  >("idle");
  const [authData, setAuthData] = useState<{
    authorize_url: string;
    state: string;
    redirect_uri: string;
  } | null>(null);
  const [error, setError] = useState("");
  const [elapsed, setElapsed] = useState(0);
  const [showManual, setShowManual] = useState(false);
  const [manualCode, setManualCode] = useState("");
  const [accountLabel, setAccountLabel] = useState("");
  const [copiedUrl, setCopiedUrl] = useState(false);

  const popupRef = useRef<Window | null>(null);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const startedRef = useRef(false);
  const exchangedRef = useRef(false);

  // Close on Escape.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  // Clean up timer on unmount.
  useEffect(() => {
    return () => {
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, []);

  const handleExchange = async (
    code: string,
    state?: string,
    label?: string
  ) => {
    if (exchangedRef.current) return;
    exchangedRef.current = true;

    setStep("exchanging");
    setError("");

    try {
      const res = await api.oauthExchange(slug, {
        code: code.trim(),
        redirect_uri: authData?.redirect_uri,
        state: state || authData?.state,
        label: label?.trim() || accountLabel.trim() || undefined,
      });

      if (res.ok) {
        setStep("done");
        if (timerRef.current) clearInterval(timerRef.current);
        qc.invalidateQueries({ queryKey: ["accounts"] });
        qc.invalidateQueries({ queryKey: ["extensions"] });
        toast.success(
          `${providerName} connected`,
          `Account "${res.label || providerName}" added successfully.`
        );
        onSuccess?.();
        setTimeout(onClose, 1400);
      } else {
        throw new Error("Exchange did not return success");
      }
    } catch (e) {
      exchangedRef.current = false;
      const msg = (e as Error).message || "Token exchange failed";
      setError(msg);
      setStep("error");
      toast.error(`${providerName} connection failed`, msg);
    }
  };

  const start = async () => {
    if (startedRef.current) return;
    startedRef.current = true;
    exchangedRef.current = false;
    setStep("starting");
    setError("");
    setElapsed(0);

    try {
      const data = await api.oauthStart(slug);
      setAuthData(data);
      setStep("waiting");

      // Start elapsed timer
      timerRef.current = setInterval(() => setElapsed((e) => e + 1), 1000);

      // Open popup window
      const popup = window.open(
        data.authorize_url,
        `fg_oauth_${slug}`,
        "width=600,height=720,menubar=no,toolbar=no,location=no,status=no"
      );
      popupRef.current = popup;

      if (!popup) {
        setShowManual(true);
      }
    } catch (e) {
      setError((e as Error).message);
      setStep("error");
      startedRef.current = false;
      toast.error(`Couldn't start ${providerName} authorization`, (e as Error).message);
    }
  };

  // Start automatically when modal is opened
  useEffect(() => {
    if (open && !startedRef.current) {
      start();
    }
  }, [open]);

  // Listen for OAuth callback via postMessage, BroadcastChannel, and localStorage
  useEffect(() => {
    if (!open) return;

    interface CallbackPayload {
      type: string;
      slug?: string;
      code?: string;
      state?: string;
      error?: string;
      errorDescription?: string;
    }

    const processCallback = (payload: CallbackPayload) => {
      if (payload.type !== "oauth_callback") return;
      if (payload.slug && payload.slug !== slug) return;

      if (payload.error) {
        setError(payload.errorDescription || payload.error);
        setStep("error");
        return;
      }

      if (payload.code) {
        handleExchange(payload.code, payload.state);
      }
    };

    // 1. PostMessage listener
    const onMessage = (e: MessageEvent) => {
      if (e.data && typeof e.data === "object") {
        processCallback(e.data);
      }
    };
    window.addEventListener("message", onMessage);

    // 2. BroadcastChannel listener
    let channel: BroadcastChannel | null = null;
    try {
      channel = new BroadcastChannel("oauth_callback");
      channel.onmessage = (e) => {
        if (e.data && typeof e.data === "object") {
          processCallback(e.data);
        }
      };
    } catch {
      /* BroadcastChannel unsupported in some envs */
    }

    // 3. LocalStorage storage event listener
    const onStorage = (e: StorageEvent) => {
      if (e.key === "oauth_callback" && e.newValue) {
        try {
          const data = JSON.parse(e.newValue);
          processCallback(data);
          localStorage.removeItem("oauth_callback");
        } catch {
          /* ignore parse failure */
        }
      }
    };
    window.addEventListener("storage", onStorage);

    return () => {
      window.removeEventListener("message", onMessage);
      window.removeEventListener("storage", onStorage);
      if (channel) channel.close();
    };
  }, [open, slug, authData]);

  const copyAuthorizeUrl = () => {
    if (!authData?.authorize_url) return;
    navigator.clipboard.writeText(authData.authorize_url);
    setCopiedUrl(true);
    setTimeout(() => setCopiedUrl(false), 2000);
  };

  const handleManualSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!manualCode.trim()) return;
    handleExchange(manualCode.trim(), authData?.state, accountLabel.trim());
  };

  const formatElapsed = (s: number) => {
    const m = Math.floor(s / 60);
    const sec = s % 60;
    return `${m}:${sec.toString().padStart(2, "0")}`;
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={
        <div className="flex items-center gap-2">
          <ShieldCheck className="h-5 w-5 text-primary" />
          <span>Connect {providerName}</span>
        </div>
      }
      subtitle="Authenticate using OAuth2 to securely connect your provider account"
      maxWidth="sm:max-w-md"
    >
      <div className="space-y-4 py-2">
        {step === "starting" && (
          <div className="flex flex-col items-center justify-center py-8 text-center space-y-3">
            <RefreshCw className="h-8 w-8 text-primary animate-spin" />
            <p className="text-sm font-medium">Preparing authorization session…</p>
            <p className="text-xs text-muted-foreground">
              Generating secure PKCE state and redirect parameters.
            </p>
          </div>
        )}

        {step === "waiting" && (
          <div className="space-y-4">
            <div className="rounded-lg border border-border/60 bg-muted/30 p-4 text-center space-y-3">
              <div className="relative mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
                <RefreshCw className="h-6 w-6 text-primary animate-spin" />
              </div>
              <div>
                <h4 className="text-sm font-medium text-foreground">
                  Waiting for authorization…
                </h4>
                <p className="text-xs text-muted-foreground mt-0.5">
                  Complete sign-in in the popup window. Time elapsed:{" "}
                  <span className="font-mono text-foreground font-medium">
                    {formatElapsed(elapsed)}
                  </span>
                </p>
              </div>

              <div className="flex items-center justify-center gap-2 pt-1">
                <Button
                  variant="outline"
                  size="sm"
                  className="h-8 text-xs gap-1.5"
                  onClick={() => {
                    if (authData?.authorize_url) {
                      window.open(
                        authData.authorize_url,
                        `fg_oauth_${slug}`,
                        "width=600,height=720"
                      );
                    }
                  }}
                >
                  <ExternalLink className="h-3.5 w-3.5" />
                  Re-open Login Window
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-8 text-xs gap-1.5"
                  onClick={copyAuthorizeUrl}
                >
                  {copiedUrl ? (
                    <Check className="h-3.5 w-3.5 text-green-500" />
                  ) : (
                    <Copy className="h-3.5 w-3.5" />
                  )}
                  {copiedUrl ? "Copied" : "Copy Auth URL"}
                </Button>
              </div>
            </div>

            <div className="border-t border-border/40 pt-3">
              <button
                type="button"
                className="w-full text-center text-xs text-muted-foreground hover:text-foreground transition-colors flex items-center justify-center gap-1.5"
                onClick={() => setShowManual(!showManual)}
              >
                <KeyRound className="h-3.5 w-3.5" />
                {showManual
                  ? "Hide manual code entry"
                  : "Can't sign in automatically? Paste callback URL or code"}
              </button>

              {showManual && (
                <form
                  onSubmit={handleManualSubmit}
                  className="mt-3 space-y-3 rounded-lg border border-border bg-card p-3.5 text-left text-xs"
                >
                  <p className="text-muted-foreground">
                    If the popup didn't close or you are on a remote server, copy
                    the authorization code or the full callback URL from your browser address bar:
                  </p>
                  <div>
                    <label className="font-medium text-foreground block mb-1">
                      Code or Callback URL
                    </label>
                    <Input
                      placeholder="e.g. eyJ... or http://localhost:20180/api/oauth/.../callback?code=..."
                      value={manualCode}
                      onChange={(e) => setManualCode(e.target.value)}
                      className="font-mono text-xs h-9"
                    />
                  </div>
                  <div>
                    <label className="font-medium text-foreground block mb-1">
                      Account Label (optional)
                    </label>
                    <Input
                      placeholder="e.g. Work Account, my@email.com"
                      value={accountLabel}
                      onChange={(e) => setAccountLabel(e.target.value)}
                      className="text-xs h-9"
                    />
                  </div>
                  <Button
                    type="submit"
                    size="sm"
                    className="w-full h-8 text-xs font-medium"
                    disabled={!manualCode.trim()}
                  >
                    Submit & Connect
                  </Button>
                </form>
              )}
            </div>
          </div>
        )}

        {step === "exchanging" && (
          <div className="flex flex-col items-center justify-center py-8 text-center space-y-3">
            <RefreshCw className="h-8 w-8 text-primary animate-spin" />
            <p className="text-sm font-medium">Exchanging tokens and saving account…</p>
            <p className="text-xs text-muted-foreground">
              Sealing credentials with envelope encryption into the vault.
            </p>
          </div>
        )}

        {step === "done" && (
          <div className="flex flex-col items-center justify-center py-8 text-center space-y-3">
            <div className="flex h-12 w-12 items-center justify-center rounded-full bg-green-500/15 text-green-500">
              <CheckCircle2 className="h-7 w-7" />
            </div>
            <p className="text-sm font-medium text-foreground">
              {providerName} connected successfully!
            </p>
            <p className="text-xs text-muted-foreground">
              Your account is active and ready to route completions.
            </p>
          </div>
        )}

        {step === "error" && (
          <div className="space-y-4 py-2">
            <ErrorBanner message={error || "Failed to complete authorization"} />
            <div className="flex justify-end gap-2">
              <Button variant="outline" size="sm" onClick={onClose}>
                Cancel
              </Button>
              <Button
                size="sm"
                className="gap-1.5"
                onClick={() => {
                  startedRef.current = false;
                  start();
                }}
              >
                <RefreshCw className="h-3.5 w-3.5" />
                Try Again
              </Button>
            </div>
          </div>
        )}
      </div>
    </Modal>
  );
}
