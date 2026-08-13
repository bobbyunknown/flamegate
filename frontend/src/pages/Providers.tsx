import { useMemo, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Boxes, Search, X, AlertTriangle, Plus } from "lucide-react";
import { api, type Provider, type Account } from "../lib/api";
import { PageHeader } from "@/components/composite/page-header";
import { Card } from "@/components/ui/card";
import { CardHeader } from "@/components/composite/card-header";
import { Badge } from "@/components/ui/badge";
import { Spinner } from "@/components/ui/spinner";
import { EmptyState } from "@/components/composite/empty-state";
import { StatusDot } from "@/components/composite/status-dot";
import { Button } from "@/components/ui/button";
import { Modal } from "@/components/composite/modal";
import { Input } from "@/components/ui/input";
import { ErrorBanner } from "@/components/composite/error-banner";
import { Field } from "@/components/composite/native-select";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useToast } from "../components/Toast";


// Popularity ranking for default sort (lower = higher). Extensions fall to DEFAULT_RANK.
const POPULARITY: Record<string, number> = {
  openai: 1,
  anthropic: 2,
  gemini: 3,
  "custom-openai": 10,
  "custom-anthropic": 11,
};
const DEFAULT_RANK = 999;

function sortByPopularity<T extends { id: string; pinned?: boolean }>(list: T[]): T[] {
  return [...list].sort((a, b) => {
    if (a.pinned && !b.pinned) return -1;
    if (!a.pinned && b.pinned) return 1;
    const ra = POPULARITY[a.id] ?? DEFAULT_RANK;
    const rb = POPULARITY[b.id] ?? DEFAULT_RANK;
    return ra - rb;
  });
}

// Only show kind tabs that still have catalog entries post-purge.
const kindFilters = [
  { id: "all", label: "All" },
  { id: "llm", label: "Chat" },
];

