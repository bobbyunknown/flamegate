import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  Settings as SettingsIcon,
  Cpu,
  Shield,
  Save,
} from "lucide-react";
import {
  api,
  type APIKey,
  type GuardrailPolicyConfig,
} from "../lib/api";
import { PageHeader } from "@/components/composite/page-header";
import { useToast } from "../components/Toast";
import { GuardrailEditor } from "../components/GuardrailEditor";
import { ModelMultiSelect } from "../components/ModelSelect";
import { Card } from "@/components/ui/card";
import { SectionHeader } from "@/components/composite/section-header";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { TabBar } from "@/components/composite/tab-bar";
import { Switch } from "@/components/ui/switch";
import { Spinner } from "@/components/ui/spinner";
import { EmptyState } from "@/components/composite/empty-state";

type Tab = "general" | "models" | "guardrails";

const TABS = [
  { value: "general" as const, label: "General", icon: SettingsIcon },
  { value: "models" as const, label: "Models", icon: Cpu },
  { value: "guardrails" as const, label: "Guardrails", icon: Shield },
];

function useHashTab(defaultTab: Tab): [Tab, (t: Tab) => void] {
  const valid: Tab[] = TABS.map((t) => t.value);
  const read = (): Tab => {
    const h = window.location.hash.replace("#", "");
    return valid.includes(h as Tab) ? (h as Tab) : defaultTab;
  };
  const [tab, setTab] = useState<Tab>(read);
  useEffect(() => {
    const onHash = () => setTab(read);
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);
  return [
    tab,
    (t: Tab) => {
      window.history.replaceState(null, "", `#${t}`);
      setTab(t);
    },
  ];
}

export function KeyDetailPage() {
  const { id } = useParams({ strict: false });
  const navigate = useNavigate();
  const [tab, setTab] = useHashTab("general");

  const keys = useQuery({ queryKey: ["keys"], queryFn: () => api.listKeys() });
  const key = useMemo(
    () => keys.data?.keys.find((k) => k.id === id) ?? null,
    [keys.data, id],
  );

  if (keys.isLoading) {
    return (
      <div className="py-12 flex justify-center">
        <Spinner />
      </div>
    );
  }
  if (!key) {
    return (
      <EmptyState
        title="API key not found"
        hint="The key may have been deleted. Go back to the API Keys page."
      />
    );
  }

  return (
    <div>
      <Button
        variant="ghost"
        size="sm"
        onClick={() => navigate({ to: "/keys" })}
        className="mb-2"
      >
        <ArrowLeft className="h-3 w-3 mr-1" /> All API keys
      </Button>
      <PageHeader
        title={key.name}
        description={`${key.display} · ${key.disabled ? "Disabled" : "Active"}`}
      />
      <div className="mb-4">
        <TabBar tabs={TABS} active={tab} onChange={setTab} />
      </div>

      {tab === "general" && <GeneralTab apiKey={key} />}
      {tab === "models" && <ModelsTab apiKey={key} />}
      {tab === "guardrails" && <GuardrailsTab apiKey={key} />}
    </div>
  );
}

function GeneralTab({ apiKey }: { apiKey: APIKey }) {
  const qc = useQueryClient();
  const toast = useToast();
  const toggle = useMutation({
    mutationFn: () => api.updateKey(apiKey.id, { disabled: !apiKey.disabled }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["keys"] });
      toast.success(apiKey.disabled ? "Key enabled" : "Key disabled");
    },
  });
  return (
    <Card>
      <SectionHeader title="General" description="Basic information about this API key." />
      <div className="px-6 py-5 space-y-4 text-sm">
        <Row label="Name" value={apiKey.name} />
        <Row label="Display" value={<code className="font-mono">{apiKey.display}</code>} />
        <Row label="ID" value={<code className="font-mono text-xs">{apiKey.id}</code>} />
        <Row
          label="Status"
          value={
            <div className="flex items-center gap-3">
              <Badge variant={apiKey.disabled ? "danger" : "success"}>
                {apiKey.disabled ? "Disabled" : "Active"}
              </Badge>
              <Button variant="secondary" onClick={() => toggle.mutate()}>
                {apiKey.disabled ? "Enable" : "Disable"}
              </Button>
            </div>
          }
        />
        {apiKey.plan_id && <Row label="Plan" value={<code className="font-mono">{apiKey.plan_id}</code>} />}
        <Row
          label="Created"
          value={apiKey.created_at ? new Date(apiKey.created_at).toLocaleString() : "—"}
        />
      </div>
    </Card>
  );
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="grid grid-cols-3 gap-3 items-center">
      <div className="text-muted-foreground">{label}</div>
      <div className="col-span-2">{value}</div>
    </div>
  );
}

