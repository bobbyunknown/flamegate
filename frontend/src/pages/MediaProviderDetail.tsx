import { useEffect, useState, useMemo } from "react";
import { useParams, Link } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft, Plus, Trash2, KeyRound, Play, Copy, Check,
  Image, AudioLines, Mic, Search, Globe, Boxes, ExternalLink,
  ToggleLeft, ToggleRight, Loader2,
} from "lucide-react";
import { api, BASE_URL, type Provider, type Account } from "../lib/api"
import { useToast } from "../components/Toast";
import { Card } from "@/components/ui/card";
import { SectionHeader } from "@/components/composite/section-header";
import { CardHeader } from "@/components/composite/card-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Spinner } from "@/components/ui/spinner";
import { EmptyState } from "@/components/composite/empty-state";
import { NativeSelect as Select, Field } from "@/components/composite/native-select";

const kindMeta: Record<string, { label: string; icon: typeof Image; color: string }> = {
  embedding: { label: "Embeddings", icon: Boxes, color: "var(--color-brand-kilo)" },
  image: { label: "Image Generation", icon: Image, color: "var(--color-brand-openclaw)" },
  tts: { label: "Text-to-Speech", icon: AudioLines, color: "var(--color-brand-opencode)" },
  stt: { label: "Speech-to-Text", icon: Mic, color: "var(--color-brand-droid)" },
  search: { label: "Web Search", icon: Search, color: "var(--color-brand-cline)" },
  fetch: { label: "Web Fetch", icon: Globe, color: "var(--color-brand-copilot)" },
};

