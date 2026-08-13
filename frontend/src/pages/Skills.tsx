import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Sparkles, Plus, Trash2, Copy, Check, BookOpen } from "lucide-react";
import { api, type Skill } from "../lib/api";
import { PageHeader } from "@/components/composite/page-header";
import { useToast } from "../components/Toast";
import { Card } from "@/components/ui/card";
import { SectionHeader } from "@/components/composite/section-header";
import { CardHeader } from "@/components/composite/card-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Spinner } from "@/components/ui/spinner";
import { EmptyState } from "@/components/composite/empty-state";
import { Switch } from "@/components/ui/switch";
import { ErrorBanner } from "@/components/composite/error-banner";
import { Field } from "@/components/composite/native-select";

// Built-in reference skills — documentation pages that teach AI agents how to
// call FlameGate endpoints. These mirror 9router's skills model: static docs
// with copyable URLs, not runtime request modifiers.
const REFERENCE_SKILLS = [
  { id: "flamegate", name: "FlameGate (Entry)", endpoint: null as string | null, description: "Setup guide and index of all capabilities." },
  { id: "flamegate-chat", name: "Chat", endpoint: "/v1/chat/completions", description: "Chat and code generation via OpenAI or Anthropic format with streaming." },
  { id: "flamegate-image", name: "Image Generation", endpoint: "/v1/images/generations", description: "Text-to-image via DALL-E, Imagen, FLUX, and more." },
  { id: "flamegate-tts", name: "Text-to-Speech", endpoint: "/v1/audio/speech", description: "OpenAI, ElevenLabs, Edge, Google, Deepgram voices." },
  { id: "flamegate-stt", name: "Speech-to-Text", endpoint: "/v1/audio/transcriptions", description: "Transcribe via Whisper, Groq, Gemini, Deepgram, AssemblyAI." },
  { id: "flamegate-embeddings", name: "Embeddings", endpoint: "/v1/embeddings", description: "Vectors for RAG and semantic search." },
  { id: "flamegate-web-search", name: "Web Search", endpoint: "/v1/search", description: "Tavily, Exa, Brave, Serper, SearXNG, Google PSE, You.com." },
  { id: "flamegate-web-fetch", name: "Web Fetch", endpoint: "/v1/web/fetch", description: "URL to markdown/text/HTML via Firecrawl, Jina, Tavily, Exa." },
];

export function SkillsPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const skills = useQuery({ queryKey: ["skills"], queryFn: () => api.listSkills() });

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [prompt, setPrompt] = useState("");
  const [error, setError] = useState("");

  const create = useMutation({
    mutationFn: () => api.createSkill({ name, description, prompt }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["skills"] });
      setName("");
      setDescription("");
      setPrompt("");
      setError("");
      toast.success("Skill created", `System-prompt augmentation "${name}" is now available for the gateway to apply.`);
    },
    onError: (e) => {
      setError((e as Error).message);
      toast.error("Skill creation failed", (e as Error).message);
    },
  });

  const toggle = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      api.updateSkill(id, { enabled }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["skills"] }),
    onError: (e) => toast.error("Skill toggle failed", (e as Error).message),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteSkill(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["skills"] });
      toast.success("Skill removed", "The system-prompt augmentation has been deleted and will no longer be applied.");
    },
    onError: (e) => toast.error("Skill removal failed", (e as Error).message),
  });

  return (
    <>
      <PageHeader
        title="Skills"
        icon={Sparkles}
        description="Reference docs for AI agents and reusable system-prompt augmentations."
      />

      <div className="space-y-6">
        <Card>
          <SectionHeader
            title="Reference skills"
            description="Copy a skill URL and paste it to your AI agent to teach it how to use FlameGate endpoints."
            icon={BookOpen}
          />
          <div className="divide-y divide-border">
            {REFERENCE_SKILLS.map((sk) => (
              <ReferenceSkillRow key={sk.id} skill={sk} />
            ))}
          </div>
        </Card>

        <Card>
          <SectionHeader
            title="Create skill"
            description="Give the skill a name and the instruction it should inject."
            icon={Plus}
          />
          <div className="space-y-4 border-t border-border px-6 py-5">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Name">
                <Input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="Concise reviewer"
                />
              </Field>
              <Field label="Description">
                <Input
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="Short summary of what it does"
                />
              </Field>
            </div>
            <Field label="Prompt">
              <Textarea
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                rows={3}
                placeholder="You are a meticulous code reviewer. Prefer small, safe diffs…"
              />
            </Field>
            {error && <ErrorBanner message={error} />}
            <div className="flex items-center gap-3">
              <Button size="sm" onClick={() => create.mutate()} disabled={!name.trim() || create.isPending}>
                <Plus className="h-3.5 w-3.5" />
                {create.isPending ? "Creating…" : "Create skill"}
              </Button>
            </div>
          </div>
        </Card>

        <Card>
          <CardHeader title="Custom skills" />
          {skills.isLoading ? (
            <Spinner />
          ) : !skills.data?.skills?.length ? (
            <EmptyState title="No custom skills yet" hint="Create a skill to augment matching requests with a system prompt." />
          ) : (
            <div className="divide-y divide-border">
              {skills.data.skills.map((sk) => (
                <SkillRow
                  key={sk.id}
                  skill={sk}
                  onToggle={(enabled) => toggle.mutate({ id: sk.id, enabled })}
                  onDelete={() => remove.mutate(sk.id)}
                />
              ))}
            </div>
          )}
        </Card>
      </div>
    </>
  );
}

function ReferenceSkillRow({ skill }: { skill: { id: string; name: string; endpoint: string | null; description: string } }) {
  const [copied, setCopied] = useState(false);
  const url = `https://raw.githubusercontent.com/bobbyunknown/flamegate/main/skills/${skill.id}/SKILL.md`;

  const copy = () => {
    navigator.clipboard.writeText(url);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="flex items-start justify-between gap-4 px-6 py-4">
      <div className="min-w-0">
        <span className="text-sm font-medium">{skill.name}</span>
        {skill.description && (
          <p className="mt-0.5 text-xs text-muted-foreground">{skill.description}</p>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-2">
        {skill.endpoint && (
          <span className="rounded bg-muted px-2 py-0.5 font-mono text-xs text-muted-foreground">
            {skill.endpoint}
          </span>
        )}
        <Button variant="ghost" size="icon-sm" onClick={copy} title="Copy skill URL">
          {copied ? <Check className="h-4 w-4 text-emerald-400" /> : <Copy className="h-4 w-4" />}
        </Button>
      </div>
    </div>
  );
}

function SkillRow({
  skill,
  onToggle,
  onDelete,
}: {
  skill: Skill;
  onToggle: (enabled: boolean) => void;
  onDelete: () => void;
}) {
  return (
    <div className="flex items-start justify-between gap-4 px-6 py-4">
      <div className="min-w-0">
        <span className="text-sm font-medium">{skill.name}</span>
        {skill.description && (
          <p className="mt-0.5 text-xs text-muted-foreground">{skill.description}</p>
        )}
        {skill.prompt && (
          <p className="mt-2 line-clamp-2 rounded-lg bg-muted px-3 py-2 font-mono text-xs text-muted-foreground">
            {skill.prompt}
          </p>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-3">
        <Switch checked={skill.enabled} onCheckedChange={onToggle} />
        <Button variant="destructiveGhost" size="icon-sm" onClick={onDelete} title="Delete skill">
          <Trash2 className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}