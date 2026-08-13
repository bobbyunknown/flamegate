import { useState, useEffect, useRef, useMemo, useCallback } from "react";
import { createPortal } from "react-dom";
import { useQuery, useQueries } from "@tanstack/react-query";
import { X, Search, ChevronDown, Check } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { api } from "../lib/api";

// ── Token Formatting ─────────────────────────────────────────────────

/** Format number with thousand separators: 1000000 → "1.000.000" */
export function formatTokenLimit(value: string): string {
  if (!value) return "";
  const n = parseInt(value.replace(/\D/g, ""), 10);
  if (isNaN(n)) return "";
  return n.toLocaleString("id-ID");
}

/** Text input that displays formatted token count continuously. */
export function FormattedTokenInput({
  value,
  onChange,
  placeholder,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
}) {
  const formatted = formatTokenLimit(value);

  return (
    <Input
      type="text"
      inputMode="numeric"
      value={formatted}
      onChange={(e) => {
        const raw = e.target.value.replace(/[^\d]/g, "");
        onChange(raw);
      }}
      placeholder={placeholder ? formatTokenLimit(placeholder) : undefined}
    />
  );
}

// ── Model Multi-Select ───────────────────────────────────────────────

interface ModelOption {
  id: string;
  name: string;
  providerId: string;
  providerName: string;
  icon: string;
}

