import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus, Pencil, Trash2, Sparkles } from "lucide-react";

import { api, type CustomModel, type CustomModelInput, type Provider } from "../lib/api";
import { Card } from "@/components/ui/card";
import { CardHeader } from "@/components/composite/card-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect as Select, Field } from "@/components/composite/native-select";
import { Badge } from "@/components/ui/badge";
import { Modal } from "@/components/composite/modal";
import { EmptyState } from "@/components/composite/empty-state";
import { useToast } from "./Toast";

const MODEL_KINDS = ["llm", "embedding", "image", "stt", "tts", "search", "fetch"] as const;

// CustomModelsSection renders a provider's user-registered models with full
// add / edit / remove controls. It is intentionally separate from the
// catalog/discovered model list so the two never blur together.
export function CustomModelsSection({ provider }: { provider: Provider }) {
  const qc = useQueryClient();
  const toast = useToast();
  const providerId = provider.id;

  const customModels = useQuery({
    queryKey: ["custom-models", providerId],
    queryFn: () => api.listCustomModels(providerId),
    enabled: !!providerId,
  });

  const [modalOpen, setModalOpen] = useState(false);
  const [editing, setEditing] = useState<CustomModel | null>(null);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["custom-models", providerId] });
    // The provider model list merges custom models, so refresh it too.
    qc.invalidateQueries({ queryKey: ["provider-models", providerId] });
  };

  const createMut = useMutation({
    mutationFn: (input: CustomModelInput) => api.createCustomModel(providerId, input),
    onSuccess: () => {
      invalidate();
      setModalOpen(false);
      toast.success("Model added", "The custom model is now available for routing.");
    },
    onError: (e: Error) => toast.error("Couldn't add model", e.message),
  });

  const updateMut = useMutation({
    mutationFn: ({ dbId, patch }: { dbId: string; patch: Partial<CustomModelInput> }) =>
      api.updateCustomModel(providerId, dbId, patch),
    onSuccess: () => {
      invalidate();
      setModalOpen(false);
      setEditing(null);
      toast.success("Model updated", "Your changes were saved.");
    },
    onError: (e: Error) => toast.error("Couldn't update model", e.message),
  });

  const deleteMut = useMutation({
    mutationFn: (dbId: string) => api.deleteCustomModel(providerId, dbId),
    onSuccess: () => {
      invalidate();
      toast.success("Model removed", "The custom model was deleted.");
    },
    onError: (e: Error) => toast.error("Couldn't remove model", e.message),
  });

  const models = customModels.data?.models ?? [];

  const openAdd = () => {
    setEditing(null);
    setModalOpen(true);
  };
  const openEdit = (m: CustomModel) => {
    setEditing(m);
    setModalOpen(true);
  };

  return (
    <Card>
      <CardHeader
        title="Custom Models"
        description="Models you register yourself, beyond the predefined catalog. Useful for endpoints that don't expose a /models list."
        action={

          <Button variant="ghost" className="h-8 px-3 text-xs" onClick={openAdd}>
            <Plus className="h-3.5 w-3.5" />
            Add model
          </Button>
        }
      />

      {models.length === 0 ? (
        <div className="border-t border-border px-6 py-10">
          <EmptyState
            title="No custom models yet"
            hint="Add a model id (e.g. my-finetune-v1) to make it routable as ${alias}/<model>."
          />
        </div>
      ) : (
        <div className="divide-y divide-border border-t border-border">
          {models.map((m) => (
            <div key={m.db_id} className="flex items-center gap-3 px-6 py-3">
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-accent-100 text-accent-700 dark:bg-accent-800/40 dark:text-accent-200">
                <Sparkles className="h-4 w-4" />
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <code className="truncate font-mono text-xs text-foreground" title={`${provider.alias || provider.id}/${m.id}`}>
                    {provider.alias || provider.id}/{m.id}
                  </code>
                  <Badge variant="default">custom</Badge>
                  {m.kind && m.kind !== "llm" && <Badge variant="secondary">{m.kind}</Badge>}
                </div>
                <div className="mt-0.5 flex items-center gap-3 text-[10px] text-muted-foreground">
                  {m.name && m.name !== m.id && <span className="truncate">{m.name}</span>}
                  {m.context_window > 0 && <span>{m.context_window.toLocaleString()} ctx</span>}
                  {(m.input_per_m > 0 || m.output_per_m > 0) && (
                    <span>
                      ${m.input_per_m}/${m.output_per_m} per M
                    </span>
                  )}
                </div>
              </div>
              <button
                className="flex h-7 w-7 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-ink-100 hover:text-foreground dark:hover:bg-ink-800"
                title="Edit model"
                onClick={() => openEdit(m)}
              >
                <Pencil className="h-3.5 w-3.5" />
              </button>
              <button
                className="flex h-7 w-7 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-[color:var(--color-danger)]/10 hover:text-[color:var(--color-danger)]"
                title="Remove model"
                onClick={() => {
                  if (confirm(`Remove custom model "${m.id}"?`)) deleteMut.mutate(m.db_id);
                }}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </div>
          ))}
        </div>
      )}

      <CustomModelModal
        open={modalOpen}
        editing={editing}
        pending={createMut.isPending || updateMut.isPending}
        onClose={() => {
          setModalOpen(false);
          setEditing(null);
        }}
        onSubmit={(input) => {
          if (editing) {
            updateMut.mutate({ dbId: editing.db_id, patch: input });
          } else {
            createMut.mutate(input);
          }
        }}
      />
    </Card>
  );
}

