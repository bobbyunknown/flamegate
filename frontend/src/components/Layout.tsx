import { Suspense, useEffect, useState, lazy, type ReactNode } from "react";
import { Link, Outlet, useNavigate, useRouterState } from "@tanstack/react-router";
import { useQueryClient, useIsFetching, useQuery } from "@tanstack/react-query";
import {
  LayoutGrid,
  Boxes,
  Layers,
  Wallet,
  Sparkles,
  Settings,
  Search,
  ChevronDown,
  LogOut,
  Network,
  BarChart3,
  Clock,
  TerminalSquare,
  Image,
  Waypoints,
  ScrollText,
  Key,
  Activity,
  Shield,
  Flame,
  Package,
  type LucideIcon,
} from "lucide-react";
import { api } from "../lib/api";
import { useBranding } from "../contexts/BrandingContext";
import { UpdateNotification } from "./UpdateNotification";
import { Button } from "./ui/button";
import {
  SidebarProvider,
  Sidebar,
  SidebarHeader,
  SidebarContent as ShadcnSidebarContent,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarGroupContent,
  SidebarMenu,
  SidebarMenuItem,
  SidebarMenuButton,
  SidebarFooter,
  SidebarInset,
  SidebarTrigger,
  SidebarRail,
} from "./ui/sidebar";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "./ui/command";

const TanStackRouterDevtools =
  import.meta.env.MODE === "production"
    ? () => null
    : lazy(() =>
        import("@tanstack/react-router-devtools").then((m) => ({
          default: m.TanStackRouterDevtools,
        }))
      );

const ReactQueryDevtools =
  import.meta.env.MODE === "production"
    ? () => null
    : lazy(() =>
        import("@tanstack/react-query-devtools").then((m) => ({
          default: m.ReactQueryDevtools,
        }))
      );

interface NavItem {
  to: string;
  label: string;
  icon: LucideIcon;
  end?: boolean;
}

interface NavGroup {
  heading?: string;
  items: NavItem[];
}

function isNavActive(currentPath: string, target: string, exact?: boolean): boolean {
  if (exact) return currentPath === target;
  if (target === "/") return currentPath === "/";
  return currentPath === target || currentPath.startsWith(target + "/");
}

const navGroups: NavGroup[] = [
  {
    items: [{ to: "/", label: "Overview", icon: LayoutGrid, end: true }],
  },
  {
    heading: "Traffic & Logic",
    items: [
      { to: "/endpoints", label: "Endpoints", icon: Network },
      { to: "/chains", label: "Chains", icon: Layers },
      { to: "/skills", label: "Skills", icon: Sparkles },
    ],
  },
  {
    heading: "Connections",
    items: [
      { to: "/keys", label: "API Keys", icon: Key },
      { to: "/providers", label: "Providers", icon: Boxes },
      { to: "/extensions", label: "Extensions", icon: Package },
      { to: "/media", label: "Media", icon: Image },
      { to: "/proxy-pools", label: "Proxy Pools", icon: Waypoints },
    ],
  },
  {
    heading: "Safety",
    items: [{ to: "/guardrails", label: "Guardrails", icon: Shield }],
  },
  {
    heading: "Cost & Analytics",
    items: [
      { to: "/usage", label: "Usage", icon: BarChart3 },
      { to: "/plans", label: "Plans", icon: Wallet },
      { to: "/quota", label: "Quota Tracker", icon: Clock },
      { to: "/system", label: "System", icon: Activity },
      { to: "/settings", label: "Settings", icon: Settings },
    ],
  },
  {
    heading: "Developer",
    items: [
      { to: "/console", label: "Console Log", icon: ScrollText },
      { to: "/cli-tools", label: "CLI Tools", icon: TerminalSquare },
    ],
  },
];

const TITLE_BY_PATH: Record<string, string> = {
  "/": "Overview",
  "/endpoints": "Endpoints",
  "/chains": "Chains",
  "/skills": "Skills",
  "/providers": "Providers",
  "/extensions": "Extensions",
  "/media": "Media",
  "/proxy-pools": "Proxy Pools",
  "/usage": "Usage",
  "/plans": "Plans",
  "/budgets": "Plans",
  "/quota": "Quota Tracker",
  "/settings": "Settings",
  "/keys": "API Keys",
  "/guardrails": "Guardrails",
  "/console": "Console Log",
  "/cli-tools": "CLI Tools",
  "/system": "System",
};

