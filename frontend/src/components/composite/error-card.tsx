import { AlertCircle } from "lucide-react";
import { Card } from "@/components/ui/card";

export function ErrorCard({ message }: { message: string }) {
  return (
    <Card className="glow-border flex flex-col items-center gap-2 px-6 py-12 text-center">
      <AlertCircle className="size-6 text-error" strokeWidth={2} />
      <p className="text-sm text-error">{message}</p>
    </Card>
  );
}
