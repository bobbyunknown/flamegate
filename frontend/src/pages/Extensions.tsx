import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Package, Plus, RefreshCw, Trash2, Power, Download } from "lucide-react";
import { api, type Extension, type StoreExtension } from "../lib/api";
import { PageHeader } from "@/components/composite/page-header";
import { useToast } from "../components/Toast";
import { Card } from "@/components/ui/card";
import { CardHeader } from "@/components/composite/card-header";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Spinner } from "@/components/ui/spinner";
import { EmptyState } from "@/components/composite/empty-state";
import { ErrorBanner } from "@/components/composite/error-banner";
import { Modal } from "@/components/composite/modal";
import { Field } from "@/components/composite/native-select";

export function ExtensionsPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const extensions = useQuery({ queryKey: ["extensions"], queryFn: () => api.listExtensions() });
  const store = useQuery({ queryKey: ["extension-store"], queryFn: () => api.listStoreExtensions() });
  const [installOpen, setInstallOpen] = useState(false);

  const srcInstall = useMutation({
    mutationFn: (source: string) => api.installRemoteExtension(source),
    onSuccess: (d) => {
      toast.success("Installed", `${d.slug}@${d.version} (${d.trust})`);
      invalidate();
      qc.invalidateQueries({ queryKey: ["extension-store"] });
    },
    onError: (e: Error) => toast.error("Install failed", e.message),
  });

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["extensions"] });
    qc.invalidateQueries({ queryKey: ["providers"] });
  };

  const enable = useMutation({
    mutationFn: (slug: string) => api.enableExtension(slug),
    onSuccess: (_d, slug) => {
      toast.success("Enabled", slug);
      invalidate();
    },
    onError: (e: Error) => toast.error("Enable failed", e.message),
  });

  const disable = useMutation({
    mutationFn: (slug: string) => api.disableExtension(slug),
    onSuccess: (_d, slug) => {
      toast.success("Disabled", slug);
      invalidate();
    },
    onError: (e: Error) => toast.error("Disable failed", e.message),
  });

  const sync = useMutation({
    mutationFn: (slug: string) => api.syncExtensionModels(slug),
    onSuccess: (d) => {
      toast.success("Models synced", `${d.synced} model(s) for ${d.slug}`);
      invalidate();
    },
    onError: (e: Error) => toast.error("Sync failed", e.message),
  });

  const setAutoSync = useMutation({
    mutationFn: ({ slug, on }: { slug: string; on: boolean }) =>
      api.setExtensionAutoSyncModels(slug, on),
    onSuccess: (d) => {
      toast.success(
        d.auto_sync_models ? "Auto-sync on" : "Auto-sync off",
        d.slug,
      );
      invalidate();
    },
    onError: (e: Error) => toast.error("Update auto-sync failed", e.message),
  });

  const uninstall = useMutation({
    mutationFn: (slug: string) => api.uninstallExtension(slug),
    onSuccess: (d) => {
      toast.success("Uninstalled", d.slug);
      invalidate();
    },
    onError: (e: Error) => toast.error("Uninstall failed", e.message),
  });
  const list = extensions.data?.extensions ?? [];

  return (
    <>
      <PageHeader
        title="Extensions"
        icon={Package}
        description="Install WASM provider modules. Active extensions appear in Providers."
        action={
          <Button onClick={() => setInstallOpen(true)}>
            <Plus className="h-4 w-4" />
            Install
          </Button>
        }
      />

      {extensions.isLoading ? (
        <Spinner />
      ) : (
        <Card>
          <CardHeader
            title="Installed"
            description="Enable to register the provider in the catalog; sync models after adding an account."
            action={<Badge variant="secondary">{list.length}</Badge>}
          />
          {!list.length ? (
            <EmptyState
              title="No extensions installed"
              hint="Install a .wasm + schema.json pair (e.g. xiaomi-mimo) to add non-native providers."
            />
          ) : (
            <ul className="divide-y divide-border">
              {list.map((ext) => (
                <ExtensionRow
                  key={ext.id}
                  ext={ext}
                  busy={
                    enable.isPending ||
                    disable.isPending ||
                    sync.isPending ||
                    setAutoSync.isPending ||
                    uninstall.isPending
                  }
                  onEnable={() => enable.mutate(ext.slug)}
                  onDisable={() => disable.mutate(ext.slug)}
                  onSync={() => sync.mutate(ext.slug)}
                  onToggleAutoSync={(on) => setAutoSync.mutate({ slug: ext.slug, on })}
                  onUninstall={() => {
                    if (window.confirm(`Uninstall ${ext.slug}? This removes models and unloads WASM.`)) {
                      uninstall.mutate(ext.slug);
                    }
                  }}
                />
              ))}
            </ul>
          )}
        </Card>
      )}

      <Store
        store={store}
        onInstallSource={(src) => srcInstall.mutate(src)}
        busy={srcInstall.isPending}
      />

      <InstallModal
        open={installOpen}
        onClose={() => setInstallOpen(false)}
        onInstalled={() => {
          invalidate();
          setInstallOpen(false);
        }}
      />
    </>
  );
}

