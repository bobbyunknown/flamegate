import { Field } from "@/components/composite/native-select";
import { EmptyState } from "@/components/composite/empty-state";
import { CardHeader } from "@/components/composite/card-header";
import { useEffect, useRef, useState, useMemo } from "react";
import { useParams, Link } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Plus, Trash2, Plug, X, Zap, ArrowUp, ArrowDown, CheckCircle, ToggleLeft, ToggleRight, Search, Route, AlertCircle, AlertTriangle, RefreshCw, Globe, Copy, Check, Upload, Loader2, XCircle, Layers, FileText } from "lucide-react";
import { api, type Provider, type Account, type ProxyPool, type UpstreamQuota, type ProviderRoutingSettings, type BulkAccountResult, type QuotaAccount } from "../lib/api";
import { KiroConnectModal } from "../components/KiroConnectModal";
import { QoderConnectModal } from "../components/QoderConnectModal";
import { KilocodeConnectModal } from "../components/KilocodeConnectModal";
import { CodebuddyConnectModal } from "../components/CodebuddyConnectModal";
import { CursorConnectModal } from "../components/CursorConnectModal";
import { CommandCodeConnectModal } from "../components/CommandCodeConnectModal";
import { CustomModelsSection } from "../components/CustomModelsSection";
import { useToast } from "../components/Toast";
import { parseKeys } from "../lib/bulk";

import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Spinner } from "@/components/ui/spinner";
import { ErrorBanner } from "@/components/composite/error-banner";
import { Modal } from "@/components/composite/modal";

