import { useMemo } from "react";
import { useQuery, useQueries } from "@tanstack/react-query";
import { api, type GuardrailScope } from "../lib/api";
import { NativeSelect as Select, Field } from "@/components/composite/native-select";
import { Spinner } from "@/components/ui/spinner";

interface Props {
  scope: GuardrailScope;
  value: string;
  onChange: (id: string) => void;
}

// ScopeIDSelector renders a dropdown appropriate to the policy scope so users
// pick from known providers / models / chains / API keys instead of typing
// raw identifiers. For models we fan out one query per provider and flatten
// the results into a single grouped dropdown.
export function ScopeIDSelector({ scope, value, onChange }: Props) {
  if (scope === "global") return null;

  switch (scope) {
    case "provider":
      return <ProviderSelector value={value} onChange={onChange} />;
    case "model":
      return <ModelSelector value={value} onChange={onChange} />;
    case "chain":
      return <ChainSelector value={value} onChange={onChange} />;
    case "apikey":
      return <APIKeySelector value={value} onChange={onChange} />;
  }
}

function ProviderSelector({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const q = useQuery({
    queryKey: ["providers"],
    queryFn: () => api.providers(),
    staleTime: 60_000,
  });
  const options = q.data?.providers ?? [];
  return (
    <Field label="Provider">
      {q.isLoading ? (
        <Spinner />
      ) : (
        <Select value={value} onChange={(e) => onChange(e.target.value)}>
          <option value="">— select a provider —</option>
          {options.map((p) => (
            <option key={p.id} value={p.id}>
              {p.display_name} ({p.id})
            </option>
          ))}
        </Select>
      )}
    </Field>
  );
}

function ChainSelector({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const q = useQuery({
    queryKey: ["chains"],
    queryFn: () => api.listChains(),
    staleTime: 30_000,
  });
  const options = q.data?.chains ?? [];
  return (
    <Field label="Chain">
      {q.isLoading ? (
        <Spinner />
      ) : options.length === 0 ? (
        <div className="text-xs text-muted-foreground">
          No chains defined yet. Create a chain on the Chains page first.
        </div>
      ) : (
        <Select value={value} onChange={(e) => onChange(e.target.value)}>
          <option value="">— select a chain —</option>
          {options.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </Select>
      )}
    </Field>
  );
}

function APIKeySelector({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const q = useQuery({
    queryKey: ["keys"],
    queryFn: () => api.listKeys(),
    staleTime: 30_000,
  });
  const options = q.data?.keys ?? [];
  return (
    <Field label="API Key">
      {q.isLoading ? (
        <Spinner />
      ) : options.length === 0 ? (
        <div className="text-xs text-muted-foreground">
          No API keys yet. Create one on the API Keys page first.
        </div>
      ) : (
        <Select value={value} onChange={(e) => onChange(e.target.value)}>
          <option value="">— select an API key —</option>
          {options.map((k) => (
            <option key={k.id} value={k.id}>
              {k.name} — {k.display}
            </option>
          ))}
        </Select>
      )}
    </Field>
  );
}

// ModelSelector fans out per-provider queries (cheap, cached, 1 min staleTime)
// and flattens results into a single grouped <optgroup>-style dropdown. The
// scope_id we save is just the model id — that's what the engine compares
// against req.Model at request time.
function ModelSelector({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const providers = useQuery({
    queryKey: ["providers"],
    queryFn: () => api.providers(),
    staleTime: 60_000,
  });
  const provs = providers.data?.providers ?? [];
  const queries = useQueries({
    queries: provs.map((p) => ({
      queryKey: ["provider-models", p.id],
      queryFn: () => api.providerModels(p.id),
      enabled: !!p.id,
      staleTime: 60_000,
    })),
  });

  const grouped = useMemo(() => {
    return provs.map((p, i) => ({
      provider: p,
      models: queries[i]?.data?.models ?? [],
    }));
  }, [provs, queries]);

  const loading = providers.isLoading || queries.some((q) => q.isLoading);

  return (
    <Field label="Model">
      {loading ? (
        <Spinner />
      ) : (
        <Select value={value} onChange={(e) => onChange(e.target.value)}>
          <option value="">— select a model —</option>
          {grouped.map((g) =>
            g.models.length === 0 ? null : (
              <optgroup key={g.provider.id} label={g.provider.display_name}>
                {g.models.map((m) => (
                  <option key={`${g.provider.id}:${m.id}`} value={m.id}>
                    {m.id}
                  </option>
                ))}
              </optgroup>
            ),
          )}
        </Select>
      )}
    </Field>
  );
}
