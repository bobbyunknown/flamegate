import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams, useNavigate } from "@tanstack/react-router";
import { Image, AudioLines, Mic, Search, Globe, Boxes, ArrowRight } from "lucide-react";
import { api, type Provider } from "../lib/api";
import { PageHeader } from "@/components/composite/page-header";
import { Card } from "@/components/ui/card";
import { CardHeader } from "@/components/composite/card-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { EmptyState } from "@/components/composite/empty-state";

// Media service kinds — everything that isn't a plain chat/LLM provider.
const mediaKinds = [
  { id: "embedding", label: "Embeddings", icon: Boxes, description: "Text embedding models for search and RAG" },
  { id: "image", label: "Image", icon: Image, description: "Text-to-image generation providers" },
  { id: "tts", label: "Text-to-Speech", icon: AudioLines, description: "Voice synthesis providers" },
  { id: "stt", label: "Speech-to-Text", icon: Mic, description: "Audio transcription providers" },
  { id: "search", label: "Web Search", icon: Search, description: "Web search API providers" },
  { id: "fetch", label: "Web Fetch", icon: Globe, description: "Web page content extraction" },
];

const kindLabels: Record<string, string> = {
  embedding: "Embed",
  image: "Image",
  tts: "TTS",
  stt: "STT",
  search: "Search",
  fetch: "Fetch",
};

export function MediaProvidersPage() {
  const { kind: urlKind } = useParams({ strict: false });
  const navigate = useNavigate();
  const providers = useQuery({ queryKey: ["providers"], queryFn: () => api.providers() });
  const [filter, setFilter] = useState(urlKind || "embedding");

  // Sync filter with URL
  const activeFilter = urlKind || filter;
  const setActiveFilter = (k: string) => {
    setFilter(k);
    navigate({ to: `/media/${k}`, replace: true });
  };

  const list = useMemo(() => {
    const all = providers.data?.providers ?? [];
    return all
      .filter((p) => !p.hidden)
      .filter((p) => p.service_kinds.includes(activeFilter));
  }, [providers.data, activeFilter]);

  const activeKind = mediaKinds.find((k) => k.id === activeFilter) ?? mediaKinds[0];

  return (
    <>
      <PageHeader
        title="Media Providers"
        icon={Image}
        description="Connect providers for embeddings, image generation, speech, and web access."
      />

      {/* Kind tabs */}
      <div className="mb-5 flex flex-wrap gap-1.5">
        {mediaKinds.map((k) => (
          <Button
            key={k.id}
            variant={activeFilter === k.id ? "default" : "ghost"}
            size="sm"
            onClick={() => setActiveFilter(k.id)}
          >
            <k.icon className="h-3.5 w-3.5" />
            {k.label}
          </Button>
        ))}
      </div>

      {/* Kind description */}
      <div className="mb-4">
        <p className="text-sm text-muted-foreground">{activeKind.description}</p>
      </div>

      <Card>
        <CardHeader
          title={`${activeKind.label} providers`}
          description={`${list.length} available`}
        />
        {providers.isLoading ? (
          <Spinner />
        ) : !list.length ? (
          <EmptyState title="No providers for this capability" hint="Add a provider account first in the Providers page." />
        ) : (
          <div className="grid grid-cols-1 gap-px overflow-hidden rounded-b-2xl bg-border sm:grid-cols-2">
            {list.map((p) => (
              <MediaRow key={p.id} provider={p} kind={activeFilter} />
            ))}
          </div>
        )}
      </Card>
    </>
  );
}

function MediaRow({ provider: p, kind }: { provider: Provider; kind: string }) {
  const navigate = useNavigate();

  return (
    <button
      type="button"
      onClick={() => navigate({ to: `/media/${kind}/${p.id}` })}
      className="flex items-start gap-3 bg-popover px-5 py-4 text-left transition-colors hover:bg-ink-50 dark:hover:bg-ink-800/40"
    >
      <ProviderIcon provider={p} />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="text-sm font-medium">{p.display_name}</span>
          {!p.drivable && <Badge variant="secondary">soon</Badge>}
        </div>
        <p className="mt-0.5 font-mono text-xs text-muted-foreground">{p.id}</p>
        <div className="mt-2 flex flex-wrap gap-1">
          {p.service_kinds.filter((k) => k !== "llm").map((k) => (
            <Badge key={k} variant="default">
              {kindLabels[k] ?? k}
            </Badge>
          ))}
        </div>
      </div>
      <ArrowRight className="mt-3 h-4 w-4 shrink-0 text-muted-foreground" />
    </button>
  );
}

function ProviderIcon({ provider: p }: { provider: Provider }) {
  const [errored, setErrored] = useState(false);
  if (errored || !p.icon) {
    return (
      <div
        className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl text-sm font-bold text-white"
        style={{ backgroundColor: p.color || "var(--color-on-surface-variant)" }}
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
