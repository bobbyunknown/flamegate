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
  iconTone?: "accent" | "warning" | "danger";
  label: string;
  value: string;
  delta?: { text: string; direction?: "up" | "down" | "flat" };
}) {
  const deltaColor =
    delta?.direction === "up"
      ? "text-primary"
      : delta?.direction === "down"
        ? "text-destructive"
        : "text-muted-foreground";
  const arrow = delta?.direction === "up" ? "↑" : delta?.direction === "down" ? "↓" : "";

  const tone = iconTone === "accent"
    ? { marker: "bg-primary", icon: "text-primary bg-primary/10" }
    : iconTone === "warning"
      ? { marker: "bg-amber-500", icon: "text-amber-500 bg-amber-500/10" }
      : { marker: "bg-destructive", icon: "text-destructive bg-destructive/10" };

  return (
    <Card className="bg-card hover:border-border/80 transition-all">
      <CardContent className="p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className={`inline-flex size-7 shrink-0 items-center justify-center rounded-md ${tone.icon}`}>
                <Icon className="size-3.5" strokeWidth={2} />
              </span>
              <p className="truncate text-[11px] font-semibold uppercase tracking-[0.18em] text-muted-foreground">
                {label}
              </p>
            </div>
            <p className="mt-3 text-3xl font-semibold tracking-tight tabular-nums text-foreground">
              {value}
            </p>
          </div>
          <span className={`h-9 w-1.5 shrink-0 rounded-full ${tone.marker}`} aria-hidden="true" />
        </div>
        {delta && (
          <p className={`mt-3 text-xs font-medium ${deltaColor}`}>
            {arrow} {delta.text}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
