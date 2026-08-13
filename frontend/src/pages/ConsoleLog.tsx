import { useEffect, useRef, useState, useMemo, useCallback, memo } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import {
  ScrollText,
  Trash2,
  Search,
  X,
  Pause,
  Play,
  Copy,
  Check,
  ChevronDown,
  ChevronRight,
} from "lucide-react";
import { PageHeader } from "@/components/composite/page-header";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { EmptyState } from "@/components/composite/empty-state";

// ── Types ────────────────────────────────────────────────────────────────────

import { BASE_URL, type ConsoleLogEntry } from "../lib/api"

type LogLevel = "DEBUG" | "INFO" | "WARN" | "ERROR" | "LOG";

// ── Constants ────────────────────────────────────────────────────────────────

// Badge text — high contrast on tinted backgrounds
const LEVEL_TEXT: Record<LogLevel, string> = {
  DEBUG: "text-purple-700 dark:text-purple-300",
  INFO: "text-blue-700 dark:text-blue-300",
  WARN: "text-amber-700 dark:text-amber-300",
  ERROR: "text-red-700 dark:text-red-300",
  LOG: "text-[color:var(--color-success)]",
};

// Badge background — visible but not loud
const LEVEL_BG: Record<LogLevel, string> = {
  DEBUG: "bg-purple-100 dark:bg-purple-500/20",
  INFO: "bg-blue-100 dark:bg-blue-500/20",
  WARN: "bg-amber-100 dark:bg-amber-500/20",
  ERROR: "bg-red-100 dark:bg-red-500/20",
  LOG: "bg-[color:var(--color-success)]/10",
};

// Left border accent for each row — quick visual scan of severity
const LEVEL_BORDER: Record<LogLevel, string> = {
  DEBUG: "border-l-purple-400 dark:border-l-purple-500",
  INFO: "border-l-blue-400 dark:border-l-blue-500",
  WARN: "border-l-amber-400 dark:border-l-amber-500",
  ERROR: "border-l-red-400 dark:border-l-red-500",
  LOG: "border-l-transparent",
};

const LEVELS: LogLevel[] = ["DEBUG", "INFO", "WARN", "ERROR"];

const MAX_LINES = 500;

const ROW_HEIGHT = 24; // approximate px per collapsed row

// Normalize an arbitrary server level string into a known LogLevel.
function normalizeLevel(level: string): LogLevel {
  const up = (level || "").toUpperCase();
  if (up === "DEBUG" || up === "INFO" || up === "WARN" || up === "ERROR") {
    return up;
  }
  return "LOG";
}

// ── Memoized log row ─────────────────────────────────────────────────────────

const LogRow = memo(function LogRow({
  entry,
  index,
  expanded,
  onToggle,
}: {
  entry: ConsoleLogEntry;
  index: number;
  expanded: boolean;
  onToggle: (seq: number) => void;
}) {
  const level = normalizeLevel(entry.level);
  const hasDetail = !!entry.detail && entry.detail.trim().length > 0;

  return (
    <div
      className={`border-b border-l-2 border-border/40 transition-colors ${LEVEL_BORDER[level]}`}
    >
      {/* Summary line */}
      <div
        className={`flex ${hasDetail ? "cursor-pointer" : ""} hover:bg-muted`}
        style={{ minHeight: ROW_HEIGHT }}
        onClick={hasDetail ? () => onToggle(entry.seq) : undefined}
      >
        {/* Line number */}
        <div className="w-[3.5rem] shrink-0 select-none px-3 py-[3px] text-right text-[11px] text-muted-foreground/50">
          {index + 1}
        </div>
        {/* Timestamp */}
        <div className="w-[6.5rem] shrink-0 px-2 py-[3px] whitespace-nowrap text-muted-foreground">
          {entry.time || "\u00A0"}
        </div>
        {/* Level badge */}
        <div className="w-[3.5rem] shrink-0 px-1 py-[3px]">
          {level !== "LOG" && (
            <Badge
              variant="neutral"
              className={`text-[11px] font-bold ${LEVEL_TEXT[level]} ${LEVEL_BG[level]}`}
            >
              {level}
            </Badge>
          )}
        </div>
        {/* Message */}
        <div className="min-w-0 flex-1 px-2 py-[3px] break-words text-foreground">
          <HighlightMessage message={entry.msg} />
        </div>
        {/* Detail toggle */}
        <div className="w-[5rem] shrink-0 px-2 py-[3px] text-right">
          {hasDetail && (
            <span className="inline-flex items-center gap-0.5 text-[11px] text-muted-foreground hover:text-foreground">
              <ChevronRight
                className={`h-3 w-3 transition-transform ${expanded ? "rotate-90" : ""}`}
              />
              detail
            </span>
          )}
        </div>
      </div>

      {/* Expanded detail block */}
      {hasDetail && expanded && (
        <div className="border-t border-border/30 bg-muted/60 py-2 pr-4 pl-[13.5rem]">
          <pre className="whitespace-pre-wrap break-words text-[12px] leading-[1.5] text-muted-foreground">
            {entry.detail}
          </pre>
        </div>
      )}
    </div>
  );
});

