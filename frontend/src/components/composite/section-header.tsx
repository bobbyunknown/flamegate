import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";

export function SectionHeader({
  title,
  description,
  icon: Icon,
  iconTone = "accent",
  action,
}: {
  title: ReactNode;
  description?: string;
  icon?: LucideIcon;
  iconTone?: "accent" | "neutral" | "danger" | "secondary";
  action?: ReactNode;
}) {
  const toneClasses: Record<string, string> = {
    accent: "bg-primary/10 text-primary",
    neutral: "bg-muted text-muted-foreground",
    danger: "bg-destructive/10 text-destructive",
    secondary: "bg-secondary text-secondary-foreground",
  };
  return (
    <div className="flex items-start justify-between gap-4 px-6 pt-5 pb-4">
      <div className="flex items-start gap-3">
        {Icon && (
          <div className={`flex size-9 shrink-0 items-center justify-center rounded-lg ${toneClasses[iconTone]}`}>
            <Icon className="size-[18px]" strokeWidth={2} />
          </div>
        )}
        <div>
          <h2 className="text-base font-semibold tracking-tight text-foreground">{title}</h2>
          {description && <p className="mt-0.5 text-sm text-muted-foreground">{description}</p>}
        </div>
      </div>
      {action}
    </div>
  );
}