const TITLE_BY_PREFIX: [string, string][] = [
  ["/providers/", "Provider"],
  ["/cli-tools/", "CLI Tool"],
  ["/media/", "Media"],
  ["/keys/", "API Key"],
];

function titleForPath(pathname: string): string {
  const exact = TITLE_BY_PATH[pathname];
  if (exact) return exact;
  for (const [prefix, label] of TITLE_BY_PREFIX) {
    if (pathname.startsWith(prefix)) return label;
  }
  return "";
}

export function Layout() {
  const { location } = useRouterState();
  const { branding } = useBranding();

  useEffect(() => {
    const label = titleForPath(location.pathname);
    const appName = branding.name || "FlameGate";
    document.title = label ? `${appName} - ${label}` : appName;
  }, [location.pathname, branding.name]);

  return (
    <SidebarProvider defaultOpen={true}>
      <AppSidebar />
      <SidebarInset>
        <RouteProgress />
        <AppHeader />
        <div className="flex-1 overflow-y-auto">
          <div className="px-6 py-6 sm:px-8 sm:py-8 lg:px-10">
            <Suspense fallback={<PageOutletFallback />}>
              <Outlet />
            </Suspense>
          </div>
        </div>
        <Suspense>
          <TanStackRouterDevtools />
          <ReactQueryDevtools />
        </Suspense>
      </SidebarInset>
    </SidebarProvider>
  );
}