/** Autocomplete multi-select for models, grouped by provider with logos. */
export function ModelMultiSelect({
  value,
  onChange,
}: {
  value: string[];
  onChange: (v: string[]) => void;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [customText, setCustomText] = useState("");
  const triggerRef = useRef<HTMLDivElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const [rect, setRect] = useState<DOMRect | null>(null);

  const providers = useQuery({ queryKey: ["providers"], queryFn: () => api.providers() });
  const visibleProviders = useMemo(
    () => (providers.data?.providers ?? []).filter((p) => !p.hidden),
    [providers.data],
  );

  const modelQueries = useQueries({
    queries: visibleProviders.map((p) => ({
      queryKey: ["providerModels", p.id],
      queryFn: () => api.providerModels(p.id),
      staleTime: 300_000,
    })),
  });

  const allModels = useMemo<ModelOption[]>(() => {
    const result: ModelOption[] = [];
    visibleProviders.forEach((p, i) => {
      const models = modelQueries[i]?.data?.models ?? [];
      models.forEach((m) => {
        result.push({
          id: m.id,
          name: m.name || m.id,
          providerId: p.id,
          providerName: p.display_name,
          icon: `/providers/${p.id}.png`,
        });
      });
    });
    return result;
  }, [visibleProviders, modelQueries]);

  const modelLookup = useMemo(() => {
    const map = new Map<string, ModelOption>();
    allModels.forEach((m) => map.set(m.id, m));
    return map;
  }, [allModels]);

  const filtered = useMemo(() => {
    if (!query.trim()) return allModels;
    const q = query.toLowerCase();
    return allModels.filter(
      (m) =>
        m.id.toLowerCase().includes(q) ||
        m.name.toLowerCase().includes(q) ||
        m.providerId.toLowerCase().includes(q) ||
        m.providerName.toLowerCase().includes(q),
    );
  }, [allModels, query]);

  const grouped = useMemo(() => {
    const map = new Map<string, { provider: string; providerName: string; icon: string; models: ModelOption[] }>();
    filtered.forEach((m) => {
      if (!map.has(m.providerId)) {
        map.set(m.providerId, { provider: m.providerId, providerName: m.providerName, icon: m.icon, models: [] });
      }
      map.get(m.providerId)!.models.push(m);
    });
    return Array.from(map.values());
  }, [filtered]);

  const updateRect = useCallback(() => {
    if (triggerRef.current) setRect(triggerRef.current.getBoundingClientRect());
  }, []);

  useEffect(() => {
    if (!open) return;
    updateRect();
    const onScroll = () => updateRect();
    const onResize = () => updateRect();
    window.addEventListener("scroll", onScroll, true);
    window.addEventListener("resize", onResize);
    return () => {
      window.removeEventListener("scroll", onScroll, true);
      window.removeEventListener("resize", onResize);
    };
  }, [open, updateRect]);

  useEffect(() => {
    if (!open) return;
    const handleClick = (e: MouseEvent) => {
      const target = e.target as globalThis.Node;
      if (triggerRef.current?.contains(target)) return;
      if (dropdownRef.current?.contains(target)) return;
      setOpen(false);
      setQuery("");
    };
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setOpen(false);
        setQuery("");
      }
    };
    document.addEventListener("mousedown", handleClick);
    document.addEventListener("keydown", handleKey);
    return () => {
      document.removeEventListener("mousedown", handleClick);
      document.removeEventListener("keydown", handleKey);
    };
  }, [open]);

  useEffect(() => {
    if (open && inputRef.current) inputRef.current.focus();
  }, [open]);

  const toggle = (id: string) => {
    onChange(value.includes(id) ? value.filter((v) => v !== id) : [...value, id]);
  };

  const removeChip = (id: string) => {
    onChange(value.filter((v) => v !== id));
  };

  const addCustom = () => {
    const t = customText.trim();
    if (t && !value.includes(t)) {
      onChange([...value, t]);
      setCustomText("");
    }
  };

  const anyLoading = modelQueries.some((q) => q.isLoading);

  const dropdown = open && rect
    ? createPortal(
        <div
          ref={dropdownRef}
          onMouseDown={(e) => e.stopPropagation()}
          className="fixed z-[100] overflow-hidden rounded-xl border border-border bg-popover text-popover-foreground shadow-2xl"
          style={{
            top: window.innerHeight - rect.bottom < 320
              ? Math.max(10, rect.top - 310)
              : rect.bottom + 4,
            left: rect.left,
            width: Math.max(rect.width, 360),
            maxHeight: 300
          }}
        >
          {/* Search */}
          <div className="border-b border-border p-2">
            <div className="relative flex items-center">
              <Search className="absolute left-2.5 h-3.5 w-3.5 shrink-0 text-muted-foreground pointer-events-none" />
              <Input
                ref={inputRef}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search models…"
                className="pl-8 text-xs h-8"
              />
            </div>
          </div>

          {/* Model list grouped by provider */}
          <div className="max-h-48 overflow-y-auto p-1">
            {anyLoading ? (
              <div className="flex items-center justify-center py-6">
                <div className="h-4 w-4 animate-spin rounded-full border-2 border-border border-t-primary" />
              </div>
            ) : grouped.length === 0 ? (
              <p className="px-3 py-2.5 text-center text-xs text-muted-foreground">No models found</p>
            ) : (
              grouped.map((g) => (
                <div key={g.provider}>
                  <div className="flex items-center gap-1.5 px-3 pt-2 pb-1">
                    <img
                      src={g.icon}
                      alt=""
                      className="h-3.5 w-3.5 shrink-0 rounded-sm object-contain"
                      onError={(e) => { (e.target as HTMLImageElement).style.display = "none"; }}
                    />
                    <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                      {g.providerName}
                    </span>
                  </div>
                  {g.models.map((m) => (
                    <button
                      key={m.id}
                      type="button"
                      onClick={() => toggle(m.id)}
                      className={`flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-left text-xs transition-colors hover:bg-muted ${
                        value.includes(m.id) ? "bg-primary/10 font-medium" : ""
                      }`}
                    >
                      <div
                        className={`flex h-4 w-4 shrink-0 items-center justify-center rounded border transition-colors ${
                          value.includes(m.id)
                            ? "border-primary bg-primary text-primary-foreground"
                            : "border-border"
                        }`}
                      >
                        {value.includes(m.id) && <Check className="h-3 w-3" />}
                      </div>
                      <span className="flex-1 truncate">{m.name}</span>
                      {m.id !== m.name && (
                        <span className="truncate text-[10px] text-muted-foreground">{m.id}</span>
                      )}
                    </button>
                  ))}
                </div>
              ))
            )}
          </div>

          {/* Custom pattern input */}
          <div className="border-t border-border p-2 bg-muted/30">
            <div className="flex items-center gap-2">
              <Input
                value={customText}
                onChange={(e) => setCustomText(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    addCustom();
                  }
                }}
                placeholder="Add pattern (e.g. claude-*)"
                className="flex-1 h-8 text-xs"
              />
              <Button
                type="button"
                size="sm"
                onClick={addCustom}
                disabled={!customText.trim()}
                className="h-8 text-xs px-3"
              >
                Add
              </Button>
            </div>
          </div>
        </div>,
        document.body,
      )
    : null;

  return (
    <div onClick={(e) => e.stopPropagation()} onMouseDown={(e) => e.stopPropagation()}>
      {/* Selected chips */}
      {value.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-1.5">
          {value.map((id) => {
            const m = modelLookup.get(id);
            return (
              <Badge
                key={id}
                variant="secondary"
                className="gap-1.5 py-1 px-2.5 font-normal text-xs"
              >
                {m && (
                  <img
                    src={m.icon}
                    alt=""
                    className="h-3.5 w-3.5 shrink-0 rounded-sm object-contain"
                    onError={(e) => { (e.target as HTMLImageElement).style.display = "none"; }}
                  />
                )}
                <span className="max-w-[180px] truncate">{id}</span>
                <button
                  type="button"
                  onClick={(e) => { e.stopPropagation(); removeChip(id); }}
                  className="ml-0.5 rounded-full p-0.5 hover:bg-foreground/10 transition-colors"
                >
                  <X className="h-3 w-3" />
                </button>
              </Badge>
            );
          })}
        </div>
      )}

      {/* Trigger button */}
      <div ref={triggerRef}>
        <button
          type="button"
          onMouseDown={(e) => {
            e.stopPropagation();
            setOpen(!open);
            setQuery("");
          }}
          className="flex w-full items-center gap-2 rounded-lg border border-border bg-background px-3 py-2 text-left text-sm transition-colors hover:border-border/80 focus:border-ring focus:outline-none focus-visible:ring-1 focus-visible:ring-ring cursor-pointer"
        >
          <Search className="h-4 w-4 shrink-0 text-muted-foreground" />
          <span className={`flex-1 text-xs ${value.length > 0 ? "text-foreground font-medium" : "text-muted-foreground"}`}>
            {value.length > 0
              ? `${value.length} model${value.length !== 1 ? "s" : ""} selected`
              : "Search and select models…"}
          </span>
          <ChevronDown
            className={`h-4 w-4 shrink-0 text-muted-foreground transition-transform ${open ? "rotate-180" : ""}`}
          />
        </button>
      </div>
      {dropdown}
    </div>
  );
}