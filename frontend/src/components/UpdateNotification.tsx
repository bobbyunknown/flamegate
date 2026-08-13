import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Sparkles, X, ExternalLink, ArrowUpCircle, FileText } from "lucide-react";
import { Link } from "@tanstack/react-router";
import { api } from "../lib/api";
import { ChangelogMarkdown } from "./ChangelogMarkdown";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent } from "@/components/ui/dialog";

// useUpdateInfo is a shared hook so the TopBar badge and the Settings page
// read from the same cached query. The check hits GitHub at most every few
// hours (the backend caches), so a long stale time keeps it cheap.
export function useUpdateInfo() {
  return useQuery({
    queryKey: ["update-check"],
    queryFn: () => api.updateCheck(),
    staleTime: 1000 * 60 * 30, // 30 min — backend caches longer anyway
    refetchOnWindowFocus: false,
    retry: false,
  });
}

// UpdateNotification renders a small badge on the TopBar edge. It only appears
// when a newer release is available. Clicking it opens a popover with a short
// changelog preview and a link into the Settings page for the full changelog.
export function UpdateNotification() {
  const { data } = useUpdateInfo();
  const [open, setOpen] = useState(false);

  // Nothing to show unless GitHub reported a strictly newer version.
  if (!data || !data.update_available) return null;

  return (
    <div className="relative">
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="true"
        aria-expanded={open}
        aria-label={`Update available: ${data.latest}`}
        className="relative h-11 w-11 rounded-xl text-accent-600 dark:text-accent-300"
      >
        <ArrowUpCircle className="h-5 w-5" strokeWidth={2} />
        {/* Edge dot — signals an available update. */}
        <span className="absolute right-2 top-2 flex h-2.5 w-2.5">
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-accent-400 opacity-75" />
          <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-accent-500" />
        </span>
      </Button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent
          className="fixed right-4 top-20 z-50 flex max-h-[calc(100vh-6rem)] w-[min(34rem,calc(100vw-2rem))] flex-col overflow-hidden rounded-2xl border border-border bg-popover/95 p-0 shadow-[0_24px_70px_rgba(15,23,42,0.18)] backdrop-blur-xl dark:shadow-[0_24px_70px_rgba(0,0,0,0.45)]"
        >
          <div className="flex shrink-0 items-start justify-between gap-3 border-b border-border bg-popover/90 px-4 py-3.5">
            <div className="flex min-w-0 items-center gap-3">
              <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-accent-100 text-accent-700 shadow-inner dark:bg-accent-900/45 dark:text-accent-200">
                <Sparkles className="h-4 w-4" strokeWidth={2} />
              </div>
              <div className="min-w-0">
                <p className="text-sm font-semibold leading-tight tracking-[-0.01em]">Update available</p>
                <p className="mt-1 flex min-w-0 items-center gap-1.5 text-xs leading-tight text-muted-foreground">
                  <span className="truncate font-mono">{data.current}</span>
                  <span className="text-muted-foreground">→</span>
                  <span className="truncate font-mono font-semibold text-accent-600 dark:text-accent-400">{data.latest}</span>
                </p>
              </div>
            </div>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={() => setOpen(false)}
              aria-label="Dismiss"
              className="h-8 w-8"
            >
              <X className="h-4 w-4" />
            </Button>
          </div>

          {data.changelog && (
            <div className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden px-4 py-3 [scrollbar-gutter:stable]">
              <ChangelogMarkdown changelog={data.changelog} compact />
            </div>
          )}

          <div className="flex shrink-0 items-center justify-between gap-2 border-t border-border bg-popover/90 px-4 py-3">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setOpen(false)}
              render={(props) => (
                <Link to="/settings" {...props} className="flex items-center gap-1.5 bg-accent-50 text-accent-700 hover:bg-accent-100 dark:bg-accent-900/30 dark:text-accent-300 dark:hover:bg-accent-900/50" />
              )}
            >
              <FileText className="h-3.5 w-3.5" />
              View full changelog
            </Button>
            {data.html_url && (
              <a
                href={data.html_url}
                target="_blank"
                rel="noreferrer noopener"
                className="flex items-center gap-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-400/60"
              >
                Release notes
                <ExternalLink className="h-3 w-3" />
              </a>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
