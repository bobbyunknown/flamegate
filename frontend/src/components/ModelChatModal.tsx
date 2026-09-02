import { useState, useRef, useEffect } from "react";
import {
  Send,
  Square,
  Trash2,
  Settings2,
  Sparkles,
  Bot,
  User,
  Check,
  Copy,
  ChevronDown,
  ChevronUp,
  Clock,
  Zap,
  Cpu,
} from "lucide-react";
import { Modal } from "@/components/composite/modal";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { streamChat, type TestChatMessage, type Provider, type ProviderModel } from "../lib/api";

interface Message {
  id: string;
  role: "user" | "assistant" | "system";
  content: string;
  thinking?: string;
  isStreaming?: boolean;
  error?: string;
}

interface ChatMetrics {
  latencyMs?: number;
  ttftMs?: number;
  promptTokens?: number;
  completionTokens?: number;
}

export function ModelChatModal({
  open,
  provider,
  model,
  onClose,
}: {
  open: boolean;
  provider: Provider;
  model: ProviderModel;
  onClose: () => void;
}) {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState("");
  const [systemPrompt, setSystemPrompt] = useState("");
  const [temperature, setTemperature] = useState(0.7);
  const [maxTokens, setMaxTokens] = useState(2048);
  const [showSettings, setShowSettings] = useState(false);
  const [showThinking, setShowThinking] = useState(true);
  const [isStreaming, setIsStreaming] = useState(false);
  const [metrics, setMetrics] = useState<ChatMetrics | null>(null);
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const abortControllerRef = useRef<AbortController | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const fullModelPath = `${provider.alias || provider.id}/${model.id}`;

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages, isStreaming]);

  useEffect(() => {
    if (open) {
      setTimeout(() => textareaRef.current?.focus(), 100);
    } else {
      handleStop();
    }
  }, [open]);

  const handleCopy = (id: string, text: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 1500);
  };

  const handleClear = () => {
    handleStop();
    setMessages([]);
    setMetrics(null);
  };

  const handleStop = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
    setIsStreaming(false);
    setMessages((prev) =>
      prev.map((m) => (m.isStreaming ? { ...m, isStreaming: false } : m))
    );
  };

  const handleSend = async (userText?: string) => {
    const text = (userText ?? input).trim();
    if (!text || isStreaming) return;

    setInput("");
    const userMsgId = `user-${Date.now()}`;
    const assistantMsgId = `asst-${Date.now()}`;

    const newMessages: Message[] = [
      ...messages,
      { id: userMsgId, role: "user", content: text },
      { id: assistantMsgId, role: "assistant", content: "", isStreaming: true },
    ];

    setMessages(newMessages);
    setIsStreaming(true);
    setMetrics(null);

    const abortController = new AbortController();
    abortControllerRef.current = abortController;

    const apiMessages: TestChatMessage[] = newMessages
      .filter((m) => m.id !== assistantMsgId && (m.role === "user" || m.role === "assistant"))
      .map((m) => ({ role: m.role, content: m.content }));

    let currentText = "";
    let currentThinking = "";

    await streamChat(
      {
        provider: provider.alias || provider.id,
        model: model.id,
        messages: apiMessages,
        system: systemPrompt.trim() || undefined,
        temperature,
        max_tokens: maxTokens,
      },
      {
        onDelta: (delta: string) => {
          currentText += delta;
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantMsgId ? { ...m, content: currentText } : m
            )
          );
        },
        onThinking: (delta: string) => {
          currentThinking += delta;
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantMsgId ? { ...m, thinking: currentThinking } : m
            )
          );
        },
        onDone: (info: { latency_ms: number; ttft_ms: number; prompt_tokens: number; completion_tokens: number; model: string }) => {
          setMetrics({
            latencyMs: info.latency_ms,
            ttftMs: info.ttft_ms,
            promptTokens: info.prompt_tokens,
            completionTokens: info.completion_tokens,
          });
          setIsStreaming(false);
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantMsgId ? { ...m, isStreaming: false } : m
            )
          );
          abortControllerRef.current = null;
        },
        onError: (err: string) => {
          setIsStreaming(false);
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantMsgId
                ? { ...m, isStreaming: false, error: err }
                : m
            )
          );
          abortControllerRef.current = null;
        },
      },
      abortController.signal
    );
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  const starterPrompts = [
    "Write a quick python function to parse JSON with error handling",
    "Explain how token streaming via SSE works in 2 sentences",
    "What are the benefits of using a unified AI Gateway router?",
  ];

  const tierBadgeClass =
    model.tier === "free"
      ? "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border-emerald-500/30"
      : model.tier === "pass"
      ? "bg-purple-500/15 text-purple-600 dark:text-purple-400 border-purple-500/30"
      : model.tier === "frontier"
      ? "bg-amber-500/15 text-amber-600 dark:text-amber-400 border-amber-500/30"
      : "bg-primary/15 text-primary border-primary/30";

  return (
    <Modal
      open={open}
      onClose={onClose}
      maxWidth="sm:max-w-4xl"
      title={
        <div className="flex items-center justify-between w-full pr-6">
          <div className="flex items-center gap-2.5 min-w-0">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary/10 text-primary border border-primary/20 shrink-0">
              <Bot className="h-4 w-4" />
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="font-semibold text-sm truncate text-foreground">
                  {model.name || model.id}
                </span>
                {model.tier && (
                  <Badge
                    variant="outline"
                    className={`text-[9px] uppercase px-1.5 py-0 font-bold border ${tierBadgeClass}`}
                  >
                    {model.tier}
                  </Badge>
                )}
                {model.context_window ? (
                  <span className="text-[10px] text-muted-foreground font-mono">
                    {model.context_window >= 1000000
                      ? `${(model.context_window / 1000000).toFixed(0)}M ctx`
                      : `${Math.round(model.context_window / 1000)}K ctx`}
                  </span>
                ) : null}
              </div>
              <code className="text-[10px] font-mono text-muted-foreground block truncate">
                {fullModelPath}
              </code>
            </div>
          </div>
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setShowSettings((v) => !v)}
              className={`h-7 px-2 text-xs gap-1.5 ${showSettings ? "bg-muted text-foreground" : "text-muted-foreground"}`}
              title="Chat Parameters"
            >
              <Settings2 className="h-3.5 w-3.5" />
              Settings
            </Button>
            {messages.length > 0 && (
              <Button
                variant="ghost"
                size="sm"
                onClick={handleClear}
                className="h-7 px-2 text-xs text-muted-foreground hover:text-red-500 gap-1.5"
                title="Clear conversation"
              >
                <Trash2 className="h-3.5 w-3.5" />
                Clear
              </Button>
            )}
          </div>
        </div>
      }
    >
      <div className="flex flex-col h-[70vh] -mx-6 -mb-6">
        {/* Collapsible Settings Drawer */}
        {showSettings && (
          <div className="border-b border-border bg-muted/40 px-6 py-3 space-y-3 transition-all">
            <div>
              <label className="text-xs font-medium text-foreground block mb-1">
                System Prompt
              </label>
              <Textarea
                value={systemPrompt}
                onChange={(e) => setSystemPrompt(e.target.value)}
                placeholder="You are a helpful and concise coding assistant..."
                className="h-16 text-xs bg-background resize-none"
              />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <div className="flex justify-between text-xs text-muted-foreground mb-1">
                  <span>Temperature</span>
                  <span className="font-mono">{temperature}</span>
                </div>
                <input
                  type="range"
                  min="0"
                  max="1"
                  step="0.05"
                  value={temperature}
                  onChange={(e) => setTemperature(parseFloat(e.target.value))}
                  className="w-full h-1.5 bg-border rounded-lg appearance-none cursor-pointer accent-primary"
                />
              </div>
              <div>
                <div className="flex justify-between text-xs text-muted-foreground mb-1">
                  <span>Max Tokens</span>
                  <span className="font-mono">{maxTokens}</span>
                </div>
                <Input
                  type="number"
                  min="1"
                  max="32768"
                  value={maxTokens}
                  onChange={(e) => setMaxTokens(parseInt(e.target.value) || 2048)}
                  className="h-7 text-xs bg-background"
                />
              </div>
            </div>
          </div>
        )}

        {/* Live Metrics Ribbon */}
        {metrics && (
          <div className="flex items-center justify-between border-b border-border bg-muted/20 px-6 py-1.5 text-[11px] text-muted-foreground font-mono">
            <div className="flex items-center gap-4">
              <span className="flex items-center gap-1">
                <Zap className="h-3 w-3 text-amber-500" />
                <span>Total: {metrics.latencyMs}ms</span>
              </span>
              {metrics.ttftMs !== undefined && metrics.ttftMs > 0 && (
                <span className="flex items-center gap-1">
                  <Clock className="h-3 w-3 text-cyan-500" />
                  <span>TTFT: {metrics.ttftMs}ms</span>
                </span>
              )}
              {metrics.promptTokens !== undefined && (
                <span className="flex items-center gap-1">
                  <Cpu className="h-3 w-3 text-indigo-500" />
                  <span>
                    Tokens: {metrics.promptTokens} in · {metrics.completionTokens} out
                  </span>
                </span>
              )}
            </div>
            <span className="text-emerald-500 flex items-center gap-1">
              <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 animate-pulse" />
              Connected
            </span>
          </div>
        )}

        {/* Message Stream Viewport */}
        <div className="flex-1 overflow-y-auto px-6 py-4 space-y-4">
          {messages.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-full text-center max-w-md mx-auto space-y-4">
              <div className="h-12 w-12 rounded-2xl bg-primary/10 flex items-center justify-center text-primary shadow-sm border border-primary/20">
                <Sparkles className="h-6 w-6" />
              </div>
              <div>
                <h3 className="text-sm font-semibold text-foreground">
                  Test Playground for {model.name || model.id}
                </h3>
                <p className="text-xs text-muted-foreground mt-1">
                  Send real-time test queries directly to this model via FlameGate's live streaming router.
                </p>
              </div>
              <div className="w-full space-y-1.5 pt-2 text-left">
                <p className="text-[11px] font-medium text-muted-foreground uppercase tracking-wider">
                  Quick Prompts
                </p>
                {starterPrompts.map((p, idx) => (
                  <button
                    key={idx}
                    type="button"
                    onClick={() => handleSend(p)}
                    className="w-full text-left text-xs px-3 py-2 rounded-lg bg-muted/60 hover:bg-muted text-foreground/80 hover:text-foreground border border-border/60 transition-colors flex items-center justify-between group"
                  >
                    <span className="truncate">{p}</span>
                    <Send className="h-3 w-3 opacity-0 group-hover:opacity-100 transition-opacity text-primary shrink-0 ml-2" />
                  </button>
                ))}
              </div>
            </div>
          ) : (
            messages.map((m) => (
              <div
                key={m.id}
                className={`flex gap-3 ${
                  m.role === "user" ? "justify-end" : "justify-start"
                }`}
              >
                {m.role === "assistant" && (
                  <div className="h-7 w-7 rounded-lg bg-primary/10 text-primary border border-primary/20 flex items-center justify-center shrink-0 mt-0.5">
                    <Bot className="h-4 w-4" />
                  </div>
                )}
                <div
                  className={`group relative max-w-[82%] rounded-2xl px-4 py-3 text-xs leading-relaxed ${
                    m.role === "user"
                      ? "bg-primary text-primary-foreground rounded-br-sm"
                      : "bg-muted/70 text-foreground border border-border/80 rounded-bl-sm"
                  }`}
                >
                  {/* Assistant Thinking Block */}
                  {m.thinking && (
                    <div className="mb-2.5 rounded-lg border border-purple-500/20 bg-purple-500/5 p-2 text-[11px]">
                      <button
                        type="button"
                        onClick={() => setShowThinking((v) => !v)}
                        className="flex items-center justify-between w-full font-medium text-purple-600 dark:text-purple-400 mb-1"
                      >
                        <span className="flex items-center gap-1.5">
                          <Sparkles className="h-3 w-3" />
                          Thinking Process
                        </span>
                        {showThinking ? (
                          <ChevronUp className="h-3 w-3" />
                        ) : (
                          <ChevronDown className="h-3 w-3" />
                        )}
                      </button>
                      {showThinking && (
                        <p className="whitespace-pre-wrap font-mono text-muted-foreground/80 pt-1 border-t border-purple-500/10 text-[10px]">
                          {m.thinking}
                        </p>
                      )}
                    </div>
                  )}

                  {/* Message Content */}
                  {m.content ? (
                    <div className="whitespace-pre-wrap break-words">
                      {m.content}
                      {m.isStreaming && (
                        <span className="inline-block w-1.5 h-3 ml-0.5 bg-primary animate-pulse" />
                      )}
                    </div>
                  ) : m.isStreaming ? (
                    <div className="flex items-center gap-1.5 text-muted-foreground py-0.5">
                      <span className="h-1.5 w-1.5 rounded-full bg-primary animate-bounce [animation-delay:-0.3s]" />
                      <span className="h-1.5 w-1.5 rounded-full bg-primary animate-bounce [animation-delay:-0.15s]" />
                      <span className="h-1.5 w-1.5 rounded-full bg-primary animate-bounce" />
                      <span className="text-[11px] ml-1">Thinking...</span>
                    </div>
                  ) : null}

                  {/* Error State */}
                  {m.error && (
                    <div className="mt-2 rounded-lg bg-red-500/10 border border-red-500/20 p-2 text-red-600 dark:text-red-400 text-[11px]">
                      <p className="font-semibold mb-0.5">Model Error:</p>
                      <p>{m.error}</p>
                    </div>
                  )}

                  {/* Quick Copy Button */}
                  {m.content && !m.isStreaming && (
                    <button
                      type="button"
                      onClick={() => handleCopy(m.id, m.content)}
                      className={`absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity p-1 rounded hover:bg-background/40 ${
                        m.role === "user" ? "text-primary-foreground" : "text-muted-foreground"
                      }`}
                      title="Copy response"
                    >
                      {copiedId === m.id ? (
                        <Check className="h-3.5 w-3.5 text-emerald-400" />
                      ) : (
                        <Copy className="h-3.5 w-3.5" />
                      )}
                    </button>
                  )}
                </div>
                {m.role === "user" && (
                  <div className="h-7 w-7 rounded-lg bg-muted text-muted-foreground border border-border flex items-center justify-center shrink-0 mt-0.5">
                    <User className="h-4 w-4" />
                  </div>
                )}
              </div>
            ))
          )}
          <div ref={messagesEndRef} />
        </div>

        {/* Input Bar */}
        <div className="border-t border-border bg-background p-4">
          <div className="relative flex items-end gap-2 rounded-xl border border-border bg-muted/30 p-2 focus-within:border-primary focus-within:ring-1 focus-within:ring-primary transition-all">
            <Textarea
              ref={textareaRef}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder={`Message ${model.name || model.id}... (Enter to send, Shift+Enter for newline)`}
              rows={1}
              className="min-h-[40px] max-h-32 flex-1 resize-none border-0 bg-transparent p-1.5 text-xs shadow-none focus-visible:ring-0"
              disabled={isStreaming}
            />
            {isStreaming ? (
              <Button
                type="button"
                variant="destructive"
                size="icon"
                onClick={handleStop}
                className="h-8 w-8 rounded-lg shrink-0"
                title="Stop generation"
              >
                <Square className="h-3.5 w-3.5 fill-current" />
              </Button>
            ) : (
              <Button
                type="button"
                size="icon"
                onClick={() => handleSend()}
                disabled={!input.trim()}
                className="h-8 w-8 rounded-lg shrink-0"
                title="Send message"
              >
                <Send className="h-3.5 w-3.5" />
              </Button>
            )}
          </div>
          <div className="flex items-center justify-between px-1 pt-2 text-[10px] text-muted-foreground">
            <span>Direct routing via FlameGate engine</span>
            <span className="font-mono">Shift + Enter for new line</span>
          </div>
        </div>
      </div>
    </Modal>
  );
}
