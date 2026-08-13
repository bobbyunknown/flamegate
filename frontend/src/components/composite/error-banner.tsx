import { AlertCircle } from "lucide-react";

export function ErrorBanner({ message, className = "" }: { message: string; className?: string }) {
  return (
    <div
      role="alert"
      className={`flex items-start gap-2.5 border border-error/30 bg-error/10 px-3.5 py-2.5 ${className}`}
    >
      <AlertCircle className="mt-0.5 size-4 shrink-0 text-error" strokeWidth={2} />
      <p className="text-sm leading-snug break-words overflow-hidden text-error">{message}</p>
    </div>
  );
}
