import type { LucideIcon } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";

export function StatCard({
  icon: Icon,
  iconTone = "accent",
  label,
  value,
  delta,
}: {
  icon: LucideIcon;
  iconTone?: "accent" | "warning" | "danger" | "neutral";
  label: string;
  value: string;
  delta?: { text: string; direction?: "up" | "down" | "flat" };
}) {
  const deltaColor =
    delta?.direction === "up"
      ? "text-emerald-400 bg-emerald-500/10 border-emerald-500/20"
      : delta?.direction === "down"
        ? "text-rose-400 bg-rose-500/10 border-rose-500/20"
        : "text-muted-foreground bg-muted/40 border-border";
  const arrow = delta?.direction === "up" ? "↑" : delta?.direction === "down" ? "↓" : "";

  const toneClasses =
    iconTone === "accent"
      ? "text-primary bg-primary/10 border-primary/20"
      : iconTone === "warning"
        ? "text-amber-400 bg-amber-500/10 border-amber-500/20"
        : iconTone === "danger"
          ? "text-rose-400 bg-rose-500/10 border-rose-500/20"
          : "text-muted-foreground bg-muted/50 border-border";

  return (
    <Card className="group relative overflow-hidden bg-card/80 backdrop-blur-sm border-border/80 hover:border-primary/30 hover:bg-card transition-all duration-200 shadow-sm">
      <CardContent className="p-4 sm:p-5">
        <div className="flex items-center justify-between gap-2 mb-3">
          <div className="flex items-center gap-2.5 min-w-0">
            <span className={`inline-flex size-8 shrink-0 items-center justify-center rounded-lg border ${toneClasses} shadow-sm transition-transform duration-200 group-hover:scale-105`}>
              <Icon className="size-4" strokeWidth={2} />
            </span>
            <p className="truncate text-xs font-medium uppercase tracking-wider text-muted-foreground">
              {label}
            </p>
          </div>
          {delta && (
            <span className={`inline-flex items-center gap-0.5 rounded-md border px-1.5 py-0.5 text-[11px] font-medium ${deltaColor}`}>
              {arrow} {delta.text}
            </span>
          )}
        </div>
        <div className="flex items-baseline justify-between">
          <p className="text-2xl sm:text-3xl font-bold tracking-tight tabular-nums text-foreground">
            {value}
          </p>
        </div>
      </CardContent>
    </Card>
  );
}