function AppSidebar() {
  const { location } = useRouterState();
  return (
    <Sidebar
      collapsible="icon"
      side="left"
    >
      <SidebarHeader>
        <div className="flex items-center gap-2.5 px-1 py-2 group-data-[collapsible=icon]:justify-center">
          <Flame className="size-6 shrink-0 text-primary-container neon-pulse" />
          <div className="flex flex-col group-data-[collapsible=icon]:hidden">
            <span className="font-bold text-sm tracking-tight text-primary-container">
              FlameGate
            </span>
            <span className="text-[10px] text-on-surface-variant/60 uppercase tracking-[0.08em]">
              AI Gateway
            </span>
          </div>
        </div>
      </SidebarHeader>
      <ShadcnSidebarContent>
        {navGroups.map((group, gi) => (
          <SidebarGroup key={gi}>
            {group.heading && (
              <SidebarGroupLabel>{group.heading}</SidebarGroupLabel>
            )}
            <SidebarGroupContent>
              <SidebarMenu>
                {group.items.map((item) => {
                  const isActive = isNavActive(
                    location.pathname,
                    item.to,
                    item.end
                  );
                  return (
                    <SidebarMenuItem key={item.to}>
                      <SidebarMenuButton
                        tooltip={item.label}
                        render={<Link to={item.to} />}
                        isActive={isActive}
                      >
                        <item.icon strokeWidth={isActive ? 2.5 : 2} />
                        <span className="group-data-[collapsible=icon]:hidden">{item.label}</span>
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  );
                })}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        ))}
      </ShadcnSidebarContent>
      <SidebarFooter>
        <div className="flex items-center gap-2 px-1 py-2">
          <span className="inline-block size-1.5 bg-primary-container animate-pulse" />
          <span className="text-[10px] text-on-surface-variant/60 uppercase tracking-[0.08em] group-data-[collapsible=icon]:hidden">
            Connected
          </span>
        </div>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}

function AppHeader() {
  const { branding } = useBranding();
  const [cmdOpen, setCmdOpen] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setCmdOpen((v) => !v);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  const go = (to: string) => {
    setCmdOpen(false);
    void navigate({ to });
  };

  return (
    <header className="sticky top-0 z-40 flex h-16 shrink-0 items-center justify-between border-b border-border bg-background/80 px-6 backdrop-blur-md">
      <div className="flex items-center gap-3">
        <SidebarTrigger />
        <div className="hidden sm:block">
          <button
            type="button"
            onClick={() => setCmdOpen(true)}
            className="relative block w-full max-w-md rounded-lg border border-border bg-muted/60 py-2 pl-9 pr-12 text-left text-sm text-muted-foreground hover:border-border/80 hover:text-foreground transition-all cursor-pointer"
          >
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            Search {branding.name || "FlameGate"}…
            <kbd className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 rounded border border-border bg-background px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              ⌘K
            </kbd>
          </button>
        </div>
        <div className="ml-4 hidden items-center gap-2 md:flex">
          <div className="pulse-square" />
          <span className="text-[10px] font-mono text-muted-foreground">
            LIVE
          </span>
        </div>
      </div>
      <div className="flex items-center gap-2">
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          className="sm:hidden"
          onClick={() => setCmdOpen(true)}
          aria-label="Open search"
        >
          <Search className="size-4" />
        </Button>
        <UpdateNotification />
        <ProfileMenu />
      </div>

      <CommandDialog open={cmdOpen} onOpenChange={setCmdOpen}>
        <CommandInput placeholder={`Search ${branding.name || "FlameGate"}…`} />
        <CommandList>
          <CommandEmpty>No pages found.</CommandEmpty>
          {navGroups.map((group, gi) => (
            <CommandGroup key={gi} heading={group.heading ?? "General"}>
              {group.items.map((item) => (
                <CommandItem
                  key={item.to}
                  value={`${item.label} ${item.to}`}
                  onSelect={() => go(item.to)}
                >
                  <item.icon className="size-4" />
                  <span>{item.label}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          ))}
        </CommandList>
      </CommandDialog>
    </header>
  );
}

function PageOutletFallback() {
  return (
    <div className="flex min-h-[240px] items-center justify-center py-16">
      <div className="size-6 animate-spin border-2 border-outline-variant border-t-primary-container opacity-40" />
    </div>
  );
}

function RouteProgress() {
  const fetching = useIsFetching();
  if (fetching === 0) return null;
  return (
    <div
      className="route-progress"
      role="progressbar"
      aria-label="Loading"
      aria-busy="true"
    />
  );
}

function ProfileMenu() {
  const qc = useQueryClient();
  const { data: authStatus } = useQuery({
    queryKey: ["auth-status"],
    queryFn: () => api.authStatus(),
  });

  const username = authStatus?.username || "admin";
  const initials = username.slice(0, 2).toUpperCase();

  return (
    <div className="relative group/profile">
      <Button
        variant="ghost"
        size="sm"
        aria-haspopup="true"
        className="flex h-11 items-center gap-2.5 px-2"
      >
        <div className="flex size-8 items-center justify-center bg-primary-container text-xs font-semibold text-on-primary-container">
          {initials}
        </div>
        <div className="hidden text-left sm:block">
          <p className="text-sm font-medium leading-tight text-on-surface">
            {username}
          </p>
          <p className="text-xs leading-tight text-on-surface-variant">
            Dashboard
          </p>
        </div>
        <ChevronDown className="size-4 text-on-surface-variant" />
      </Button>

      <div
        role="menu"
        className="invisible group-hover/profile:visible absolute right-0 top-full z-50 w-48 border border-outline-variant bg-surface-container pt-2"
      >
        <div className="px-4 py-3">
          <p className="text-sm font-medium text-on-surface">{username}</p>
          <p className="text-xs text-on-surface-variant">Signed in</p>
        </div>
        <div className="my-1 h-px bg-outline-variant" />
        <div className="py-1">
          <Button
            variant="ghost"
            size="sm"
            role="menuitem"
            onClick={async () => {
              await api.logout();
              qc.invalidateQueries({ queryKey: ["auth-status"] });
            }}
            className="flex w-full items-center gap-2.5 px-4 py-2 text-left text-sm text-error transition-colors hover:bg-error/10 focus:outline-none focus-visible:bg-error/10"
          >
            <LogOut className="size-4" strokeWidth={2} />
            Sign out
          </Button>
        </div>
      </div>
    </div>
  );
}

export function PageHeader({
  title,
  description,
  icon: _Icon,
  action,
}: {
  title: string;
  description?: string;
  icon?: LucideIcon;
  action?: ReactNode;
}) {
  return (
    <div className="mb-5 flex items-start justify-between gap-4">
      <div>
        <h1 className="font-display text-2xl font-semibold tracking-tight text-on-surface">
          {title}
        </h1>
        {description && (
          <p className="mt-1 text-sm text-on-surface-variant">{description}</p>
        )}
      </div>
      {action}
    </div>
  );
}