export function MediaProviderDetailPage() {
  const { kind, id } = useParams({ strict: false });
  const qc = useQueryClient();
  const toast = useToast();

  const providers = useQuery({ queryKey: ["providers"], queryFn: () => api.providers() });
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: () => api.listAccounts() });
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

  // Model search and pagination
  const [modelSearchQuery, setModelSearchQuery] = useState("");
  const [modelPage, setModelPage] = useState(1);
  const MODELS_PER_PAGE = 12;

  const filteredModels = useMemo(() => {
    if (!models.data?.models) return [];
    if (!modelSearchQuery.trim()) return models.data.models;
    const lowerQ = modelSearchQuery.toLowerCase();
    return models.data.models.filter(m => 
      m.id.toLowerCase().includes(lowerQ) || 
      (m.name && m.name.toLowerCase().includes(lowerQ))
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

  const create = useMutation({
    mutationFn: () => api.createAccount({
      provider: id!,
      label,
      api_key: apiKey || undefined,
      base_url: baseURL || undefined,
      region: provider?.regions?.length ? region || provider.default_region : undefined,
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
      toast.success("Account connected", "Upstream credentials saved and encrypted. The account is ready for routing.");
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

  const toggleAccount = useMutation({
    mutationFn: ({ accId, disabled }: { accId: string; disabled: boolean }) =>
      api.updateAccount(accId, { disabled }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["accounts"] }),
  });

  const meta = kindMeta[kind ?? ""] ?? kindMeta.embedding;
  const hasRegions = (provider?.regions?.length ?? 0) > 0;
  const isNoAuth = provider?.auth_kind === "none" || provider?.auth_modes.includes("none");
  const isAzure = provider?.id === "azure";
  const isCloudflare = provider?.id === "cloudflare-ai";
  const requiresBaseURL = provider?.id === "custom-openai" || provider?.id === "custom-anthropic";
  const canSubmit =
    !!provider &&
    (isNoAuth || !!apiKey.trim()) &&
    (!isCloudflare || !!accountID.trim()) &&
    (!isAzure || (!!azureEndpoint.trim() && !!azureDeployment.trim())) &&
    (!requiresBaseURL || !!baseURL.trim());

  if (providers.isLoading) return <Spinner />;

  if (!provider) {
    return (
      <div className="space-y-4">
        <Link to={`/media/${kind}` as any} className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
          <ArrowLeft className="h-4 w-4" /> Back to {meta.label}
        </Link>
        <EmptyState title="Provider not found" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <Link to={`/media/${kind}` as any} className="mb-3 flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
          <ArrowLeft className="h-4 w-4" /> Back to {meta.label}
        </Link>
        <div className="flex items-center gap-3">
          <ProviderIcon provider={provider} />
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-xl font-semibold">{provider.display_name}</h1>
              <Badge variant="default">{meta.label}</Badge>
              {!provider.drivable && <Badge variant="secondary">Coming soon</Badge>}
            </div>
            <p className="font-mono text-xs text-muted-foreground">{provider.id}</p>
          </div>
          {provider.api_key_url && (
            <a
              href={provider.api_key_url}
              target="_blank"
              rel="noopener"
              className="ml-auto flex items-center gap-1 text-xs text-accent-500 hover:underline"
            >
              Get API Key <ExternalLink className="h-3 w-3" />
            </a>
          )}
        </div>
      </div>

      {/* Accounts */}
      <Card>
        <SectionHeader title="Accounts" description="Provider credentials for this service." icon={KeyRound} />
        <div className="space-y-3 px-6 pb-6">
          {myAccounts.length > 0 && (
            <div className="space-y-2">
              {myAccounts.map((a) => (
                <AccountRow
                  key={a.id}
                  account={a}
                  onRemove={() => remove.mutate(a.id)}
                  onToggle={() => toggleAccount.mutate({ accId: a.id, disabled: !a.disabled })}
                />
              ))}
            </div>
          )}

          {/* Add account form */}
          {provider.drivable && (
            <form
              className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4"
              onSubmit={(e) => {
                e.preventDefault();
                if (canSubmit) create.mutate();
              }}
            >
              <Field label="Label (optional)">
                <Input value={label} onChange={(e) => setLabel(e.target.value)} placeholder="my-key" />
              </Field>
              {!isNoAuth && (
                <Field label="API Key">
                  <Input value={apiKey} onChange={(e) => setApiKey(e.target.value)} placeholder="sk-..." type="password" />
                </Field>
              )}
              {isCloudflare && (
                <Field label="Account ID">
                  <Input value={accountID} onChange={(e) => setAccountID(e.target.value)} placeholder="abc123def456..." />
                </Field>
              )}
              {isAzure ? (
                <>
                  <Field label="Azure endpoint">
                    <Input value={azureEndpoint} onChange={(e) => setAzureEndpoint(e.target.value)} placeholder="https://resource.openai.azure.com" />
                  </Field>
                  <Field label="Deployment">
                    <Input value={azureDeployment} onChange={(e) => setAzureDeployment(e.target.value)} placeholder="gpt-4o" />
                  </Field>
                  <Field label="API version">
                    <Input value={azureAPIVersion} onChange={(e) => setAzureAPIVersion(e.target.value)} placeholder="2024-10-01-preview" />
                  </Field>
                  <Field label="Organization">
                    <Input value={azureOrganization} onChange={(e) => setAzureOrganization(e.target.value)} placeholder="org_..." />
                  </Field>
                </>
              ) : hasRegions ? (
                <Field label="Region">
                  <select
                    value={region || provider.default_region || ""}
                    onChange={(e) => setRegion(e.target.value)}
                    className="w-full rounded-lg border border-input bg-popover px-3 py-2 text-sm focus:border-accent-400 focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/40"
                  >
                    {(provider.regions ?? []).map((r) => (
                      <option key={r.id} value={r.id}>{r.label}</option>
                    ))}
                  </select>
                </Field>
              ) : (
                <Field label={requiresBaseURL ? "Base URL" : "Base URL (optional)"}>
                  <Input value={baseURL} onChange={(e) => setBaseURL(e.target.value)} placeholder="for custom endpoints" />
                </Field>
              )}
              <Button type="submit" disabled={create.isPending || !canSubmit} className="self-end">
                <Plus className="h-4 w-4" />
                {create.isPending ? "Adding…" : isNoAuth ? "Connect" : "Add"}
              </Button>
            </form>
          )}
          {error && <p className="text-xs text-[color:var(--color-danger)]">{error}</p>}
        </div>
      </Card>

      {/* Models */}
      {kind !== "search" && kind !== "fetch" && models.data?.models && models.data.models.length > 0 && (
        <Card>
          <CardHeader title="Available Models" description={`${models.data.models.length} models`} />
          <div className="flex flex-col gap-3 border-t border-border bg-muted px-6 py-3 sm:flex-row sm:items-center">
            <div className="relative w-full max-w-sm">
              <Search className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search models..."
                value={modelSearchQuery}
                onChange={(e) => setModelSearchQuery(e.target.value)}
                className="pl-9 h-8 text-sm"
              />
            </div>
          </div>
          {filteredModels.length === 0 ? (
            <div className="px-6 py-12 text-center text-sm text-muted-foreground border-t border-border">
              No models found matching "{modelSearchQuery}"
            </div>
          ) : (
            <div className={`grid grid-cols-1 gap-px overflow-hidden border-t border-border bg-border sm:grid-cols-2 md:grid-cols-3 ${totalModelPages <= 1 ? "rounded-b-2xl" : ""}`}>
              {paginatedModels.map((m) => (
                <div key={m.id} className="flex flex-col justify-center gap-1 bg-popover px-4 py-2.5 transition-colors hover:bg-muted">
                  <span className="font-mono text-xs truncate" title={m.id}>{m.id}</span>
                  {m.name && m.name !== m.id && (
                    <span className="text-[10px] text-muted-foreground truncate" title={m.name}>{m.name}</span>
                  )}
                </div>
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

      {/* Test card */}
      {provider.drivable && myAccounts.length > 0 && (
        <TestCard kind={kind ?? "embedding"} provider={provider} models={models.data?.models ?? []} />
      )}
    </div>
  );
}

function AccountRow({ account: a, onRemove, onToggle }: { account: Account; onRemove: () => void; onToggle: () => void }) {
  return (
    <div className="flex items-center justify-between rounded-lg border border-border bg-muted px-4 py-2.5">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon-xs" onClick={onToggle}>
          {a.disabled ? <ToggleLeft className="h-5 w-5 text-muted-foreground" /> : <ToggleRight className="h-5 w-5 text-[color:var(--color-success)]" />}
        </Button>
        <div>
          <span className="text-sm font-medium">{a.label || a.provider}</span>
          {a.disabled && <span className="ml-2"><Badge variant="secondary">disabled</Badge></span>}
        </div>
      </div>
      <Button variant="ghost" onClick={onRemove} className="px-2">
        <Trash2 className="h-4 w-4 text-muted-foreground" />
      </Button>
    </div>
  );
}

function ProviderIcon({ provider: p }: { provider: Provider }) {
  const [errored, setErrored] = useState(false);
  if (errored || !p.icon) {
    return (
      <div
        className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl text-lg font-bold text-white"
        style={{ backgroundColor: p.color || "var(--color-on-surface-variant)" }}
      >
        {p.display_name.slice(0, 1).toUpperCase()}
      </div>
    );
  }
  return <img src={p.icon} alt={p.display_name} onError={() => setErrored(true)} className="h-12 w-12 shrink-0 rounded-xl object-contain" />;
}

// ─── Test Cards ──────────────────────────────────────────────────────────────

function TestCard({ kind, provider, models }: { kind: string; provider: Provider; models: { id: string }[] }) {
  switch (kind) {
    case "embedding":
      return <EmbeddingTestCard provider={provider} models={models} />;
    case "image":
      return <ImageTestCard provider={provider} models={models} />;
    case "tts":
      return <TtsTestCard provider={provider} models={models} />;
    case "stt":
      return <SttTestCard provider={provider} models={models} />;
    case "search":
      return <SearchTestCard provider={provider} />;
    case "fetch":
      return <FetchTestCard provider={provider} />;
    default:
      return null;
  }
}

function EmbeddingTestCard({ provider, models }: { provider: Provider; models: { id: string }[] }) {
  const [model, setModel] = useState(models[0]?.id ?? "");
  const [input, setInput] = useState("Hello world");
  const [result, setResult] = useState<any>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const run = async () => {
    setLoading(true);
    setError("");
    setResult(null);
    try {
      const resp = await fetch(`${BASE_URL}/v1/embeddings`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ model: `${provider.id}/${model}`, input }),
      });
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.error?.message || JSON.stringify(data));
      setResult(data);
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  const curlSnippet = `curl -X POST http://localhost:20180/v1/embeddings \\
  -H "Content-Type: application/json" \\
  -d '{"model":"${provider.id}/${model}","input":"${input}"}'`;

  return (
    <Card>
      <SectionHeader title="Test Embeddings" description="Send text and get embedding vectors." icon={Play} />
      <div className="space-y-3 px-6 pb-6">
        <div className="grid grid-cols-2 gap-3">
          <Field label="Model">
            {models.length > 1 ? (
              <Select value={model} onChange={(e) => setModel(e.target.value)}>
                {models.map((m) => <option key={m.id} value={m.id}>{m.id}</option>)}
              </Select>
            ) : (
              <Input value={model} onChange={(e) => setModel(e.target.value)} placeholder="model id" />
            )}
          </Field>
        </div>
        <Field label="Input text">
          <Input value={input} onChange={(e) => setInput(e.target.value)} placeholder="Text to embed" />
        </Field>
        <div className="flex items-center gap-2">
          <Button onClick={run} disabled={loading || !model}>
            {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
            {loading ? "Running…" : "Run"}
          </Button>
          <CopyButton text={curlSnippet} />
        </div>
        {error && <p className="text-xs text-[color:var(--color-danger)]">{error}</p>}
        {result && (
          <pre className="max-h-48 overflow-auto rounded-lg bg-muted p-3 text-xs">
            {JSON.stringify(result, null, 2).slice(0, 2000)}
          </pre>
        )}
      </div>
    </Card>
  );
}

function ImageTestCard({ provider, models }: { provider: Provider; models: { id: string }[] }) {
  const [model, setModel] = useState(models[0]?.id ?? "");
  const [prompt, setPrompt] = useState("A cute cat wearing a hat");
  const [size, setSize] = useState("1024x1024");
  const [result, setResult] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const run = async () => {
    setLoading(true);
    setError("");
    setResult(null);
    try {
      const resp = await fetch(`${BASE_URL}/v1/images/generations`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ model: `${provider.id}/${model}`, prompt, size }),
      });
      const contentType = resp.headers.get("content-type") || "";
      if (contentType.includes("image")) {
        const blob = await resp.blob();
        setResult(URL.createObjectURL(blob));
      } else {
        const data = await resp.json();
        if (!resp.ok) throw new Error(data.error?.message || JSON.stringify(data));
        if (data.data?.[0]?.b64_json) {
          setResult(`data:image/png;base64,${data.data[0].b64_json}`);
        } else if (data.data?.[0]?.url) {
          setResult(data.data[0].url);
        }
      }
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Card>
      <SectionHeader title="Test Image Generation" description="Generate images from text prompts." icon={Play} />
      <div className="space-y-3 px-6 pb-6">
        <div className="grid grid-cols-2 gap-3">
          <Field label="Model">
            {models.length > 1 ? (
              <Select value={model} onChange={(e) => setModel(e.target.value)}>
                {models.map((m) => <option key={m.id} value={m.id}>{m.id}</option>)}
              </Select>
            ) : (
              <Input value={model} onChange={(e) => setModel(e.target.value)} placeholder="model id" />
            )}
          </Field>
          <Field label="Size">
            <Select value={size} onChange={(e) => setSize(e.target.value)}>
              <option value="256x256">256×256</option>
              <option value="512x512">512×512</option>
              <option value="1024x1024">1024×1024</option>
              <option value="1792x1024">1792×1024</option>
              <option value="1024x1792">1024×1792</option>
            </Select>
          </Field>
        </div>
        <Field label="Prompt">
          <Input value={prompt} onChange={(e) => setPrompt(e.target.value)} placeholder="Describe the image" />
        </Field>
        <Button onClick={run} disabled={loading || !model}>
          {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
          {loading ? "Generating…" : "Generate"}
        </Button>
        {error && <p className="text-xs text-[color:var(--color-danger)]">{error}</p>}
        {result && (
          <div className="mt-2">
            <img src={result} alt="Generated" className="max-h-80 rounded-lg border border-border" />
          </div>
        )}
      </div>
    </Card>
  );
}

function TtsTestCard({ provider, models }: { provider: Provider; models: { id: string }[] }) {
  const [model, setModel] = useState(models[0]?.id ?? "");
  const [text, setText] = useState("Hello, this is a test of text to speech.");
  const [voice, setVoice] = useState("");
  const [audioUrl, setAudioUrl] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const run = async () => {
    setLoading(true);
    setError("");
    setAudioUrl(null);
    try {
      const body: any = { model: `${provider.id}/${model || provider.id}`, input: text };
      if (voice) body.voice = voice;
      const resp = await fetch(`${BASE_URL}/v1/audio/speech`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (!resp.ok) {
        const data = await resp.json().catch(() => ({}));
        throw new Error(data.error?.message || `HTTP ${resp.status}`);
      }
      const blob = await resp.blob();
      setAudioUrl(URL.createObjectURL(blob));
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Card>
      <SectionHeader title="Test Text-to-Speech" description="Convert text to audio." icon={Play} />
      <div className="space-y-3 px-6 pb-6">
        <div className="grid grid-cols-2 gap-3">
          <Field label="Model">
            <Input value={model} onChange={(e) => setModel(e.target.value)} placeholder={provider.id} />
          </Field>
          <Field label="Voice (optional)">
            <Input value={voice} onChange={(e) => setVoice(e.target.value)} placeholder="e.g. alloy" />
          </Field>
        </div>
        <Field label="Text">
          <textarea
            value={text}
            onChange={(e) => setText(e.target.value)}
            rows={3}
            className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/30"
          />
        </Field>
        <Button onClick={run} disabled={loading || !text}>
          {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
          {loading ? "Synthesizing…" : "Speak"}
        </Button>
        {error && <p className="text-xs text-[color:var(--color-danger)]">{error}</p>}
        {audioUrl && (
          <div className="mt-2 flex items-center gap-3">
            <audio controls src={audioUrl} className="flex-1" />
            <a href={audioUrl} download="speech.mp3" className="text-xs text-accent-500 hover:underline">Download</a>
          </div>
        )}
      </div>
    </Card>
  );
}

function SttTestCard({ provider, models }: { provider: Provider; models: { id: string }[] }) {
  const [model, setModel] = useState(models[0]?.id ?? "");
  const [file, setFile] = useState<File | null>(null);
  const [result, setResult] = useState<any>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const run = async () => {
    if (!file) return;
    setLoading(true);
    setError("");
    setResult(null);
    try {
      const form = new FormData();
      form.append("file", file);
      form.append("model", `${provider.id}/${model || provider.id}`);
      const resp = await fetch(`${BASE_URL}/v1/audio/transcriptions`, { method: "POST", body: form });
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.error?.message || JSON.stringify(data));
      setResult(data);
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Card>
      <SectionHeader title="Test Speech-to-Text" description="Transcribe audio files." icon={Play} />
      <div className="space-y-3 px-6 pb-6">
        <div className="grid grid-cols-2 gap-3">
          <Field label="Model">
            <Input value={model} onChange={(e) => setModel(e.target.value)} placeholder={provider.id} />
          </Field>
          <Field label="Audio file">
            <input
              type="file"
              accept="audio/*"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
              className="text-sm text-muted-foreground file:mr-2 file:rounded file:border-0 file:bg-muted file:px-2 file:py-1 file:text-xs"
            />
          </Field>
        </div>
        <Button onClick={run} disabled={loading || !file}>
          {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
          {loading ? "Transcribing…" : "Transcribe"}
        </Button>
        {error && <p className="text-xs text-[color:var(--color-danger)]">{error}</p>}
        {result && (
          <pre className="max-h-48 overflow-auto rounded-lg bg-muted p-3 text-xs">
            {JSON.stringify(result, null, 2)}
          </pre>
        )}
      </div>
    </Card>
  );
}

function SearchTestCard({ provider }: { provider: Provider }) {
  const [query, setQuery] = useState("What is the weather today?");
  const [result, setResult] = useState<any>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const run = async () => {
    setLoading(true);
    setError("");
    setResult(null);
    try {
      const resp = await fetch(`${BASE_URL}/v1/search`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ model: `${provider.id}/search`, query }),
      });
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.error?.message || JSON.stringify(data));
      setResult(data);
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Card>
      <SectionHeader title="Test Web Search" description="Search the web." icon={Play} />
      <div className="space-y-3 px-6 pb-6">
        <Field label="Query">
          <Input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search query" />
        </Field>
        <Button onClick={run} disabled={loading || !query}>
          {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
          {loading ? "Searching…" : "Search"}
        </Button>
        {error && <p className="text-xs text-[color:var(--color-danger)]">{error}</p>}
        {result && (
          <pre className="max-h-64 overflow-auto rounded-lg bg-muted p-3 text-xs">
            {JSON.stringify(result, null, 2).slice(0, 3000)}
          </pre>
        )}
      </div>
    </Card>
  );
}

function FetchTestCard({ provider }: { provider: Provider }) {
  const [url, setUrl] = useState("https://example.com");
  const [result, setResult] = useState<any>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const run = async () => {
    setLoading(true);
    setError("");
    setResult(null);
    try {
      const resp = await fetch(`${BASE_URL}/v1/web/fetch`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ model: `${provider.id}/fetch`, url }),
      });
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.error?.message || JSON.stringify(data));
      setResult(data);
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Card>
      <SectionHeader title="Test Web Fetch" description="Fetch and extract web page content." icon={Play} />
      <div className="space-y-3 px-6 pb-6">
        <Field label="URL">
          <Input value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://example.com" />
        </Field>
        <Button onClick={run} disabled={loading || !url}>
          {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
          {loading ? "Fetching…" : "Fetch"}
        </Button>
        {error && <p className="text-xs text-[color:var(--color-danger)]">{error}</p>}
        {result && (
          <pre className="max-h-64 overflow-auto rounded-lg bg-muted p-3 text-xs">
            {JSON.stringify(result, null, 2).slice(0, 3000)}
          </pre>
        )}
      </div>
    </Card>
  );
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  const copy = () => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };
  return (
    <Button variant="ghost" onClick={copy} className="px-2">
      {copied ? <Check className="h-4 w-4 text-[color:var(--color-success)]" /> : <Copy className="h-4 w-4" />}
    </Button>
  );
}