function Store({
  store,
  onInstallSource,
  busy,
}: {
  store: { data?: { extensions: StoreExtension[] }; isLoading: boolean };
  onInstallSource: (source: string) => void;
  busy: boolean;
}) {
  const [url, setUrl] = useState("");
  const items = store.data?.extensions ?? [];
  const installBySlug = (slug: string) => onInstallSource(`store:${slug}`);
  return (
    <Card>
      <CardHeader
        title="Extension Store"
        description="Browse the catalog and install verified extensions from GitHub releases."
        action={<Badge variant="secondary">{items.length}</Badge>}
      />
      {store.isLoading ? (
        <Spinner />
      ) : (
        <div className="flex flex-col gap-2 p-5">
          <div className="flex items-center gap-2">
            <input
              className="flex h-9 w-full rounded border border-input bg-transparent px-3 py-1 text-sm shadow-sm"
              placeholder="Or install from a URL: https://…/ext.zip"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
            />
            <Button
              variant="outline"
              size="sm"
              disabled={busy || !url.trim()}
              onClick={() => {
                onInstallSource(`url:${url.trim()}`);
                setUrl("");
              }}
            >
              <Download className="h-3.5 w-3.5" />
              Install URL
            </Button>
          </div>
          {!items.length ? (
            <EmptyState
              title="Store empty"
              hint="No extensions in the catalog. Configure wasm.store_index_url to point at an index."
            />
          ) : (
            <ul className="divide-y divide-border">
              {items.map((it) => (
                <li
                  key={it.slug}
                  className="flex flex-col gap-3 px-1 py-3 sm:flex-row sm:items-center sm:justify-between"
                >
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="truncate text-sm font-semibold">{it.name}</p>
                      <Badge variant="secondary">store:{it.slug}</Badge>
                      {it.version ? (
                        <span className="font-mono text-xs text-muted-foreground">{it.version}</span>
                      ) : null}
                    </div>
                    {it.description ? (
                      <p className="mt-0.5 text-xs text-muted-foreground line-clamp-2">{it.description}</p>
                    ) : null}
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={busy}
                    onClick={() => installBySlug(it.slug)}
                  >
                    <Download className="h-3.5 w-3.5" />
                    Install
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </Card>
  );
}

function ExtensionRow({
  ext,
  busy,
  onEnable,
  onDisable,
  onSync,
  onToggleAutoSync,
  onUninstall,
}: {
  ext: Extension;
  busy: boolean;
  onEnable: () => void;
  onDisable: () => void;
  onSync: () => void;
  onToggleAutoSync: (on: boolean) => void;
  onUninstall: () => void;
}) {
  const active = ext.state === "ACTIVE";
  const autoSync = ext.auto_sync_models !== false;
  return (
    <li className="flex flex-col gap-3 px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <p className="truncate text-sm font-semibold">{ext.name || ext.slug}</p>
          <Badge variant={active ? "default" : "secondary"}>{ext.state}</Badge>
          {ext.version ? (
            <span className="font-mono text-xs text-muted-foreground">v{ext.version}</span>
          ) : null}
          <Badge variant={autoSync ? "default" : "secondary"}>
            auto-sync {autoSync ? "on" : "off"}
          </Badge>
        </div>
        <p className="mt-0.5 truncate font-mono text-xs text-muted-foreground">{ext.slug}</p>
        {ext.description ? (
          <p className="mt-1 text-xs text-muted-foreground line-clamp-2">{ext.description}</p>
        ) : null}
        <p className="mt-1 text-xs text-muted-foreground">
          {ext.model_count} model{ext.model_count === 1 ? "" : "s"}
          {ext.last_error ? ` · last error: ${ext.last_error}` : ""}
        </p>
      </div>
      <div className="flex flex-wrap items-center gap-2 shrink-0">
        {active ? (
          <Button variant="outline" size="sm" disabled={busy} onClick={onDisable}>
            <Power className="h-3.5 w-3.5" />
            Disable
          </Button>
        ) : (
          <Button variant="outline" size="sm" disabled={busy} onClick={onEnable}>
            <Power className="h-3.5 w-3.5" />
            Enable
          </Button>
        )}
        <Button
          variant="outline"
          size="sm"
          disabled={busy}
          onClick={() => onToggleAutoSync(!autoSync)}
          title="Install/enable will auto-run list_models when on"
        >
          Auto-sync: {autoSync ? "on" : "off"}
        </Button>
        <Button variant="outline" size="sm" disabled={busy || !active} onClick={onSync}>
          <RefreshCw className="h-3.5 w-3.5" />
          Sync models
        </Button>
        <Button variant="ghost" size="sm" disabled={busy} onClick={onUninstall}>
          <Trash2 className="h-3.5 w-3.5" />
          Uninstall
        </Button>
      </div>
    </li>
  );
}

function InstallModal({
  open,
  onClose,
  onInstalled,
}: {
  open: boolean;
  onClose: () => void;
  onInstalled: () => void;
}) {
  const toast = useToast();
  const wasmRef = useRef<HTMLInputElement>(null);
  const schemaRef = useRef<HTMLInputElement>(null);
  const [wasm, setWasm] = useState<File | null>(null);
  const [schema, setSchema] = useState<File | null>(null);
  const [error, setError] = useState("");

  const reset = () => {
    setWasm(null);
    setSchema(null);
    setError("");
    if (wasmRef.current) wasmRef.current.value = "";
    if (schemaRef.current) schemaRef.current.value = "";
  };

  const install = useMutation({
    mutationFn: () => {
      if (!wasm || !schema) throw new Error("wasm and schema files required");
      return api.installExtension(wasm, schema);
    },
    onSuccess: (ext) => {
      toast.success("Installed", ext.slug);
      reset();
      onInstalled();
    },
    onError: (e: Error) => setError(e.message),
  });

  const canSubmit = !!wasm && !!schema && !install.isPending;

  return (
    <Modal
      open={open}
      onClose={() => {
        reset();
        onClose();
      }}
      title="Install extension"
      subtitle="Multipart upload: .wasm binary + schema.json. Slug must not collide with native providers."
    >
      <form
        className="space-y-4 px-6 py-5"
        onSubmit={(e) => {
          e.preventDefault();
          if (canSubmit) install.mutate();
        }}
      >
        <Field label="WASM file (required)">
          <input
            ref={wasmRef}
            type="file"
            accept=".wasm,application/wasm"
            className="block w-full text-sm text-muted-foreground file:mr-2 file:rounded file:border-0 file:bg-muted file:px-2 file:py-1 file:text-xs"
            onChange={(e) => {
              setWasm(e.target.files?.[0] ?? null);
              setError("");
            }}
          />
        </Field>
        <Field label="schema.json (required)">
          <input
            ref={schemaRef}
            type="file"
            accept=".json,application/json"
            className="block w-full text-sm text-muted-foreground file:mr-2 file:rounded file:border-0 file:bg-muted file:px-2 file:py-1 file:text-xs"
            onChange={(e) => {
              setSchema(e.target.files?.[0] ?? null);
              setError("");
            }}
          />
        </Field>
        {error && <ErrorBanner message={error} />}
        <div className="flex items-center justify-end gap-2 pt-1">
          <Button
            type="button"
            variant="ghost"
            onClick={() => {
              reset();
              onClose();
            }}
          >
            Cancel
          </Button>
          <Button type="submit" disabled={!canSubmit}>
            <Plus className="h-4 w-4" />
            {install.isPending ? "Installing…" : "Install"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
