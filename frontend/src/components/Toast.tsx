import { createContext, useContext, type ReactNode } from "react";
import { Toaster, toast as sonnerToast } from "sonner";
import { CheckCircle2, AlertCircle, Info, AlertTriangle } from "lucide-react";

type ToastTone = "success" | "error" | "info" | "warning";

interface ToastAPI {
  toast: (t: { tone?: ToastTone; title: string; description?: string }) => void;
  success: (title: string, description?: string) => void;
  error: (title: string, description?: string) => void;
  info: (title: string, description?: string) => void;
  warning: (title: string, description?: string) => void;
}

const ToastContext = createContext<ToastAPI | null>(null);

const defaultToastAPI: ToastAPI = {
  toast: ({ tone, title, description }) => {
    switch (tone) {
      case "success":
        sonnerToast.success(title, {
          description,
          icon: <CheckCircle2 className="size-4 text-emerald-400 shrink-0" />,
        });
        break;
      case "error":
        sonnerToast.error(title, {
          description,
          icon: <AlertCircle className="size-4 text-rose-500 shrink-0" />,
        });
        break;
      case "warning":
        sonnerToast.warning(title, {
          description,
          icon: <AlertTriangle className="size-4 text-amber-500 shrink-0" />,
        });
        break;
      default:
        sonnerToast.info(title, {
          description,
          icon: <Info className="size-4 text-sky-400 shrink-0" />,
        });
    }
  },
  success: (title, description) =>
    sonnerToast.success(title, {
      description,
      icon: <CheckCircle2 className="size-4 text-emerald-400 shrink-0" />,
    }),
  error: (title, description) =>
    sonnerToast.error(title, {
      description,
      icon: <AlertCircle className="size-4 text-rose-500 shrink-0" />,
    }),
  info: (title, description) =>
    sonnerToast.info(title, {
      description,
      icon: <Info className="size-4 text-sky-400 shrink-0" />,
    }),
  warning: (title, description) =>
    sonnerToast.warning(title, {
      description,
      icon: <AlertTriangle className="size-4 text-amber-500 shrink-0" />,
    }),
};

// useToast returns the imperative toast API.
export function useToast(): ToastAPI {
  const ctx = useContext(ToastContext);
  return ctx || defaultToastAPI;
}

export function ToastProvider({ children }: { children: ReactNode }) {
  return (
    <ToastContext.Provider value={defaultToastAPI}>
      {children}
      <Toaster
        position="top-right"
        offset="76px"
        theme="dark"
        closeButton
        duration={4000}
        toastOptions={{
          className:
            "group font-mono text-xs border border-border/80 bg-[#181717] text-foreground shadow-2xl rounded-xl p-3.5 gap-3",
          descriptionClassName: "text-muted-foreground text-[11px] font-mono leading-relaxed mt-0.5",
        }}
      />
    </ToastContext.Provider>
  );
}