export function ProviderDetailPage() {
  const { id } = useParams({ strict: false });
  const qc = useQueryClient();
  const toast = useToast();

  const providers = useQuery({ queryKey: ["providers"], queryFn: () => api.providers() });
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: () => api.listAccounts() });
  const bulkQuota = useQuery({
    queryKey: ["quota", "today", id],
    queryFn: () => api.quotaByProvider(id!),
    enabled: !!id,
    staleTime: 60_000,
  });
  const quotaMap: Record<string, QuotaAccount> = Object.fromEntries(
    (bulkQuota.data?.accounts ?? []).map((a) => [a.id, a]),
  );
  const pools = useQuery({ queryKey: ["proxy-pools"], queryFn: () => api.listProxyPools() });
  const routing = useQuery({
    queryKey: ["provider-routing", id],
    queryFn: () => api.providerRouting(id!),
    enabled: !!id,
  });
  const disabledModels = useQuery({
    queryKey: ["disabled-models", id],
    queryFn: () => api.listDisabledModels(id!),
    enabled: !!id,
  });
  const models = useQuery({
    queryKey: ["provider-models", id],
    queryFn: () => api.providerModels(id!),
    enabled: !!id,
    staleTime: 60_000,
  });

  const provider = providers.data?.providers.find((p) => p.id === id);
  const myAccounts = (accounts.data?.accounts ?? []).filter((a) => a.provider === id);

  const [label, setLabel] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [baseURL, setBaseURL] = useState("");
  const [region, setRegion] = useState("");
  const [accountID, setAccountID] = useState("");
  const [azureEndpoint, setAzureEndpoint] = useState("");
  const [azureDeployment, setAzureDeployment] = useState("");
  const [azureAPIVersion, setAzureAPIVersion] = useState("2024-10-01-preview");
  const [azureOrganization, setAzureOrganization] = useState("");
  const [error, setError] = useState("");
  const [kiroOpen, setKiroOpen] = useState(false);
  const [qoderOpen, setQoderOpen] = useState(false);
  const [kilocodeOpen, setKilocodeOpen] = useState(false);
  const [codebuddyOpen, setCodebuddyOpen] = useState(false);
  const [cursorOpen, setCursorOpen] = useState(false);
  const [commandcodeOpen, setCommandcodeOpen] = useState(false);
  const [addKeyOpen, setAddKeyOpen] = useState(false);
  const [bulkOpen, setBulkOpen] = useState(false);

  // Model search and pagination
  const [modelSearchQuery, setModelSearchQuery] = useState("");
  const [modelPage, setModelPage] = useState(1);
  const MODELS_PER_PAGE = 12;

  // Multi-select state for bulk enable/disable. Holds the ids of selected
  // models; selection persists across pagination and search changes.
  const [selectedModelIds, setSelectedModelIds] = useState<Set<string>>(new Set());

  const toggleModelSelection = (id: string) => {
    setSelectedModelIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const filteredModels = useMemo(() => {
    if (!models.data?.models) return [];
    if (!modelSearchQuery.trim()) return models.data.models;
    const lowerQ = modelSearchQuery.toLowerCase();
    return models.data.models.filter(m => 
      m.id.toLowerCase().includes(lowerQ) || 
      (m.name && m.name.toLowerCase().includes(lowerQ)) ||
      (m.kind && m.kind.toLowerCase().includes(lowerQ))
    );
  }, [models.data?.models, modelSearchQuery]);

  useEffect(() => {
    setModelPage(1);
  }, [modelSearchQuery]);

  const totalModelPages = Math.ceil(filteredModels.length / MODELS_PER_PAGE);
  const paginatedModels = filteredModels.slice(
    (modelPage - 1) * MODELS_PER_PAGE, 
    modelPage * MODELS_PER_PAGE
  );

  // Set default region when provider loads.
  useEffect(() => {
    if (provider?.default_region && !region) {
      setRegion(provider.default_region);
    }
  }, [provider, region]);

  const hasRegions = (provider?.regions?.length ?? 0) > 0;

  const create = useMutation({
    mutationFn: () =>
      api.createAccount({
        provider: id!,
        label,
        api_key: apiKey,
        base_url: baseURL || undefined,
        region: hasRegions ? region : undefined,
        account_id: accountID || undefined,
        azure_endpoint: azureEndpoint || undefined,
        azure_deployment: azureDeployment || undefined,
        azure_api_version: azureAPIVersion || undefined,
        azure_organization: azureOrganization || undefined,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["accounts"] });
      setLabel("");
      setApiKey("");
      setBaseURL("");
      setAccountID("");
      setAzureEndpoint("");
      setAzureDeployment("");
      setAzureAPIVersion("2024-10-01-preview");
      setAzureOrganization("");
      setError("");
      setAddKeyOpen(false);
      toast.success("Account connected", `Upstream credentials saved and encrypted. The account is ready for routing.`);
    },
    onError: (e: Error) => {
      setError(e.message);
      toast.error("Account connection failed", e.message);
    },
  });

  const remove = useMutation({
    mutationFn: (accountId: string) => api.deleteAccount(accountId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["accounts"] });
      toast.success("Account removed", "The upstream credential has been deleted and encrypted secrets purged.");
    },
    onError: (e: Error) => toast.error("Account removal failed", e.message),
  });

  const updateAccount = useMutation({
    mutationFn: ({ id: accId, patch }: { id: string; patch: { label?: string; priority?: number; disabled?: boolean; proxy_pool_id?: string } }) =>
      api.updateAccount(accId, patch),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["accounts"] }),
    onError: (e: Error) => toast.error("Account update failed", e.message),
  });

  // Per-account connection test results. Each entry holds the latest status
  // for an account: testing (in flight), ok, or error (with the upstream
  // message). Drives the inline status badge in each account row.
  const [testResults, setTestResults] = useState<Record<string, { status: "testing" | "ok" | "error"; message?: string }>>({});
  const [testingAll, setTestingAll] = useState(false);

  // runTest probes a single account's credentials and records the result.
  // Returns true when the credential is valid. On failure, refetches the
  // account list so server-side state changes (e.g. needs_reconnect) appear
  // immediately.
  const runTest = async (accountId: string): Promise<boolean> => {
    setTestResults((prev) => ({ ...prev, [accountId]: { status: "testing" } }));
    try {
      const res = await api.testAccount(accountId);
      const ok = res.status === "ok";
      setTestResults((prev) => ({ ...prev, [accountId]: { status: ok ? "ok" : "error", message: res.message } }));
      if (!ok) {
        // Refetch accounts so needs_reconnect flag is picked up.
        qc.invalidateQueries({ queryKey: ["accounts"] });
      }
      return ok;
    } catch (e) {
      setTestResults((prev) => ({ ...prev, [accountId]: { status: "error", message: (e as Error).message } }));
      qc.invalidateQueries({ queryKey: ["accounts"] });
      return false;
    }
  };

  const disableModelsMut = useMutation({
    mutationFn: (ids: string[]) => api.disableModels(id!, ids),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["disabled-models", id] });
      toast.success("Models disabled", "Selected models will be excluded from routing until re-enabled.");
    },
    onError: (e: Error) => toast.error("Model disable failed", e.message),
  });

  const enableModelsMut = useMutation({
    mutationFn: (ids: string[]) => api.enableModels(id!, ids),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["disabled-models", id] });
      toast.success("Models re-enabled", "Selected models are available for routing again.");
    },
    onError: (e: Error) => toast.error("Couldn't enable models", e.message),
  });

  const updateRouting = useMutation({
    mutationFn: (patch: Partial<ProviderRoutingSettings>) => api.updateProviderRouting(id!, patch),
    onSuccess: (data) => {
      qc.setQueryData(["provider-routing", id], data);
      toast.success("Routing updated", "Account routing strategy for this provider was saved.");
    },
    onError: (e: Error) => toast.error("Routing update failed", e.message),
  });

  // Multi-select for connected accounts: enables bulk enable / disable / delete.
  const [selectedAccountIds, setSelectedAccountIds] = useState<Set<string>>(new Set());
  // Controls the bulk-delete confirmation dialog (replaces the native confirm()).
  const [bulkDeleteConfirmOpen, setBulkDeleteConfirmOpen] = useState(false);
  const toggleAccountSelection = (accId: string) =>
    setSelectedAccountIds((prev) => {
      const next = new Set(prev);
      if (next.has(accId)) next.delete(accId);
      else next.add(accId);
      return next;
    });
  const clearAccountSelection = () => setSelectedAccountIds(new Set());

  const bulkUpdateAccounts = useMutation({
    mutationFn: async ({ ids, disabled }: { ids: string[]; disabled: boolean }) => {
      await Promise.all(ids.map((accId) => api.updateAccount(accId, { disabled })));
    },
    onSuccess: (_, { ids, disabled }) => {
      qc.invalidateQueries({ queryKey: ["accounts"] });
      clearAccountSelection();
      toast.success(
        `${ids.length} account${ids.length > 1 ? "s" : ""} ${disabled ? "disabled" : "enabled"}`,
        disabled ? "Selected accounts are paused and excluded from routing." : "Selected accounts are active again.",
      );
    },
    onError: (e: Error) => toast.error("Bulk update failed", e.message),
  });

  const bulkDeleteAccounts = useMutation({
    mutationFn: async (ids: string[]) => {
      await Promise.all(ids.map((accId) => api.deleteAccount(accId)));
    },
    onSuccess: (_, ids) => {
      qc.invalidateQueries({ queryKey: ["accounts"] });
      clearAccountSelection();
      setBulkDeleteConfirmOpen(false);
      toast.success(`${ids.length} account${ids.length > 1 ? "s" : ""} removed`, "Encrypted secrets have been purged.");
    },
    onError: (e: Error) => toast.error("Bulk removal failed", e.message),
  });

  // Sort accounts by priority for display.
  const sortedAccounts = [...myAccounts].sort((a, b) => a.priority - b.priority);
  const disabledModelIds = new Set(disabledModels.data?.ids ?? []);

  // Derived selection state (scoped to this provider's accounts).
  const selectedList = sortedAccounts.filter((a) => selectedAccountIds.has(a.id));
  const allAccountsSelected = sortedAccounts.length > 0 && selectedList.length === sortedAccounts.length;
  const someAccountsSelected = selectedList.length > 0 && !allAccountsSelected;
  const bulkBusy = bulkUpdateAccounts.isPending || bulkDeleteAccounts.isPending;

  const toggleSelectAllAccounts = () => {
    if (allAccountsSelected) clearAccountSelection();
    else setSelectedAccountIds(new Set(sortedAccounts.map((a) => a.id)));
  };
  const handleBulkDisable = () => {
    const ids = selectedList.filter((a) => !a.disabled).map((a) => a.id);
    if (ids.length) bulkUpdateAccounts.mutate({ ids, disabled: true });
  };
  const handleBulkEnable = () => {
    const ids = selectedList.filter((a) => a.disabled).map((a) => a.id);
    if (ids.length) bulkUpdateAccounts.mutate({ ids, disabled: false });
  };
  const handleBulkDeleteAccounts = () => {
    if (!selectedList.length) return;
    setBulkDeleteConfirmOpen(true);
  };
  const confirmBulkDeleteAccounts = () => {
    const ids = selectedList.map((a) => a.id);
    if (!ids.length) return;
    bulkDeleteAccounts.mutate(ids);
  };

  // runTestAll tests every account sequentially (one at a time), updating each
  // row's status as it goes, then summarizes the outcome. Failures don't stop
  // the run — every account is checked.
  const runTestAll = async () => {
    if (testingAll || sortedAccounts.length === 0) return;
    setTestingAll(true);
    let ok = 0;
    let failed = 0;
    for (const a of sortedAccounts) {
      const success = await runTest(a.id);
      if (success) ok++;
      else failed++;
    }
    setTestingAll(false);
    if (failed === 0) {
      toast.success("All accounts verified", `${ok} account${ok === 1 ? "" : "s"} passed the connection test.`);
    } else {
      toast.error("Some checks failed", `${ok} ok, ${failed} failed.`);
    }
  };

  const moveAccount = (accId: string, direction: "up" | "down") => {
    const idx = sortedAccounts.findIndex((a) => a.id === accId);
    if (idx < 0) return;
    const target = direction === "up" ? idx - 1 : idx + 1;
    if (target < 0 || target >= sortedAccounts.length) return;
    const swapFrom = sortedAccounts[idx];
    const swapTo = sortedAccounts[target];
    // Optimistically swap priorities in the query cache for instant UI.
    qc.setQueryData<{ accounts: Account[] }>(["accounts"], (old) => {
      if (!old) return old;
      return {
        accounts: old.accounts.map((a) => {
          if (a.id === swapFrom.id) return { ...a, priority: swapTo.priority };
          if (a.id === swapTo.id) return { ...a, priority: swapFrom.priority };
          return a;
        }),
      };
    });
    // Persist both swaps to backend, refetch on settle.
    updateAccount.mutate({ id: swapFrom.id, patch: { priority: swapTo.priority } });
    updateAccount.mutate({ id: swapTo.id, patch: { priority: swapFrom.priority } }, {
      onSettled: () => qc.invalidateQueries({ queryKey: ["accounts"] }),
    });
  };

  if (providers.isLoading) return <Spinner />;
  if (!provider) {
    return (
      <Card className="px-6 py-12 text-center">
        <p className="text-sm text-muted-foreground">Provider not found.</p>
        <Link to="/providers" className="mt-3 inline-block text-sm font-medium text-accent-600">
          Back to Providers
        </Link>
      </Card>
    );
  }

  const isKiro = provider.id === "kiro";
  const isQoder = provider.id === "qoder";
  const isKilocode = provider.id === "kilocode";
  const isCodebuddy = provider.id === "codebuddy";
  const isCursor = provider.id === "cursor";
  const isCommandCode = provider.id === "commandcode";
  const hasCustomModal = isKiro || isQoder || isKilocode || isCodebuddy || isCursor || isCommandCode;
  const supportsManualConnect = !hasCustomModal && (
    provider.auth_modes.includes("api_key") ||
    provider.auth_modes.includes("none")
  );
  // Bulk key upload applies to providers authenticated by API key. It is hidden
  // for Azure (each key needs its own endpoint + deployment, so there is no
  // shared config to bulk against) and for no-auth providers (nothing to bulk).
  const providerSupportsApiKey = provider.auth_modes.includes("api_key") || provider.auth_kind === "api_key";
  const supportsBulkUpload = supportsManualConnect && providerSupportsApiKey && provider.id !== "azure";

  return (
    <>
      <Link
        to="/providers"
        className="mb-5 inline-flex items-center gap-2 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        Back to Providers
      </Link>

      <header className="mb-7 flex items-start gap-4">
        <ProviderIcon provider={provider} size={56} />
        <div className="min-w-0 flex-1">
          <h1 className="font-display text-3xl font-semibold tracking-tight">{provider.display_name}</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {myAccounts.length} connected {myAccounts.length === 1 ? "account" : "accounts"}
          </p>
          <div className="mt-2 flex flex-wrap gap-1">
            {(provider.service_kinds ?? []).map((k) => (
              <Badge key={k} variant="secondary">
                {k}
              </Badge>
            ))}
            {provider.deprecated && (
              <Badge variant="outline" title={provider.notice || "Account may be restricted"}>
                <AlertTriangle className="mr-1 h-3 w-3 text-amber-500" />
                unofficial
              </Badge>
            )}
            {provider.auth_kind === "none" && (
              <Badge variant="secondary">free</Badge>
            )}
          </div>
          {provider.custom && provider.base_url && (
            <BaseURLDisplay baseURL={provider.base_url} dialect={provider.dialect} />
          )}
        </div>
      </header>

      {provider.deprecated && provider.notice && (
        <div className="mb-4 flex items-start gap-2.5 rounded-lg border border-[color:var(--color-warning)]/25 bg-[color:var(--color-warning)]/8 px-4 py-3 text-xs leading-relaxed text-[color:var(--color-warning)]">
          <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
          <span>{provider.notice}</span>
        </div>
      )}

      <div className="space-y-6">
        <Card>
          <CardHeader
            title="Connected accounts"
            action={
              <div className="flex items-center gap-2">
                {myAccounts.length > 0 && (
                  <Button
                    variant="ghost"
                    className="h-8 px-3 text-xs"
                    onClick={runTestAll}
                    disabled={testingAll}
                  >
                    <CheckCircle className={`h-3.5 w-3.5 ${testingAll ? "animate-pulse" : ""}`} />
                    {testingAll
                      ? `Testing ${Object.values(testResults).filter((r) => r.status !== "testing").length}/${myAccounts.length}`
                      : "Test all"}
                  </Button>
                )}
                {isKiro && (
                  <Button variant="ghost" className="h-8 px-3 text-xs" onClick={() => setKiroOpen(true)}>
                    <Plug className="h-3.5 w-3.5" />
                    Connect Kiro
                  </Button>
                )}
                {isQoder && (
                  <Button variant="ghost" className="h-8 px-3 text-xs" onClick={() => setQoderOpen(true)}>
                    <Plug className="h-3.5 w-3.5" />
                    Connect Qoder
                  </Button>
                )}
                {isKilocode && (
                  <Button variant="ghost" className="h-8 px-3 text-xs" onClick={() => setKilocodeOpen(true)}>
                    <Plug className="h-3.5 w-3.5" />
                    Connect Kilo Code
                  </Button>
                )}
                {isCodebuddy && (
                  <Button variant="ghost" className="h-8 px-3 text-xs" onClick={() => setCodebuddyOpen(true)}>
                    <Plug className="h-3.5 w-3.5" />
                    Connect CodeBuddy
                  </Button>
                )}
                {isCursor && (
                  <Button variant="ghost" className="h-8 px-3 text-xs" onClick={() => setCursorOpen(true)}>
                    <Plug className="h-3.5 w-3.5" />
                    Connect Cursor
                  </Button>
                )}
                {isCommandCode && (
                  <Button variant="ghost" className="h-8 px-3 text-xs" onClick={() => setCommandcodeOpen(true)}>
                    <Plug className="h-3.5 w-3.5" />
                    Connect CLI
                  </Button>
                )}
                {supportsManualConnect && (
                  <Button variant="ghost" className="h-8 px-3 text-xs" onClick={() => setAddKeyOpen(true)}>
                    <Plus className="h-3.5 w-3.5" />
                    {provider.auth_kind === "none" ? "Connect" : "Add API key"}
                  </Button>
                )}
                {supportsBulkUpload && (
                  <Button variant="ghost" className="h-8 px-3 text-xs" onClick={() => setBulkOpen(true)}>
                    <Layers className="h-3.5 w-3.5" />
                    Bulk add
                  </Button>
                )}
              </div>
            }
          />
          {routing.data && (
            <RoutingControls
              settings={routing.data}
              saving={updateRouting.isPending}
              onUpdate={(patch) => updateRouting.mutate(patch)}
            />
          )}
          {myAccounts.some((a) => a.needs_reconnect) && (
            <div className="flex items-start gap-2.5 border-t border-[color:var(--color-warning)]/25 bg-[color:var(--color-warning)]/8 px-6 py-3 text-xs leading-relaxed text-[color:var(--color-warning)]">
              <RefreshCw className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <span>
                One or more accounts have a revoked OAuth token and cannot be refreshed.
                Delete the affected account and reconnect to restore access.
              </span>
            </div>
          )}
          {accounts.isLoading ? (
            <Spinner />
          ) : !myAccounts.length ? (
            <EmptyState
              title="No accounts yet"
              hint="Add an account to start routing through this provider."
            />
          ) : (
            <>
              <div className="flex flex-wrap items-center gap-2 border-t border-border bg-muted px-4 py-2.5">
                <label className="flex cursor-pointer items-center gap-2 text-xs text-muted-foreground">
                  <input
                    type="checkbox"
                    className="h-3.5 w-3.5 rounded border-border accent-[var(--color-primary-container)]"
                    checked={allAccountsSelected}
                    ref={(el) => { if (el) el.indeterminate = someAccountsSelected; }}
                    onChange={toggleSelectAllAccounts}
                  />
                  Select all
                </label>
                {selectedList.length > 0 ? (
                  <>
                    <span className="text-xs text-muted-foreground">{selectedList.length} selected</span>
                    <div className="flex-1" />
                    <Button variant="ghost" className="h-7 px-2 text-xs" onClick={handleBulkEnable} disabled={bulkBusy}>
                      <ToggleRight className="h-3.5 w-3.5 text-[var(--color-primary-container)]" />
                      Enable
                    </Button>
                    <Button variant="ghost" className="h-7 px-2 text-xs" onClick={handleBulkDisable} disabled={bulkBusy}>
                      <ToggleLeft className="h-3.5 w-3.5" />
                      Disable
                    </Button>
                    <Button variant="ghost" className="h-7 px-2 text-xs" onClick={handleBulkDeleteAccounts} disabled={bulkBusy}>
                      <Trash2 className="h-3.5 w-3.5 text-red-500" />
                      Delete
                    </Button>
                    <Button variant="ghost" className="h-7 px-2 text-xs" onClick={clearAccountSelection} disabled={bulkBusy}>
                      Clear
                    </Button>
                  </>
                ) : (
                  <span className="text-xs text-muted-foreground">Select accounts for bulk actions</span>
                )}
              </div>
              <div className="divide-y divide-border">
                {sortedAccounts.map((a, i) => (
                  <AccountRow
                    key={a.id}
                    account={a}
                    index={i}
                    total={sortedAccounts.length}
                    pools={pools.data?.pools ?? []}
                    selected={selectedAccountIds.has(a.id)}
                    onToggleSelect={() => toggleAccountSelection(a.id)}
                    onDelete={() => remove.mutate(a.id)}
                    onMoveUp={() => moveAccount(a.id, "up")}
                    onMoveDown={() => moveAccount(a.id, "down")}
                    onTest={() => runTest(a.id)}
                    onUpdateProxy={(patch) => updateAccount.mutate({ id: a.id, patch })}
                    testResult={testResults[a.id]}
                    disabledByBatch={testingAll}
                    quotaData={quotaMap[a.id]}
                  />
                ))}
              </div>
            </>
          )}
        </Card>

        {/* Available Models */}
        {models.data?.models && models.data.models.length > 0 && (
          <Card>
            <CardHeader
              title="Available Models"
              description={`${models.data.models.length} model${models.data.models.length === 1 ? "" : "s"} configured for this provider.`}
            />
            <div className="flex flex-col gap-3 border-t border-border bg-muted px-6 py-3 sm:flex-row sm:items-center sm:justify-between">
              <div className="relative w-full max-w-sm">
                <Search className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  placeholder="Search models..."
                  value={modelSearchQuery}
                  onChange={(e) => setModelSearchQuery(e.target.value)}
                  className="pl-9 h-8 text-sm"
                />
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <label className="flex cursor-pointer items-center gap-1.5 text-xs text-muted-foreground">
                  <input
                    type="checkbox"
                    className="h-3.5 w-3.5 rounded border-border accent-[var(--color-primary-container)]"
                    checked={filteredModels.length > 0 && filteredModels.every((m) => selectedModelIds.has(m.id))}
                    ref={(el) => {
                      if (el) {
                        const someSelected = filteredModels.some((m) => selectedModelIds.has(m.id));
                        const allSelected = filteredModels.length > 0 && filteredModels.every((m) => selectedModelIds.has(m.id));
                        el.indeterminate = someSelected && !allSelected;
                      }
                    }}
                    onChange={(e) => {
                      setSelectedModelIds((prev) => {
                        const next = new Set(prev);
                        if (e.target.checked) {
                          filteredModels.forEach((m) => next.add(m.id));
                        } else {
                          filteredModels.forEach((m) => next.delete(m.id));
                        }
                        return next;
                      });
                    }}
                  />
                  Select all
                </label>
                {selectedModelIds.size > 0 && (
                  <span className="text-xs text-muted-foreground">{selectedModelIds.size} selected</span>
                )}
                <Button
                  variant="ghost"
                  className="h-8 px-3 text-xs"
                  onClick={() => enableModelsMut.mutate([...selectedModelIds])}
                  disabled={enableModelsMut.isPending || selectedModelIds.size === 0}
                >
                  <ToggleRight className="h-3.5 w-3.5 text-[var(--color-primary-container)]" />
                  Enable
                </Button>
                <Button
                  variant="ghost"
                  className="h-8 px-3 text-xs"
                  onClick={() => disableModelsMut.mutate([...selectedModelIds])}
                  disabled={disableModelsMut.isPending || selectedModelIds.size === 0}
                >
                  <ToggleLeft className="h-3.5 w-3.5 text-muted-foreground" />
                  Disable
                </Button>
                {selectedModelIds.size > 0 && (
                  <Button
                    variant="ghost"
                    className="h-8 px-2 text-xs"
                    onClick={() => setSelectedModelIds(new Set())}
                  >
                    Clear
                  </Button>
                )}
              </div>
            </div>
            {filteredModels.length === 0 ? (
              <div className="px-6 py-12 text-center text-sm text-muted-foreground border-t border-border">
                No models found matching "{modelSearchQuery}"
              </div>
            ) : (
              <div className={`grid grid-cols-1 gap-px overflow-hidden border-t border-border bg-border sm:grid-cols-2 lg:grid-cols-3 ${totalModelPages <= 1 ? "rounded-b-2xl" : ""}`}>
                {paginatedModels.map((m) => (
                  <ModelCell
                    key={m.id}
                    model={m}
                    provider={provider}
                    disabled={disabledModelIds.has(m.id)}
                    selected={selectedModelIds.has(m.id)}
                    onToggleSelect={() => toggleModelSelection(m.id)}
                    onToggleDisable={() => {
                      if (disabledModelIds.has(m.id)) {
                        enableModelsMut.mutate([m.id]);
                      } else {
                        disableModelsMut.mutate([m.id]);
                      }
                    }}
                  />
                ))}
              </div>
            )}
            {totalModelPages > 0 && (
              <div className="flex items-center justify-between rounded-b-2xl border-t border-border bg-muted px-6 py-3">
                <span className="text-xs text-muted-foreground">
                  Showing {(modelPage - 1) * MODELS_PER_PAGE + 1} to {Math.min(modelPage * MODELS_PER_PAGE, filteredModels.length)} of {filteredModels.length} models
                </span>
                <div className="flex items-center gap-1">
                  <Button
                    variant="ghost"
                    className="h-8 px-2 text-xs"
                    disabled={modelPage === 1}
                    onClick={() => setModelPage((p) => p - 1)}
                  >
                    Previous
                  </Button>
                  <Button
                    variant="ghost"
                    className="h-8 px-2 text-xs"
                    disabled={modelPage === totalModelPages}
                    onClick={() => setModelPage((p) => p + 1)}
                  >
                    Next
                  </Button>
                </div>
              </div>
            )}
          </Card>
        )}

        {/* User-registered custom models (separate from the catalog list). */}
        <CustomModelsSection provider={provider} />
      </div>

      {kiroOpen && <KiroConnectModal onClose={() => setKiroOpen(false)} />}
      {qoderOpen && <QoderConnectModal onClose={() => setQoderOpen(false)} />}
      {kilocodeOpen && <KilocodeConnectModal onClose={() => setKilocodeOpen(false)} />}
      {codebuddyOpen && <CodebuddyConnectModal onClose={() => setCodebuddyOpen(false)} />}
      {cursorOpen && <CursorConnectModal onClose={() => setCursorOpen(false)} />}
      {commandcodeOpen && <CommandCodeConnectModal onClose={() => setCommandcodeOpen(false)} />}
      {addKeyOpen && (
        <AddApiKeyModal
          provider={provider}
          hasRegions={hasRegions}
          label={label}
          apiKey={apiKey}
          baseURL={baseURL}
          region={region}
          accountID={accountID}
          azureEndpoint={azureEndpoint}
          azureDeployment={azureDeployment}
          azureAPIVersion={azureAPIVersion}
          azureOrganization={azureOrganization}
          error={error}
          pending={create.isPending}
          onLabel={setLabel}
          onApiKey={setApiKey}
          onBaseURL={setBaseURL}
          onRegion={setRegion}
          onAccountID={setAccountID}
          onAzureEndpoint={setAzureEndpoint}
          onAzureDeployment={setAzureDeployment}
          onAzureAPIVersion={setAzureAPIVersion}
          onAzureOrganization={setAzureOrganization}
          onSubmit={() => create.mutate()}
          onClose={() => { setAddKeyOpen(false); setError(""); }}
        />
      )}
      {bulkOpen && (
        <BulkAddKeysModal provider={provider} onClose={() => setBulkOpen(false)} />
      )}
      <Modal
        open={bulkDeleteConfirmOpen}
        onClose={() => { if (!bulkDeleteAccounts.isPending) setBulkDeleteConfirmOpen(false); }}
        title="Delete selected accounts?"
        subtitle={`${selectedList.length} account${selectedList.length > 1 ? "s" : ""} on ${label} will be removed.`}
        maxWidth="max-w-md"
      >
        <div className="space-y-4 px-6 py-5">
          <div className="flex items-start gap-3 rounded-xl border border-[color:var(--color-danger)]/30 bg-[color:var(--color-danger)]/10 px-3.5 py-3">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-[color:var(--color-danger)]" strokeWidth={2} />
            <div className="text-sm leading-snug text-[color:var(--color-danger)]">
              This permanently purges each account's encrypted secrets and removes them from routing.
              <span className="font-semibold"> This action cannot be undone.</span>
            </div>
          </div>
          {selectedList.length > 0 && (
            <ul className="max-h-40 space-y-1 overflow-y-auto rounded-xl border border-border bg-muted px-3.5 py-2.5">
              {selectedList.map((a) => (
                <li key={a.id} className="flex items-center gap-2 text-sm text-foreground">
                  <Trash2 className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                  <span className="truncate">{a.label || a.id}</span>
                </li>
              ))}
            </ul>
          )}
          <div className="flex justify-end gap-2">
            <Button
              variant="ghost"
              onClick={() => setBulkDeleteConfirmOpen(false)}
              disabled={bulkDeleteAccounts.isPending}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={confirmBulkDeleteAccounts}
              disabled={bulkDeleteAccounts.isPending}
            >
              {bulkDeleteAccounts.isPending ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Trash2 className="h-3.5 w-3.5" />
              )}
              Delete {selectedList.length} account{selectedList.length > 1 ? "s" : ""}
            </Button>
          </div>
        </div>
      </Modal>
    </>
  );
}