function ModelsTab({ apiKey }: { apiKey: APIKey }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [models, setModels] = useState<string[]>(apiKey.allowed_models ?? []);

  const save = useMutation({
    mutationFn: () => api.updateKey(apiKey.id, { allowed_models: models }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["keys"] });
      toast.success("Models updated");
    },
    onError: (e) =>
      toast.error(e instanceof Error ? e.message : "Update failed"),
  });

  return (
    <Card>
      <SectionHeader
        title="Model Access"
        description="Restrict which models this key can call. Empty = inherit from plan or allow all."
      />
      <div className="px-6 py-5 space-y-4">
        <ModelMultiSelect value={models} onChange={setModels} />
        <div className="flex justify-end">
          <Button onClick={() => save.mutate()} disabled={save.isPending}>
            <Save className="h-4 w-4 mr-1" /> Save
          </Button>
        </div>
      </div>
    </Card>
  );
}

function GuardrailsTab({ apiKey }: { apiKey: APIKey }) {
  const qc = useQueryClient();
  const toast = useToast();
  // Per-key policy is stored under scope=apikey, scope_id=apiKey.id. We fetch
  // the whole list and pick out this key's row.
  const policies = useQuery({
    queryKey: ["guardrails", "apikey"],
    queryFn: () => api.listGuardrails("apikey"),
  });
  const effective = useQuery({
    queryKey: ["guardrails", "effective", apiKey.id],
    queryFn: () => api.effectiveGuardrail({ apikey: apiKey.id }),
  });

  const existing = policies.data?.guardrails.find((p) => p.scope_id === apiKey.id);

  const [config, setConfig] = useState<GuardrailPolicyConfig>(existing?.config ?? {});
  const [enabled, setEnabled] = useState<boolean>(existing?.enabled ?? true);

  // Re-sync local state whenever the row from the server changes.
  useEffect(() => {
    if (existing) {
      setConfig(existing.config ?? {});
      setEnabled(existing.enabled);
    }
  }, [existing?.id, existing?.updated_at]);

  const save = useMutation({
    mutationFn: () => {
      if (existing) {
        return api.updateGuardrail(existing.id, { enabled, config });
      }
      return api.createGuardrail({
        scope: "apikey",
        scope_id: apiKey.id,
        name: `Guardrails for ${apiKey.name}`,
        enabled,
        config,
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["guardrails"] });
      qc.invalidateQueries({ queryKey: ["guardrails", "effective", apiKey.id] });
      toast.success("Guardrails saved");
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : "Save failed"),
  });

  const remove = useMutation({
    mutationFn: () => api.deleteGuardrail(existing!.id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["guardrails"] });
      qc.invalidateQueries({ queryKey: ["guardrails", "effective", apiKey.id] });
      setConfig({});
      setEnabled(true);
      toast.success("Override removed — this key now inherits the upstream policy");
    },
  });

  return (
    <div className="space-y-4">
      <Card>
      <SectionHeader
          title="Per-key Override"
          description="When set, this configuration takes priority over global / provider / model / chain policies."
          action={
            <div className="flex items-center gap-3">
              <span className="text-xs">Override active</span>
              <Switch checked={enabled} onCheckedChange={setEnabled} />
              {existing && (
                <Button
                  variant="ghost"
                  onClick={() => {
                    if (!confirm("Remove the per-key override?")) return;
                    remove.mutate();
                  }}
                >
                  Remove
                </Button>
              )}
              <Button onClick={() => save.mutate()} disabled={save.isPending}>
                <Save className="h-4 w-4 mr-1" />
                {save.isPending ? "Saving..." : "Save"}
              </Button>
            </div>
          }
        />
      </Card>

      <GuardrailEditor value={config} onChange={setConfig} />

      <Card>
      <SectionHeader
          title="Effective Policy"
          description="Final merged policy after layering global → provider → model → chain → this API key."
        />
        <div className="px-6 py-4">
          {effective.isLoading ? (
            <Spinner />
          ) : (
            <pre className="text-[11px] font-mono whitespace-pre-wrap bg-black/5 dark:bg-white/5 rounded p-3 max-h-64 overflow-y-auto">
              {JSON.stringify(effective.data?.policy ?? {}, null, 2)}
            </pre>
          )}
        </div>
      </Card>
    </div>
  );
}
