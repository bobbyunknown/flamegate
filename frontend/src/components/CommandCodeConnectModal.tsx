import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api } from "../lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ErrorBanner } from "@/components/composite/error-banner";
import { Modal } from "@/components/composite/modal";
import { useToast } from "./Toast";
import { Done } from "./KilocodeConnectModal";

export function CommandCodeConnectModal({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  const submit = async () => {
    if (!token.trim()) {
      setError("Please enter a token");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api.commandcodeImport(token.trim());
      setDone(true);
      qc.invalidateQueries({ queryKey: ["accounts"] });
      toast.success("Command Code connected", "Token imported successfully.");
      setTimeout(onClose, 1200);
    } catch (e) {
      setError((e as Error).message);
      toast.error("Command Code import failed", (e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={true} onClose={onClose} title="Connect Command Code" maxWidth="max-w-md">
      {done ? (
        <Done provider="Command Code" />
      ) : (
        <div className="space-y-4">
          <p className="text-sm text-muted-foreground">
            Import your Command Code token from the CLI or generate an API key
            from the studio. CLI subscriptions (Go, Pro, Max, Ultra) are supported.
          </p>

          <div className="rounded-xl border border-border bg-muted p-4">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
              Option A: CLI token
            </h3>
            <ol className="space-y-2.5">
              <li className="flex items-start gap-2.5">
                <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-accent-100 text-[10px] font-bold text-accent-700 dark:bg-accent-800/40 dark:text-accent-200">1</span>
                <span className="text-sm text-foreground">
                  Run <code className="rounded bg-popover px-1.5 py-0.5 font-mono text-xs">cmd login</code> in your terminal
                </span>
              </li>
              <li className="flex items-start gap-2.5">
                <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-accent-100 text-[10px] font-bold text-accent-700 dark:bg-accent-800/40 dark:text-accent-200">2</span>
                <span className="text-sm text-foreground">
                  Copy the token from <code className="rounded bg-popover px-1.5 py-0.5 font-mono text-xs">~/.commandcode/auth.json</code>
                </span>
              </li>
            </ol>
          </div>

          <div className="rounded-xl border border-border bg-muted p-4">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
              Option B: API key
            </h3>
            <ol className="space-y-2.5">
              <li className="flex items-start gap-2.5">
                <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-accent-100 text-[10px] font-bold text-accent-700 dark:bg-accent-800/40 dark:text-accent-200">1</span>
                <span className="text-sm text-foreground">
                  Go to{" "}
                  <a href="https://commandcode.ai/studio" target="_blank" rel="noopener noreferrer" className="text-accent-600 underline underline-offset-2">
                    commandcode.ai/studio
                  </a>
                </span>
              </li>
              <li className="flex items-start gap-2.5">
                <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-accent-100 text-[10px] font-bold text-accent-700 dark:bg-accent-800/40 dark:text-accent-200">2</span>
                <span className="text-sm text-foreground">Generate and copy an API key</span>
              </li>
            </ol>
          </div>

          <div className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-on-surface-variant">Token / API Key</span>
            <Input
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder="Paste your Command Code token or API key…"
              className="font-mono"
            />
          </div>

          {error && <ErrorBanner message={error} />}

          <Button className="w-full" onClick={submit} disabled={busy || !token.trim()}>
            {busy ? "Importing…" : "Import Token"}
          </Button>
        </div>
      )}
    </Modal>
  );
}