// ── Message highlighting ─────────────────────────────────────────────────────

// Highlights key=value pairs and important tokens in the message
const HighlightMessage = memo(function HighlightMessage({
  message,
}: {
  message: string;
}) {
  // Split on key=value patterns and highlight them
  const parts = message.split(/(\b\w+=\S+)/g);
  return (
    <>
      {parts.map((part, i) => {
        if (part.includes("=") && /^\w+=\S+$/.test(part)) {
          const eqIdx = part.indexOf("=");
          const key = part.slice(0, eqIdx);
          const val = part.slice(eqIdx + 1);
          return (
            <span key={i}>
              <span className="text-muted-foreground">{key}</span>
              <span className="text-muted-foreground/60">=</span>
              <span className="font-medium text-foreground">{val}</span>
            </span>
          );
        }
        return <span key={i}>{part}</span>;
      })}
    </>
  );
});

// ── Component ────────────────────────────────────────────────────────────────

export function ConsoleLogPage() {
  const [entries, setEntries] = useState<ConsoleLogEntry[]>([]);
  const [connected, setConnected] = useState(false);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [activeLevels, setActiveLevels] = useState<Set<LogLevel>>(
    new Set(LEVELS),
  );
  const [expanded, setExpanded] = useState<Set<number>>(new Set());
  const [autoScroll, setAutoScroll] = useState(true);
  const [copied, setCopied] = useState(false);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const isAutoScrolling = useRef(false);

  // ── SSE stream ───────────────────────────────────────────────────────────

  useEffect(() => {
    const es = new EventSource(`${BASE_URL}/api/console/stream`);

    es.onopen = () => setConnected(true);

    es.onmessage = (e) => {
      const msg = JSON.parse(e.data);
      if (msg.type === "init") {
        setEntries(msg.logs || []);
        setLoading(false);
      } else if (msg.type === "line") {
        setEntries((prev) => {
          const next = [...prev, msg.log as ConsoleLogEntry];
          return next.length > MAX_LINES ? next.slice(-MAX_LINES) : next;
        });
      } else if (msg.type === "clear") {
        setEntries([]);
        setExpanded(new Set());
      }
    };

    es.onerror = () => setConnected(false);

    return () => es.close();
  }, []);

  // ── Filtering ────────────────────────────────────────────────────────────

  const filtered = useMemo(() => {
    let list = entries;
    if (activeLevels.size < LEVELS.length) {
      list = list.filter((l) => {
        const lvl = normalizeLevel(l.level);
        return activeLevels.has(lvl) || lvl === "LOG";
      });
    }
    if (search) {
      const q = search.toLowerCase();
      list = list.filter(
        (l) =>
          l.msg.toLowerCase().includes(q) ||
          (l.detail || "").toLowerCase().includes(q),
      );
    }
    return list;
  }, [entries, activeLevels, search]);

  // ── Stats ────────────────────────────────────────────────────────────────

  const stats = useMemo(() => {
    const counts: Record<LogLevel, number> = {
      DEBUG: 0,
      INFO: 0,
      WARN: 0,
      ERROR: 0,
      LOG: 0,
    };
    for (const l of entries) counts[normalizeLevel(l.level)]++;
    return counts;
  }, [entries]);

  // ── Virtualizer ──────────────────────────────────────────────────────────

  const virtualizer = useVirtualizer({
    count: filtered.length,
    getScrollElement: () => scrollContainerRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 20,
  });

  // ── Auto-scroll ──────────────────────────────────────────────────────────

  // When filtered list grows and autoScroll is on, scroll to the end
  useEffect(() => {
    if (!autoScroll || filtered.length === 0) return;
    isAutoScrolling.current = true;
    virtualizer.scrollToIndex(filtered.length - 1, { align: "end" });
    // Small delay to let the scroll settle before re-enabling manual detection
    const t = setTimeout(() => {
      isAutoScrolling.current = false;
    }, 50);
    return () => clearTimeout(t);
  }, [filtered.length, autoScroll, virtualizer]);

  // Detect manual scroll-up to pause auto-scroll
  const handleScroll = useCallback(() => {
    if (isAutoScrolling.current) return;
    const el = scrollContainerRef.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
    setAutoScroll(atBottom);
  }, []);

  // ── Actions ──────────────────────────────────────────────────────────────

  const handleClear = async () => {
    try {
      await fetch(`${BASE_URL}/api/console`, { method: "DELETE", credentials: "include" });
    } catch {
      /* ignore */
    }
  };

  const handleCopy = async () => {
    const text = filtered
      .map((l) => {
        const head = `${l.time} ${normalizeLevel(l.level)} ${l.msg}`;
        if (l.detail && l.detail.trim()) {
          const indented = l.detail
            .split("\n")
            .map((line) => "    " + line)
            .join("\n");
          return `${head}\n${indented}`;
        }
        return head;
      })
      .join("\n");
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      /* ignore */
    }
  };

  const toggleExpand = useCallback((seq: number) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(seq)) {
        next.delete(seq);
      } else {
        next.add(seq);
      }
      return next;
    });
  }, []);

  const toggleLevel = (level: LogLevel) => {
    setActiveLevels((prev) => {
      const next = new Set(prev);
      if (next.has(level)) {
        next.delete(level);
      } else {
        next.add(level);
      }
      return next;
    });
  };

  const scrollToBottom = () => {
    setAutoScroll(true);
    if (filtered.length > 0) {
      virtualizer.scrollToIndex(filtered.length - 1, { align: "end" });
    }
  };

  // ── Render ───────────────────────────────────────────────────────────────

  return (
    <>
      <PageHeader
        title="Console Log"
        icon={ScrollText}
        description={
          connected
            ? `${entries.length} lines · live`
            : "Connecting…"
        }
        action={
          <div className="flex items-center gap-2">
            <span
              className={`h-2 w-2 rounded-full ${
                connected
                  ? "bg-[color:var(--color-success)]"
                  : "bg-muted-foreground"
              }`}
            />
            <Button variant="ghost" onClick={handleCopy} disabled={filtered.length === 0}>
              {copied ? (
                <Check className="h-4 w-4 text-accent-500" />
              ) : (
                <Copy className="h-4 w-4" />
              )}
              {copied ? "Copied" : "Copy"}
            </Button>
            <Button variant="ghost" onClick={handleClear} disabled={entries.length === 0}>
              <Trash2 className="h-4 w-4" />
              Clear
            </Button>
          </div>
        }
      />

      {/* ── Toolbar ──────────────────────────────────────────────────────── */}
      <Card className="mb-3">
        <div className="flex flex-wrap items-center gap-3 px-4 py-3">
          {/* Search */}
          <div className="relative min-w-0 flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search logs…"
              className="w-full pl-9 pr-8"
            />
            {search && (
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => setSearch("")}
                className="absolute right-2 top-1/2 -translate-y-1/2"
              >
                <X className="h-3.5 w-3.5" />
              </Button>
            )}
          </div>

          {/* Level filters */}
          <div className="flex items-center gap-1.5">
            {LEVELS.map((level) => {
              const active = activeLevels.has(level);
              const count = stats[level];
              return (
                <Button
                  key={level}
                  variant="ghost"
                  size="sm"
                  onClick={() => toggleLevel(level)}
                  className={`flex items-center gap-1 ${
                    active
                      ? `${LEVEL_BG[level]} ${LEVEL_TEXT[level]} ring-1 ring-current/20`
                      : "text-muted-foreground opacity-50 hover:opacity-75"
                  }`}
                >
                  {level}
                  {count > 0 && (
                    <span className="ml-0.5 tabular-nums opacity-70">
                      {count}
                    </span>
                  )}
                </Button>
              );
            })}
          </div>

          {/* Stats & scroll control */}
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            {search && (
              <span>
                {filtered.length}/{entries.length} matched
              </span>
            )}
            <Button
              variant={autoScroll ? "default" : "ghost"}
              size="sm"
              onClick={() => setAutoScroll(!autoScroll)}
              title={autoScroll ? "Pause auto-scroll" : "Resume auto-scroll"}
            >
              {autoScroll ? (
                <Pause className="h-3 w-3" />
              ) : (
                <Play className="h-3 w-3" />
              )}
              {autoScroll ? "Live" : "Paused"}
            </Button>
          </div>
        </div>
      </Card>

      {/* ── Log output ───────────────────────────────────────────────────── */}
      <Card className="relative overflow-hidden">
        {loading ? (
          <div className="flex items-center justify-center py-12">
            <div className="h-5 w-5 animate-spin rounded-full border-2 border-border border-t-accent-500" />
          </div>
        ) : filtered.length === 0 && entries.length === 0 ? (
          <EmptyState
            title="No console logs yet"
            hint="Requests will appear here once traffic flows through FlameGate."
          />
        ) : filtered.length === 0 ? (
          <EmptyState
            title="No matching logs"
            hint="Try adjusting your search or filters."
          />
        ) : (
          <div
            ref={scrollContainerRef}
            onScroll={handleScroll}
            className="overflow-y-auto bg-background font-mono text-[13px] leading-[1.6]"
            style={{ height: "calc(100vh - 310px)" }}
          >
            <div
              style={{
                height: `${virtualizer.getTotalSize()}px`,
                width: "100%",
                position: "relative",
              }}
            >
              {virtualizer.getVirtualItems().map((virtualRow) => {
                const entry = filtered[virtualRow.index];
                return (
                  <div
                    key={entry.seq}
                    data-index={virtualRow.index}
                    ref={virtualizer.measureElement}
                    style={{
                      position: "absolute",
                      top: 0,
                      left: 0,
                      width: "100%",
                      transform: `translateY(${virtualRow.start}px)`,
                    }}
                  >
                    <LogRow
                      entry={entry}
                      index={virtualRow.index}
                      expanded={expanded.has(entry.seq)}
                      onToggle={toggleExpand}
                    />
                  </div>
                );
              })}
            </div>
          </div>
        )}

        {/* Scroll-to-bottom fab */}
        {!autoScroll && filtered.length > 0 && (
          <div className="absolute bottom-4 right-4 z-10">
            <Button
              variant="default"
              size="icon"
              onClick={scrollToBottom}
              className="rounded-full shadow-lg"
            >
              <ChevronDown className="h-4 w-4" />
            </Button>
          </div>
        )}
      </Card>
    </>
  );
}