function CustomModelModal({
  open,
  editing,
  pending,
  onClose,
  onSubmit,
}: {
  open: boolean;
  editing: CustomModel | null;
  pending: boolean;
  onClose: () => void;
  onSubmit: (input: CustomModelInput) => void;
}) {
  const [id, setId] = useState("");
  const [name, setName] = useState("");
  const [kind, setKind] = useState("llm");
  const [contextWindow, setContextWindow] = useState("");
  const [inputPerM, setInputPerM] = useState("");
  const [outputPerM, setOutputPerM] = useState("");

  // Sync form state whenever the modal opens or the editing target changes.
  const [syncedFor, setSyncedFor] = useState<string | null>(null);
  const syncKey = open ? editing?.db_id ?? "new" : "closed";
  if (open && syncedFor !== syncKey) {
    setSyncedFor(syncKey);
    setId(editing?.id ?? "");
    setName(editing?.name ?? "");
    setKind(editing?.kind || "llm");
    setContextWindow(editing?.context_window ? String(editing.context_window) : "");
    setInputPerM(editing?.input_per_m ? String(editing.input_per_m) : "");
    setOutputPerM(editing?.output_per_m ? String(editing.output_per_m) : "");
  } else if (!open && syncedFor !== "closed") {
    setSyncedFor("closed");
  }

  const canSubmit = id.trim().length > 0 && !pending;

  const submit = () => {
    if (!canSubmit) return;
    onSubmit({
      id: id.trim(),
      name: name.trim() || undefined,
      kind,
      context_window: contextWindow ? Number(contextWindow) : undefined,
      input_per_m: inputPerM ? Number(inputPerM) : undefined,
      output_per_m: outputPerM ? Number(outputPerM) : undefined,
    });
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={editing ? "Edit custom model" : "Add custom model"}
      subtitle="Register a model id the upstream accepts. Pricing and context are optional metadata."
    >
      <div className="space-y-4 px-6 py-5">
        <Field label="Model ID (required)">
          <Input
            value={id}
            onChange={(e) => setId(e.target.value)}
            placeholder="e.g. gpt-4o-mini or my-finetune-v1"
            autoFocus
          />
        </Field>
        <Field label="Display name">
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Friendly label (optional)" />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Kind">
            <Select value={kind} onChange={(e) => setKind(e.target.value)}>
              {MODEL_KINDS.map((k) => (
                <option key={k} value={k}>
                  {k}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Context window">
            <Input
              type="number"
              min={0}
              value={contextWindow}
              onChange={(e) => setContextWindow(e.target.value)}
              placeholder="e.g. 128000"
            />
          </Field>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Input $/M tokens">
            <Input
              type="number"
              min={0}
              step="0.01"
              value={inputPerM}
              onChange={(e) => setInputPerM(e.target.value)}
              placeholder="0"
            />
          </Field>
          <Field label="Output $/M tokens">
            <Input
              type="number"
              min={0}
              step="0.01"
              value={outputPerM}
              onChange={(e) => setOutputPerM(e.target.value)}
              placeholder="0"
            />
          </Field>
        </div>
      </div>
      <div className="flex items-center justify-end gap-2 border-t border-border px-6 py-4">
        <Button variant="ghost" onClick={onClose}>
          Cancel
        </Button>
        <Button onClick={submit} disabled={!canSubmit}>
          {editing ? "Save changes" : "Add model"}
        </Button>
      </div>
    </Modal>
  );
}
