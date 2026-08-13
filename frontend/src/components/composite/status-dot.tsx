export function StatusDot({
  tone = "success",
  label,
}: {
  tone?: "success" | "danger" | "warning" | "secondary";
  label?: string;
}) {
  const colors = {
    success: "bg-emerald-500",
    secondary: "bg-muted-foreground",
    danger: "bg-destructive",
    warning: "bg-amber-500",
  };
  return (
    <span
      className={`inline-block size-2 rounded-full ${colors[tone]}`}
      role="img"
      aria-label={label || tone}
    />
  );
}