export function ProvidersPage() {
  const providers = useQuery({ queryKey: ["providers"], queryFn: () => api.providers() });
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: () => api.listAccounts() });
  const [filter, setFilter] = useState("all");
  const [searchQuery, setSearchQuery] = useState("");
  const [customOpen, setCustomOpen] = useState(false);


  // Count accounts per provider id so we can split connected vs available.
  const accountsByProvider = useMemo(() => {
    const map = new Map<string, Account[]>();
    for (const a of accounts.data?.accounts ?? []) {
      const list = map.get(a.provider) ?? [];
      list.push(a);
      map.set(a.provider, list);
    }
    return map;
  }, [accounts.data]);

  const visible = useMemo(() => {
    const all = providers.data?.providers ?? [];
    return all
      .filter((p) => !p.hidden)
      // Templates custom-openai / custom-anthropic are created via "New custom provider"
      // (dynamic custom-openai-* instances). Hide the bare templates from the grid.
      .filter((p) => p.id !== "custom-openai" && p.id !== "custom-anthropic")
      .filter((p) => filter === "all" || p.service_kinds.includes(filter))
      .filter((p) => {
        if (!searchQuery.trim()) return true;
        const q = searchQuery.toLowerCase();
        return (
          p.display_name.toLowerCase().includes(q) ||
          p.id.toLowerCase().includes(q) ||
          p.alias.toLowerCase().includes(q)
        );
      });
  }, [providers.data, filter, searchQuery]);

  const connected = sortByPopularity(visible.filter((p) => accountsByProvider.has(p.id)));
  const available = sortByPopularity(visible.filter((p) => !accountsByProvider.has(p.id)));

  return (
    <>
      <PageHeader
        title="Providers"
        icon={Boxes}
        description="Connect and manage AI providers to power your routing."
      />

      <div className="mb-5 flex flex-col gap-3 sm:flex-row sm:items-center">
        <div className="flex flex-wrap gap-1.5">
          {kindFilters.map((k) => (
            <Button
              key={k.id}
              variant={filter === k.id ? "default" : "ghost"}
              size="sm"
              onClick={() => setFilter(k.id)}
              aria-pressed={filter === k.id}
              className={filter === k.id ? "" : "text-muted-foreground"}
            >
              {k.label}
            </Button>
          ))}
        </div>
        <div className="relative max-w-sm flex-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search providers…"
            className="h-9 pl-9 pr-9"
          />
          {searchQuery && (
            <Button
              variant="ghost"
              size="icon-sm"
              className="absolute right-2 top-1/2 -translate-y-1/2"
              aria-label="Clear search"
              onClick={() => setSearchQuery("")}
            >
              <X className="h-4 w-4" />
            </Button>
          )}
        </div>
        <Button variant="outline" className="h-9 shrink-0 px-3 text-xs" onClick={() => setCustomOpen(true)}>
          <Plus className="h-3.5 w-3.5" />
          New custom provider
        </Button>
      </div>


      {providers.isLoading ? (
        <Spinner />
      ) : (
        <div className="space-y-6">
          <Card>
            <CardHeader
              title="Connected providers"
              description="These providers have accounts and are ready to use."
              action={<Badge variant="default">{connected.length}</Badge>}
            />
            {!connected.length ? (
              <EmptyState
                title={searchQuery ? `No connected providers match "${searchQuery}"` : "No connected providers yet"}
                hint={searchQuery ? "Try a different search term." : "Pick a provider below to add your first account."}
              />
            ) : (
              <div className="grid grid-cols-2 gap-px overflow-hidden rounded-b-2xl bg-border sm:grid-cols-3 lg:grid-cols-4">
                {connected.map((p) => (
                  <ProviderCard
                    key={p.id}
                    provider={p}
                    accountCount={accountsByProvider.get(p.id)?.length ?? 0}
                  />
                ))}
              </div>
            )}
          </Card>

          <Card>
            <CardHeader
              title="Available providers"
              description="Add new providers to expand your routing options."
              action={<Badge variant="secondary">{available.length}</Badge>}
            />
            {!available.length ? (
              <EmptyState
                title={searchQuery ? `No providers match "${searchQuery}"` : "No providers for this capability"}
              />
            ) : (
              <div className="grid grid-cols-2 gap-px overflow-hidden rounded-b-2xl bg-border sm:grid-cols-3 lg:grid-cols-4">
                {available.map((p) => (
                  <ProviderCard key={p.id} provider={p} accountCount={0} />
                ))}
              </div>
            )}
          </Card>
        </div>
      )}

      <CreateCustomProviderModal open={customOpen} onClose={() => setCustomOpen(false)} />
    </>
  );
}

