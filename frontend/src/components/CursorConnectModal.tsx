import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api } from "../lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ErrorBanner } from "@/components/composite/error-banner";
import { Modal } from "@/components/composite/modal";
import { useToast } from "./Toast";
import { Done } from "./KilocodeConnectModal";

// CursorConnectModal takes a token exported from the Cursor IDE and validates
// it via the backend /cursor/import endpoint. Cursor doesn't have a public
// OAuth flow, so users copy the token from their Cursor app settings.
export function CursorConnectModal({ onClose }: { onClose: () => void }) {
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
      setError("Please enter a token from Cursor IDE");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api.cursorImport(token.trim());
      setDone(true);
      qc.invalidateQueries({ queryKey: ["accounts"] });
      toast.success("Cursor connected", "Token imported successfully.");
      setTimeout(onClose, 1200);
    } catch (e) {
      setError((e as Error).message);
      toast.error("Cursor import failed", (e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={true} onClose={onClose} title="Connect Cursor" maxWidth="max-w-md">
      {done ? (
        <Done provider="Cursor" />
      ) : (
        <div className="space-y-4">
          <p className="text-sm text-muted-foreground">
            Paste the access token from your Cursor IDE. You can find it in
            the Cursor settings under your account section.
          </p>

          <div className="rounded-xl border border-border bg-muted p-4">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-3">
              How to get the token
            </h3>
            <ol className="space-y-2.5">
              <li className="flex items-start gap-2.5">
                <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-accent-100 text-[10px] font-bold text-accent-700 dark:bg-accent-800/40 dark:text-accent-200">1</span>
                <span className="text-sm text-foreground">Open Cursor IDE settings</span>
              </li>
              <li className="flex items-start gap-2.5">
                <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-accent-100 text-[10px] font-bold text-accent-700 dark:bg-accent-800/40 dark:text-accent-200">2</span>
                <span className="text-sm text-foreground">Navigate to your account section</span>
              </li>
              <li className="flex items-start gap-2.5">
                <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-accent-100 text-[10px] font-bold text-accent-700 dark:bg-accent-800/40 dark:text-accent-200">3</span>
                <span className="text-sm text-foreground">Copy the access token</span>
              </li>
            </ol>
          </div>

          <div className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-on-surface-variant">Access Token</span>
            <Input
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder="Paste your Cursor access token…"
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