// BaseURLDisplay shows the upstream base URL for a user-defined custom
// provider (OpenAI- or Anthropic-compatible) on the provider detail header,
// with a one-click copy affordance. Hidden for built-in providers whose base
// URL is fixed and not user-configurable.
function BaseURLDisplay({ baseURL, dialect }: { baseURL: string; dialect?: string }) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(baseURL);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard may be unavailable (insecure context); silently ignore.
    }
  };

  const dialectLabel =
    dialect === "anthropic"
      ? "Anthropic-compatible"
      : dialect === "openai"
        ? "OpenAI-compatible"
        : dialect;

  return (
    <div className="mt-3 flex flex-wrap items-center gap-2">
      <div className="inline-flex max-w-full items-center gap-2 rounded-lg border border-border bg-muted px-3 py-1.5">
        <Globe className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="text-xs font-medium text-muted-foreground">Base URL</span>
        <code className="truncate font-mono text-xs text-foreground" title={baseURL}>
          {baseURL}
        </code>
        <Button
          variant="ghost"
          size="icon-xs"
          type="button"
          onClick={copy}
          title="Copy base URL"
        >
          {copied ? (
            <Check className="h-3.5 w-3.5 text-[color:var(--color-success)]" />
          ) : (
            <Copy className="h-3.5 w-3.5" />
          )}
        </Button>
      </div>
      {dialectLabel && (
        <Badge variant="secondary">{dialectLabel}</Badge>
      )}
    </div>
  );
}