// CreateCustomProviderModal creates a new dynamic custom provider instance.
// Each instance gets a unique id so multiple OpenAI-/Anthropic-compatible
// endpoints stay fully isolated (own base URL, accounts, and models).
function CreateCustomProviderModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const toast = useToast();
  const [name, setName] = useState("");
  const [dialect, setDialect] = useState("openai");
  const [baseURL, setBaseURL] = useState("");
  const [error, setError] = useState("");

  const reset = () => {
    setName("");
    setDialect("openai");
    setBaseURL("");
    setError("");
  };

  const create = useMutation({
    mutationFn: () =>
      api.createCustomProvider({ display_name: name.trim(), dialect, base_url: baseURL.trim() }),
    onSuccess: (p) => {
      qc.invalidateQueries({ queryKey: ["providers"] });
      toast.success("Custom provider created", "Add an account and models to start routing.");
      reset();
      onClose();
      navigate({ to: `/providers/${p.id}` });
    },
    onError: (e: Error) => setError(e.message),
  });

  const canSubmit = name.trim().length > 0 && baseURL.trim().length > 0 && !create.isPending;

  return (
    <Modal
      open={open}
      onClose={() => { reset(); onClose(); }}
      title="New custom provider"
      subtitle="A dedicated instance of an OpenAI- or Anthropic-compatible endpoint. Each instance is isolated with its own base URL, accounts, and models."
    >
      <form
        className="space-y-4 px-6 py-5"
        onSubmit={(e) => {
          e.preventDefault();
          if (canSubmit) create.mutate();
        }}
      >
        <Field label="Name (required)">
          <Input
            value={name}
            onChange={(e) => { setName(e.target.value); setError(""); }}
            placeholder="e.g. Local vLLM or Acme Gateway"
            autoFocus
          />
        </Field>
        <Field label="Dialect">
          <Select value={dialect} onValueChange={setDialect}>
            <SelectTrigger className="h-9 w-full border-border bg-background text-xs">
              <SelectValue placeholder="Select dialect" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="openai">OpenAI-compatible</SelectItem>
              <SelectItem value="anthropic">Anthropic-compatible</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        <Field label="Base URL (required)">
          <Input
            value={baseURL}
            onChange={(e) => { setBaseURL(e.target.value); setError(""); }}
            placeholder="https://llm.example.com/v1"
          />
        </Field>
        <p className="text-xs text-muted-foreground">
          Tip: add two separate instances for two endpoints of the same type — they will never share models or credentials.
        </p>
        {error && <ErrorBanner message={error} />}
        <div className="flex items-center justify-end gap-2 pt-1">
          <Button type="button" variant="ghost" onClick={() => { reset(); onClose(); }}>
            Cancel
          </Button>
          <Button type="submit" disabled={!canSubmit}>
            <Plus className="h-4 w-4" />
            {create.isPending ? "Creating…" : "Create provider"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function ProviderCard({ provider: p, accountCount }: { provider: Provider; accountCount: number }) {

  const navigate = useNavigate();
  const connected = accountCount > 0;

  return (
    <button
      type="button"
      onClick={() => navigate({ to: `/providers/${p.id}` })}
      className="flex h-full flex-col items-start gap-3 bg-popover p-5 text-left transition-colors hover:bg-ink-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent-400/30 dark:hover:bg-ink-800/40"
    >
      <div className="flex w-full items-start justify-between gap-2">
        <ProviderIcon provider={p} />
        {connected ? (
          <span className="inline-flex items-center gap-1.5 rounded-md bg-accent-100 px-2 py-0.5 text-xs font-medium text-accent-700 dark:bg-accent-800/40 dark:text-accent-200">
            <StatusDot tone="success" />
            Connected
          </span>
        ) : p.notice?.toLowerCase().includes("wasm") ? (
          <Badge variant="secondary">wasm</Badge>
        ) : p.deprecated ? (
          <Badge variant="outline" title={p.notice || "Account may be restricted"}>
            <AlertTriangle className="mr-1 h-3 w-3" />
            unofficial
          </Badge>
        ) : !p.drivable ? (
          <Badge variant="secondary">soon</Badge>
        ) : p.auth_kind === "none" ? (
          <Badge variant="default">free</Badge>
        ) : null}
      </div>

      <div className="min-w-0">
        <p className="truncate text-sm font-semibold">{p.display_name}</p>
        <p className="mt-0.5 truncate font-mono text-xs text-muted-foreground">{p.id}</p>
      </div>

      <p className="mt-auto text-xs text-muted-foreground">
        {connected
          ? `${accountCount} ${accountCount === 1 ? "account" : "accounts"}`
          : "No account"}
      </p>
    </button>
  );
}

// ProviderIcon renders the provider PNG with a colored fallback initial.
function ProviderIcon({ provider: p }: { provider: Provider }) {
  const [errored, setErrored] = useState(false);
  if (errored || !p.icon) {
    return (
      <div
        className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl text-sm font-bold text-white"
        style={{ backgroundColor: p.color || "var(--text-muted)" }}
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
      className="h-10 w-10 shrink-0 rounded-xl object-contain"
    />
  );
}