const routingOptions = [
  { value: "inherit", label: "Inherit" },
  { value: "fill-first", label: "Fill first" },
  { value: "round-robin", label: "Round robin" },
  { value: "smart-round-robin", label: "Smart" },
];

function RoutingControls({
  settings,
  saving,
  onUpdate,
}: {
  settings: ProviderRoutingSettings;
  saving: boolean;
  onUpdate: (patch: Partial<ProviderRoutingSettings>) => void;
}) {
  const mode = settings?.routing_strategy || "inherit";
  const stickyLimit = settings?.sticky_limit || 3;
  const ttlHours = Math.max(1, Math.round((settings?.affinity_ttl_minutes || 1440) / 60));
  const rotatesAccounts = mode === "round-robin" || mode === "smart-round-robin";

  return (
    <div className="border-t border-border bg-muted px-6 py-3">
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex items-center gap-2">
          <Route className="h-3.5 w-3.5 text-muted-foreground" />
          <span className="text-xs font-medium text-muted-foreground">Routing</span>
        </div>
        <Select value={mode} onValueChange={(val) => onUpdate({ routing_strategy: val })} disabled={saving}>
          <SelectTrigger className="h-7 w-32 border-border bg-background text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {routingOptions.map((o) => (
              <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
            ))}
          </SelectContent>
        </Select>

        {rotatesAccounts && (
          <>
            <span className="text-border">·</span>
            <div className="flex items-center gap-1.5">
              <span className="text-xs text-muted-foreground">Sticky</span>
              <Input
                type="number"
                min={1}
                max={100}
                value={stickyLimit}
                disabled={saving}
                onChange={(e) => onUpdate({ sticky_limit: parseInt(e.target.value, 10) || 1 })}
                className="h-7 w-16 text-center text-xs"
              />
            </div>
          </>
        )}

        {mode === "smart-round-robin" && (
          <>
            <span className="text-border">·</span>
            <div className="flex items-center gap-1.5">
              <span className="text-xs text-muted-foreground">Affinity TTL</span>
              <Input
                type="number"
                min={1}
                max={168}
                value={ttlHours}
                disabled={saving}
                onChange={(e) => onUpdate({ affinity_ttl_minutes: (parseInt(e.target.value, 10) || 1) * 60 })}
                className="h-7 w-16 text-center text-xs"
              />
              <span className="text-xs text-muted-foreground">h</span>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

function AccountRow({
  account: a,
  index,
  total,
  pools,
  selected,
  onToggleSelect,
  onDelete,
  onMoveUp,
  onMoveDown,
  onTest,
  onUpdateProxy,
  testResult,
  disabledByBatch,
  quotaData,
}: {
  account: Account;
  index: number;
  total: number;
  pools: ProxyPool[];
  selected?: boolean;
  onToggleSelect?: () => void;
  onDelete: () => void;
  onMoveUp: () => void;
  onMoveDown: () => void;
  onTest: () => void;
  onUpdateProxy: (patch: { priority?: number; proxy_pool_id?: string; disabled?: boolean }) => void;
  testResult?: { status: "testing" | "ok" | "error"; message?: string };
  disabledByBatch?: boolean;
  quotaData?: QuotaAccount;
}) {
  const testing = testResult?.status === "testing";
  const [localPriority, setLocalPriority] = useState(a.priority);
  const priorityRef = useRef(a.priority);

  // Keep local priority in sync when account data changes from server.
  if (a.priority !== priorityRef.current) {
    priorityRef.current = a.priority;
    setLocalPriority(a.priority);
  }

  const commitPriority = () => {
    const val = localPriority;
    if (!isNaN(val) && val >= 0 && val !== a.priority) {
      onUpdateProxy({ priority: val });
    }
  };

  const hasQuota = !!quotaData?.upstream_quotas && quotaData.upstream_quotas.length > 0;
  const boundPool = pools.find((p) => p.id === a.proxy_pool_id);

  return (
    <div className={`px-4 py-3 ${a.disabled ? "opacity-60" : ""} ${selected ? "bg-accent-50/50 dark:bg-accent-900/10" : ""}`}>
      {/* Header row */}
      <div className="flex items-center justify-between gap-3">
        {onToggleSelect && (
          <input
            type="checkbox"
            checked={!!selected}
            onChange={onToggleSelect}
            aria-label={`Select ${a.label || a.provider}`}
            className="h-3.5 w-3.5 shrink-0 rounded border-border accent-[var(--color-primary-container)]"
          />
        )}
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="text-sm font-medium">{a.label || a.provider}</span>
            <Badge variant="secondary">{a.auth_kind === "oauth" ? "OAuth" : "API Key"}</Badge>
            {a.disabled && <Badge variant="destructive">disabled</Badge>}
            {a.needs_reconnect && (
              <Badge
                variant="warning"
                className="text-[10px]"
                title="The OAuth refresh token was revoked by the provider. Delete this account and reconnect."
              >
                <RefreshCw className="h-3 w-3" />
                reconnect required
              </Badge>
            )}
            {testResult?.status === "ok" && (
              <span className="inline-flex items-center gap-1 rounded-full bg-[var(--color-primary-container)]/10 px-1.5 py-0.5 text-[10px] font-medium text-[var(--color-primary-container)] dark:bg-[var(--color-primary-container)]/30 dark:text-[var(--color-primary)]">
                ✓ ok
              </span>
            )}
            {testResult?.status === "error" && (
              <span
                className="inline-flex items-center gap-1 rounded-full bg-red-100 px-1.5 py-0.5 text-[10px] font-medium text-red-700 dark:bg-red-900/30 dark:text-red-400"
                title={testResult.message}
              >
                ✗ {testResult.message ? "failed" : "error"}
              </span>
            )}
            {testResult?.status === "testing" && (
              <span className="inline-flex items-center gap-1 rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">
                testing…
              </span>
            )}
          </div>
          {testResult?.status === "error" && testResult.message && (
            <div className="mt-2 flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2 dark:border-red-900/40 dark:bg-red-900/15">
              <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-red-500 dark:text-red-400" />
              <div className="min-w-0">
                <p className="text-[11px] font-medium text-red-700 dark:text-red-300">Connection failed</p>
                <p className="mt-0.5 break-words text-[11px] leading-relaxed text-red-600/90 dark:text-red-400/90">
                  {testResult.message}
                </p>
              </div>
            </div>
          )}
        </div>
        <div className="flex shrink-0 items-center gap-0.5">
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={onTest}
            disabled={testing || disabledByBatch}
            title="Test credentials"
          >
            <CheckCircle className={`h-4 w-4 ${testing ? "animate-pulse" : ""}`} />
          </Button>
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={() => onUpdateProxy({ disabled: !a.disabled })}
            title={a.disabled ? "Enable" : "Disable"}
          >
            {a.disabled ? <ToggleLeft className="h-4 w-4" /> : <ToggleRight className="h-4 w-4 text-[var(--color-primary-container)] dark:text-[var(--color-primary)]" />}
          </Button>
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={onDelete}
            title="Delete"
            className="hover:bg-destructive/10 hover:text-destructive"
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* Settings row: Priority + Proxy Pool */}
      <div className="mt-2 flex flex-wrap items-center gap-3">
        {/* Priority */}
        <div className="flex items-center gap-1.5">
          <span className="text-[11px] text-muted-foreground">Priority:</span>
          <div className="inline-flex items-center overflow-hidden rounded-md border border-border bg-muted">
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={onMoveUp}
              disabled={index === 0}
            >
              <ArrowUp className="h-3 w-3" />
            </Button>
            <input
              type="number"
              value={localPriority}
              onChange={(e) => {
                const val = parseInt(e.target.value, 10);
                if (!isNaN(val) && val >= 0) setLocalPriority(val);
              }}
              onBlur={commitPriority}
              onKeyDown={(e) => e.key === "Enter" && (e.target as HTMLInputElement).blur()}
              className="h-6 w-10 border-x border-border bg-transparent text-center text-xs font-medium text-foreground focus:outline-none focus:bg-background [appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none"
              min={0}
              max={999}
            />
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={onMoveDown}
              disabled={index === total - 1}
            >
              <ArrowDown className="h-3 w-3" />
            </Button>
          </div>
        </div>

        {/* Proxy Pool */}
        <div className="flex items-center gap-1.5">
          <span className="text-[11px] text-muted-foreground">Proxy:</span>
          <Select
            value={a.proxy_pool_id || "direct"}
            onValueChange={(val) => onUpdateProxy({ proxy_pool_id: val === "direct" ? "" : val })}
          >
            <SelectTrigger className="h-6 w-36 border-border bg-muted text-[11px]">
              <SelectValue placeholder="Direct (no proxy)" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="direct">Direct (no proxy)</SelectItem>
              {pools.map((p) => (
                <SelectItem key={p.id} value={p.id}>
                  {p.name}{!p.is_active ? " (inactive)" : ""}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {boundPool && (
            <span className={`inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 text-[10px] font-medium ${
              boundPool.test_status === "active"
                ? "bg-[var(--color-primary-container)]/10 text-[var(--color-primary-container)] dark:bg-[var(--color-primary-container)]/30 dark:text-[var(--color-primary)]"
                : boundPool.test_status === "error"
                  ? "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400"
                  : "bg-muted text-muted-foreground"
            }`}>
              {boundPool.test_status === "active" ? "✓" : boundPool.test_status === "error" ? "✗" : "?"}
              {boundPool.type !== "http" && ` ${boundPool.type}`}
            </span>
          )}
        </div>
      </div>

      {/* Quota / credit info */}
      {hasQuota && quotaData && (
        <div className="mt-2.5 rounded-lg border border-border bg-muted px-3 py-2.5">
          <div className="mb-2 flex items-center gap-2">
            <Zap className="h-3.5 w-3.5 text-muted-foreground" />
            <span className="text-xs font-medium">
              {quotaData.plan_name ? `${quotaData.plan_name} — Credits` : "Credits & Quota"}
            </span>
          </div>
          {quotaData.upstream_quotas && (
            <div className="space-y-2">
              {quotaData.upstream_quotas.map((q) => (
                <QuotaBarInline key={q.resource_type} quota={q} />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function QuotaBarInline({ quota: q }: { quota: UpstreamQuota }) {
  const pct = q.limit > 0 ? Math.min(100, Math.round((q.used / q.limit) * 100)) : 0;
  const remainingPct = q.limit > 0 ? Math.round((q.remaining / q.limit) * 100) : 0;
  const tone =
    remainingPct < 30
      ? "bg-[color:var(--color-danger)]"
      : remainingPct < 70
        ? "bg-[color:var(--color-warning)]"
        : "bg-accent-500";
  const label = q.resource_type
    .toLowerCase()
    .replace(/_/g, " ")
    .replace(/\b\w/g, (c) => c.toUpperCase());

  const resetDate = q.reset_at ? new Date(q.reset_at) : null;
  const resetLabel = resetDate && !isNaN(resetDate.getTime())
    ? resetDate.toLocaleDateString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })
    : null;

  return (
    <div>
      <div className="mb-1 flex items-center justify-between text-[11px]">
        <span className="font-medium text-foreground">{label}</span>
        <div className="flex items-center gap-2">
          {resetLabel && (
            <span className="text-[10px] text-muted-foreground">resets {resetLabel}</span>
          )}
          <span className="tabular-nums">
            {q.used.toLocaleString()} / {q.limit.toLocaleString()}
            <span className="ml-1 text-muted-foreground">({q.remaining.toLocaleString()} left)</span>
          </span>
        </div>
      </div>
      <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
        <div className={`h-full rounded-full ${tone}`} style={{ width: `${Math.max(2, pct)}%` }} />
      </div>
    </div>
  );
}

// BulkAddKeysModal imports many API keys for a provider in one shot. Shared
// provider config (base URL, region, Cloudflare account) is entered once and
// applied to every key; only the key (and an optional inline label / base URL)
// varies per line. The standardized paste format is parsed live with a preview,
// keys can be loaded from a .txt/.csv file, and the backend returns a per-row
// outcome that is rendered after import.
function BulkAddKeysModal({ provider, onClose }: { provider: Provider; onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [text, setText] = useState("");
  const [validate, setValidate] = useState(false);
  const [baseURL, setBaseURL] = useState(provider.base_url ?? "");
  const [region, setRegion] = useState(provider.default_region ?? "");
  const [accountID, setAccountID] = useState("");
  const [results, setResults] = useState<BulkAccountResult[] | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const isCloudflare = provider.id === "cloudflare-ai";
  const isCustom = provider.id === "custom-openai" || provider.id === "custom-anthropic" || !!provider.custom;
  const hasRegions = (provider.regions?.length ?? 0) > 0;
  const requiresBaseURL = isCustom;
  // Generic providers expose an optional shared base URL; region/Cloudflare
  // providers use their own dedicated control instead.
  const showBaseURL = !hasRegions && !isCloudflare;
  const keyPlaceholder = provider.id === "xai" ? "xai-..." : "sk-...";

  const parsed = useMemo(() => parseKeys(text), [text]);
  const validCount = parsed.entries.length;

  const importMut = useMutation({
    mutationFn: () =>
      api.bulkCreateAccounts({
        provider: provider.id,
        base_url: showBaseURL && baseURL.trim() ? baseURL.trim() : undefined,
        region: hasRegions ? region : undefined,
        account_id: isCloudflare ? accountID.trim() : undefined,
        validate,
        items: parsed.entries.map((e) => ({
          label: e.label || undefined,
          api_key: e.apiKey,
          base_url: e.baseURL,
        })),
      }),
    onSuccess: (res) => {
      setResults(res.results);
      qc.invalidateQueries({ queryKey: ["accounts"] });
      if (res.failed === 0) {
        toast.success(
          "Bulk import complete",
          `${res.created} key${res.created === 1 ? "" : "s"} added${res.skipped ? `, ${res.skipped} duplicate skipped` : ""}.`,
        );
      } else {
        toast.error(
          "Bulk import finished with errors",
          `${res.created} added, ${res.failed} failed${res.skipped ? `, ${res.skipped} skipped` : ""}.`,
        );
      }
    },
    onError: (e: Error) => toast.error("Bulk import failed", e.message),
  });

  const canImport =
    validCount > 0 &&
    !importMut.isPending &&
    (!requiresBaseURL || !!baseURL.trim()) &&
    (!isCloudflare || !!accountID.trim());
  const onFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const content = await file.text();
    setText((prev) => (prev.trim() ? `${prev.replace(/\s+$/, "")}\n${content}` : content));
    e.target.value = "";
  };

  const reset = () => {
    setResults(null);
    setText("");
  };

  return (
    <Modal
      open
      onClose={onClose}
      title={`Bulk add API keys — ${provider.display_name}`}
      subtitle="Paste one key per line, or load a .txt/.csv file."
      maxWidth="max-w-2xl"
    >
      {results ? (
        <BulkResultsView
          results={results}
          onClose={onClose}
          onAgain={reset}
        />
      ) : (
        <div className="space-y-4 px-6 py-5">
          {/* Shared provider config applied to every key. */}
          {(showBaseURL || hasRegions || isCloudflare) && (
            <div className="space-y-3 rounded-xl border border-border bg-muted p-4">
              <p className="text-xs font-medium text-muted-foreground">Shared settings (applied to every key)</p>
              {hasRegions && (
                <Field label="Region">
                  <Select value={region} onValueChange={setRegion}>
                    <SelectTrigger className="h-9 w-full border-border bg-background text-xs">
                      <SelectValue placeholder="Select region" />
                    </SelectTrigger>
                    <SelectContent>
                      {(provider.regions ?? []).map((r) => (
                        <SelectItem key={r.id} value={r.id}>
                          {r.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>
              )}
              {isCloudflare && (
                <div className="space-y-3">
                  <div className="rounded-xl border border-border bg-popover p-3 space-y-2">
                    <p className="text-xs font-medium text-muted-foreground">Cloudflare Workers AI setup</p>
                    <ol className="space-y-1 text-[11px] text-muted-foreground">
                      <li className="flex gap-1.5">
                        <span className="font-medium text-foreground">1.</span>
                        <span>
                          Create an API token at{" "}
                          <a href="https://dash.cloudflare.com/profile/api-tokens" target="_blank" rel="noopener noreferrer" className="text-accent-600 hover:underline dark:text-accent-400">
                            dash.cloudflare.com
                          </a>{" "}
                          — use the <code className="rounded bg-muted px-1 py-0.5 text-[10px]">Workers AI</code> template
                        </span>
                      </li>
                      <li className="flex gap-1.5">
                        <span className="font-medium text-foreground">2.</span>
                        <span>
                          Copy your Account ID from the{" "}
                          <a href="https://dash.cloudflare.com" target="_blank" rel="noopener noreferrer" className="text-accent-600 hover:underline dark:text-accent-400">
                            Cloudflare dashboard
                          </a>{" "}
                          right sidebar
                        </span>
                      </li>
                    </ol>
                  </div>
                  <Field label="Account ID">
                    <Input value={accountID} onChange={(e) => setAccountID(e.target.value)} placeholder="e.g. a1b2c3d4e5f6..." required />
                  </Field>
                </div>
              )}
              {showBaseURL && (
                <Field label={requiresBaseURL ? "Base URL" : "Base URL (optional)"}>
                  <Input
                    value={baseURL}
                    onChange={(e) => setBaseURL(e.target.value)}
                    placeholder="for custom endpoints"
                    required={requiresBaseURL}
                  />
                </Field>
              )}
            </div>
          )}

          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <label className="text-sm font-medium">API keys</label>
              <Button
                variant="outline"
                size="xs"
                type="button"
                onClick={() => fileRef.current?.click()}
              >
                <FileText className="h-3.5 w-3.5" />
                Load file
              </Button>
              <input ref={fileRef} type="file" accept=".txt,.csv,text/plain,text/csv" className="hidden" onChange={onFile} />
            </div>
            <textarea
              value={text}
              onChange={(e) => setText(e.target.value)}
              rows={8}
              spellCheck={false}
              placeholder={`${keyPlaceholder}\nlabel-2, ${keyPlaceholder}\n# lines starting with # are comments`}
              className="w-full rounded-lg border border-border bg-popover px-3 py-2 font-mono text-xs placeholder:text-muted-foreground focus:border-accent-400 focus:outline-none"
            />
            <p className="text-[11px] leading-relaxed text-muted-foreground">
              One key per line. Optional inline label: <code className="font-mono">label,key</code>. Blank lines and{" "}
              <code className="font-mono">#</code> comments are ignored.
            </p>
          </div>

          {/* Live parse preview. */}
          {text.trim() && (
            <div className="flex flex-wrap items-center gap-2 text-xs">
              <Badge variant={validCount > 0 ? "success" : "neutral"}>{validCount} ready</Badge>
              {parsed.duplicates > 0 && <Badge variant="outline">{parsed.duplicates} duplicate</Badge>}
              {parsed.errors.length > 0 && <Badge variant="destructive">{parsed.errors.length} invalid</Badge>}
              {parsed.errors.slice(0, 3).map((err) => (
                <span key={err.line} className="text-muted-foreground">
                  line {err.line}: {err.message}
                </span>
              ))}
            </div>
          )}

          <label className="flex cursor-pointer items-start gap-2.5 rounded-xl border border-border bg-muted p-3">
            <input
              type="checkbox"
              checked={validate}
              onChange={(e) => setValidate(e.target.checked)}
              className="mt-0.5 h-3.5 w-3.5 rounded border-border accent-[var(--color-primary-container)]"
            />
            <span className="text-xs leading-relaxed text-muted-foreground">
              <span className="font-medium text-foreground">Validate each key against the upstream</span> before saving.
              Slower for large batches and may hit provider rate limits. Off by default.
            </span>
          </label>

          <div className="flex gap-3">
            <Button type="button" variant="ghost" onClick={onClose} className="flex-1">
              Cancel
            </Button>
            <Button type="button" onClick={() => importMut.mutate()} disabled={!canImport} className="flex-1">
              {importMut.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}
              {importMut.isPending ? "Importing…" : `Import ${validCount || ""}`.trim()}
            </Button>
          </div>
        </div>
      )}
    </Modal>
  );
}

// BulkResultsView renders the per-row outcome of a bulk import.
function BulkResultsView({
  results,
  onClose,
  onAgain,
}: {
  results: BulkAccountResult[];
  onClose: () => void;
  onAgain: () => void;
}) {
  const created = results.filter((r) => r.status === "created").length;
  const skipped = results.filter((r) => r.status === "skipped").length;
  const failed = results.filter((r) => r.status === "error").length;

  return (
    <div className="space-y-4 px-6 py-5">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant="success">{created} added</Badge>
        {skipped > 0 && <Badge variant="outline">{skipped} skipped</Badge>}
        {failed > 0 && <Badge variant="destructive">{failed} failed</Badge>}
      </div>
      <div className="max-h-72 divide-y divide-border overflow-y-auto rounded-xl border border-border">
        {results.map((r) => (
          <div key={r.index} className="flex items-center gap-3 px-3 py-2 text-xs">
            {r.status === "created" ? (
              <CheckCircle className="h-4 w-4 shrink-0 text-[var(--color-primary-container)]" />
            ) : r.status === "skipped" ? (
              <AlertCircle className="h-4 w-4 shrink-0 text-amber-500" />
            ) : (
              <XCircle className="h-4 w-4 shrink-0 text-red-500" />
            )}
            <span className="w-10 shrink-0 text-muted-foreground">#{r.index + 1}</span>
            <span className="flex-1 truncate font-medium">{r.label || "(unlabeled)"}</span>
            {r.error && <span className="truncate text-muted-foreground" title={r.error}>{r.error}</span>}
          </div>
        ))}
      </div>
      <div className="flex gap-3">
        <Button type="button" variant="ghost" onClick={onAgain} className="flex-1">
          <Layers className="h-4 w-4" />
          Import more
        </Button>
        <Button type="button" onClick={onClose} className="flex-1">
          <Check className="h-4 w-4" />
          Done
        </Button>
      </div>
    </div>
  );
}

function AddApiKeyModal({
  provider,
  hasRegions,
  label,
  apiKey,
  baseURL,
  region,
  accountID,
  azureEndpoint,
  azureDeployment,
  azureAPIVersion,
  azureOrganization,
  error,
  pending,
  onLabel,
  onApiKey,
  onBaseURL,
  onRegion,
  onAccountID,
  onAzureEndpoint,
  onAzureDeployment,
  onAzureAPIVersion,
  onAzureOrganization,
  onSubmit,
  onClose,
}: {
  provider: Provider;
  hasRegions: boolean;
  label: string;
  apiKey: string;
  baseURL: string;
  region: string;
  accountID: string;
  azureEndpoint: string;
  azureDeployment: string;
  azureAPIVersion: string;
  azureOrganization: string;
  error: string;
  pending: boolean;
  onLabel: (v: string) => void;
  onApiKey: (v: string) => void;
  onBaseURL: (v: string) => void;
  onRegion: (v: string) => void;
  onAccountID: (v: string) => void;
  onAzureEndpoint: (v: string) => void;
  onAzureDeployment: (v: string) => void;
  onAzureAPIVersion: (v: string) => void;
  onAzureOrganization: (v: string) => void;
  onSubmit: () => void;
  onClose: () => void;
}) {
  const [checkStatus, setCheckStatus] = useState<"idle" | "ok" | "error">("idle");
  const [checkMsg, setCheckMsg] = useState("");
  const [checking, setChecking] = useState(false);
  const supportsApiKey = provider.auth_modes.includes("api_key") || provider.auth_kind === "api_key";
  const supportsNone = provider.auth_modes.includes("none") || provider.auth_kind === "none";
  // Hide the API-key field only when API key auth is not supported at all.
  const isNoAuth = supportsNone && !supportsApiKey;
  // When both modes are offered, the API key is optional.
  const apiKeyOptional = supportsNone && supportsApiKey;
  const isAzure = provider.id === "azure";
  const isCloudflare = provider.id === "cloudflare-ai";
  const requiresBaseURL = provider.id === "custom-openai" || provider.id === "custom-anthropic";
  const credentialLabel = isNoAuth ? "Connection" : apiKeyOptional ? "API key (optional)" : "API key";
  const canSubmit =
    !pending &&
    (isNoAuth || apiKeyOptional || !!apiKey.trim()) &&
    (!isCloudflare || !!accountID.trim()) &&
    (!isAzure || (!!azureEndpoint.trim() && !!azureDeployment.trim())) &&
    (!requiresBaseURL || !!baseURL.trim());

  const handleCheck = async () => {
    if (!canSubmit && !isNoAuth) return;
    setChecking(true);
    setCheckStatus("idle");
    setCheckMsg("");
    try {
      const res = await api.validateKey({
        provider: provider.id,
        label,
        api_key: apiKey || undefined,
        base_url: baseURL || undefined,
        region: hasRegions ? region : undefined,
        account_id: accountID || undefined,
        azure_endpoint: azureEndpoint || undefined,
        azure_deployment: azureDeployment || undefined,
        azure_api_version: azureAPIVersion || undefined,
        azure_organization: azureOrganization || undefined,
      });
      setCheckStatus(res.status === "ok" ? "ok" : "error");
      setCheckMsg(res.message || "");
    } catch (e) {
      setCheckStatus("error");
      setCheckMsg((e as Error).message);
    } finally {
      setChecking(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        className="w-full max-w-md rounded-2xl border border-border bg-popover shadow-lg overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-border px-6 py-4">
          <h2 className="text-sm font-semibold">Add API key — {provider.display_name}</h2>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={onClose}
          >
            <X className="h-4 w-4" />
          </Button>
        </div>
        <form
          className="space-y-4 px-6 py-5"
          onSubmit={(e) => {
            e.preventDefault();
            if (canSubmit) onSubmit();
          }}
        >
          <Field label="Label">
            <Input value={label} onChange={(e) => onLabel(e.target.value)} placeholder="personal" />
          </Field>
          {!isNoAuth && (
            <Field label={credentialLabel}>
              <Input
                type="password"
                value={apiKey}
                onChange={(e) => { onApiKey(e.target.value); setCheckStatus("idle"); }}
                placeholder={isCloudflare ? "CF API token (v1.0-...)" : provider.id === "xai" ? "xai-..." : "sk-..."}
                required={!apiKeyOptional}
              />
            </Field>
          )}
          {isCloudflare && (
            <div className="space-y-3">
              <div className="rounded-xl border border-border bg-muted p-4 space-y-2">
                <p className="text-xs font-medium text-muted-foreground">Cloudflare Workers AI setup</p>
                <ol className="space-y-1.5 text-xs text-muted-foreground">
                  <li className="flex gap-1.5">
                    <span className="font-medium text-foreground">1.</span>
                    <span>
                      Create an API token at{" "}
                      <a href="https://dash.cloudflare.com/profile/api-tokens" target="_blank" rel="noopener noreferrer" className="text-accent-600 hover:underline dark:text-accent-400">
                        dash.cloudflare.com
                      </a>{" "}
                      — use the <code className="rounded bg-popover px-1 py-0.5 text-[10px]">Workers AI</code> template
                    </span>
                  </li>
                  <li className="flex gap-1.5">
                    <span className="font-medium text-foreground">2.</span>
                    <span>
                      Copy your Account ID from the{" "}
                      <a href="https://dash.cloudflare.com" target="_blank" rel="noopener noreferrer" className="text-accent-600 hover:underline dark:text-accent-400">
                        Cloudflare dashboard
                      </a>{" "}
                      right sidebar
                    </span>
                  </li>
                </ol>
              </div>
              <Field label="Account ID">
                <Input
                  value={accountID}
                  onChange={(e) => { onAccountID(e.target.value); setCheckStatus("idle"); }}
                  placeholder="e.g. a1b2c3d4e5f6..."
                  required
                />
              </Field>
            </div>
          )}
          {isAzure ? (
            <div className="space-y-3 rounded-xl border border-border bg-muted p-4">
              <Field label="Azure endpoint">
                <Input
                  value={azureEndpoint}
                  onChange={(e) => { onAzureEndpoint(e.target.value); setCheckStatus("idle"); }}
                  placeholder="https://your-resource.openai.azure.com"
                  required
                />
              </Field>
              <Field label="Deployment name">
                <Input
                  value={azureDeployment}
                  onChange={(e) => { onAzureDeployment(e.target.value); setCheckStatus("idle"); }}
                  placeholder="gpt-4o"
                  required
                />
              </Field>
              <Field label="API version">
                <Input
                  value={azureAPIVersion}
                  onChange={(e) => { onAzureAPIVersion(e.target.value); setCheckStatus("idle"); }}
                  placeholder="2024-10-01-preview"
                />
              </Field>
              <Field label="Organization (optional)">
                <Input
                  value={azureOrganization}
                  onChange={(e) => { onAzureOrganization(e.target.value); setCheckStatus("idle"); }}
                  placeholder="org_..."
                />
              </Field>
            </div>
          ) : hasRegions ? (
            <Field label="Region">
              <Select value={region} onValueChange={onRegion}>
                <SelectTrigger className="h-9 w-full border-border bg-background text-xs">
                  <SelectValue placeholder="Select region" />
                </SelectTrigger>
                <SelectContent>
                  {(provider.regions ?? []).map((r) => (
                    <SelectItem key={r.id} value={r.id}>
                      {r.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
          ) : (
            <Field label={requiresBaseURL ? "Base URL" : "Base URL (optional)"}>
              <Input
                value={baseURL}
                onChange={(e) => onBaseURL(e.target.value)}
                placeholder="for custom endpoints"
                required={requiresBaseURL}
              />
            </Field>
          )}

          {checkStatus === "ok" && (
            <div className="flex items-center gap-2 rounded-lg border border-accent-300 bg-accent-50 px-3 py-2 text-sm text-accent-700 dark:border-accent-700 dark:bg-accent-900/30 dark:text-accent-200">
              <CheckCircle className="h-4 w-4 shrink-0" />
              Key is valid
            </div>
          )}
          {checkStatus === "error" && (
            <ErrorBanner message={checkMsg || "Key validation failed"} />
          )}
          {error && <ErrorBanner message={error} />}

          <div className="flex gap-3">
            <Button type="button" variant="ghost" onClick={handleCheck} disabled={checking || !canSubmit} className="flex-1">
              <CheckCircle className={`h-4 w-4 ${checking ? "animate-pulse" : ""}`} />
              {checking ? "Checking…" : "Check"}
            </Button>
            <Button type="submit" disabled={!canSubmit} className="flex-1">
              <Plus className="h-4 w-4" />
              {pending ? "Adding…" : isNoAuth ? "Connect" : "Add account"}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}
// ---- Account quota card -----------------------------------------------------


// ModelCell renders a single model in a structural hairline grid.
function ModelCell({
  model,
  provider,
  disabled,
  selected,
  onToggleSelect,
  onToggleDisable,
}: {
  model: { id: string; name: string; kind: string };
  provider: Provider;
  disabled?: boolean;
  selected?: boolean;
  onToggleSelect?: () => void;
  onToggleDisable?: () => void;
}) {
  const [copied, setCopied] = useState(false);
  const fullModel = `${provider.alias || provider.id}/${model.id}`;

  const handleCopy = () => {
    navigator.clipboard.writeText(fullModel);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  return (
    <div className={`group relative flex flex-col justify-between p-4 transition-all ${disabled ? "bg-muted" : "bg-popover hover:bg-muted"} ${selected ? "ring-1 ring-inset ring-primary-container/60" : ""}`}>
      <div className="mb-3 flex items-start justify-between">
        <div className="flex items-center gap-2">
          {onToggleSelect && (
            <input
              type="checkbox"
              className="h-3.5 w-3.5 rounded border border-border bg-background accent-[var(--color-primary-container)]"
              checked={!!selected}
              onChange={onToggleSelect}
              title="Select model"
            />
          )}
          <div className={`h-1.5 w-1.5 rounded-full ${disabled ? "bg-ink-400 dark:bg-ink-600" : "bg-[var(--color-primary-container)] shadow-[0_0_8px_var(--color-primary-container)]"}`} />
          <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
            {model.kind || "Model"}
          </span>
        </div>
        <div className="flex items-center gap-0.5">
          {onToggleDisable && (
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={onToggleDisable}
              title={disabled ? "Enable model" : "Disable model"}
            >
              {disabled ? <ToggleLeft className="h-4 w-4" /> : <ToggleRight className="h-4 w-4 text-[var(--color-primary-container)]" />}
            </Button>
          )}
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={handleCopy}
            title="Copy model path"
          >
            {copied ? (
              <CheckCircle className="h-3.5 w-3.5 text-[var(--color-primary-container)]" />
            ) : (
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect width="14" height="14" x="8" y="8" rx="2" ry="2" /><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2" /></svg>
            )}
          </Button>
        </div>
      </div>
      <div>
        <code className="block truncate font-mono text-xs text-foreground tracking-tight" title={fullModel}>
          {fullModel}
        </code>
        {model.name && model.name !== model.id && (
          <span className="mt-1 block truncate text-[10px] text-muted-foreground" title={model.name}>
            {model.name}
          </span>
        )}
      </div>
    </div>
  );
}

// ProviderIcon renders the provider PNG with a colored fallback initial.
function ProviderIcon({ provider: p, size = 40 }: { provider: Provider; size?: number }) {
  const [errored, setErrored] = useState(false);
  const dim = { width: size, height: size };
  if (errored || !p.icon) {
    return (
      <div
        className="flex shrink-0 items-center justify-center rounded-2xl text-lg font-bold text-white"
        style={{ ...dim, backgroundColor: p.color || "var(--text-muted)" }}
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
      className="shrink-0 rounded-2xl object-contain"
      style={dim}
    />
  );
}
