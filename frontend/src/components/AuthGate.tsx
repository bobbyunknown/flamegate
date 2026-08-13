import { useState, useMemo, type ReactNode } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { WifiOff, Flame } from "lucide-react";
import { api, fetchPortalBranding } from "../lib/api";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";

export function AuthGate({ children }: { children: ReactNode }) {
  const status = useQuery({ queryKey: ["auth-status"], queryFn: () => api.authStatus() });

  if (status.isLoading) {
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner />
      </div>
    );
  }
  if (status.isError) {
    return (
      <div className="flex h-full items-center justify-center px-4">
        <Card className="w-full max-w-sm p-8 text-center">
          <div className="mx-auto mb-4 flex h-14 w-14 items-center justify-center bg-error-container/10">
            <WifiOff className="h-7 w-7 text-error" strokeWidth={1.75} />
          </div>
          <Flame className="mx-auto size-10 text-primary-container opacity-60" />
          <h1 className="mt-4 text-base font-semibold tracking-tight text-on-surface">Cannot reach <AuthGateName /></h1>
          <p className="mt-1.5 text-sm text-on-surface-variant">
            Is the backend running on <code className="bg-surface-container-high px-1.5 py-0.5 font-mono text-xs text-on-surface">:20180</code>?
          </p>
          <button
            onClick={() => status.refetch()}
            className="mt-5 inline-flex items-center gap-1.5 border border-outline-variant bg-surface-container-high px-4 py-2 text-sm font-medium text-on-surface transition-colors hover:bg-surface-container"
          >
            Try again
          </button>
        </Card>
      </div>
    );
  }

  const s = status.data!;
  if (!s.authenticated) {
    return <LoginScreen />;
  }
  if (s.using_default && !s.onboarding_complete) {
    return <OnboardingScreen />;
  }
  return <>{children}</>;
}

function AuthGateName() {
  const { data } = useQuery({
    queryKey: ["portal-branding"],
    queryFn: fetchPortalBranding,
    staleTime: 5 * 60_000,
    retry: false,
  });
  return <>{data?.name || "FlameGate"}</>;
}

function FirePanel() {
  const orbs = useMemo(() => {
    const palette = ['#bf360c', '#d84315', '#e64a19', '#a84844', '#8b3a1d'];
    return Array.from({ length: 5 }, (_, i) => ({
      size: 100 + Math.random() * 120,
      left: 35 + Math.random() * 30,
      bottom: -5 + Math.random() * 15,
      duration: 4 + Math.random() * 5,
      delay: Math.random() * 3,
      color: palette[i],
      opacity: 0.25 + Math.random() * 0.2,
    }));
  }, []);

  const sparks = useMemo(() => {
    const palette = ['#d84315', '#bf360c'];
    return Array.from({ length: 20 }, (_, i) => ({
      left: 38 + Math.random() * 24,
      size: 2 + Math.random() * 2,
      duration: 3 + Math.random() * 4,
      delay: Math.random() * 6,
      wobble: Math.random() > 0.5 ? 1 : -1,
      color: palette[i % 2],
    }));
  }, []);

  return (
    <div className="relative hidden w-1/2 flex-col items-center justify-center overflow-hidden bg-[#0a0a09] lg:flex">
      <div className="absolute inset-0 bg-gradient-to-t from-[#0d0503]/95 via-[#0a0a09] to-[#0a0a09]" />

      <div className="absolute bottom-0 left-0 right-0 h-1/2 overflow-hidden pointer-events-none">
        {orbs.map((o, i) => (
          <div
            key={i}
            className="absolute rounded-full mix-blend-screen blur-2xl"
            style={{
              width: `${o.size}px`,
              height: `${o.size * 1.3}px`,
              left: `${o.left}%`,
              bottom: `${o.bottom}%`,
              background: o.color,
              opacity: o.opacity,
              animation: `fire-orb-pulse ${o.duration}s ease-in-out infinite alternate`,
              animationDelay: `${o.delay}s`,
              transform: 'translateX(-50%)',
            }}
          />
        ))}
        <div className="absolute bottom-[-10%] left-1/2 h-[45%] w-[50%] -translate-x-1/2 rounded-[100%] bg-primary-container/15 blur-[100px] animate-pulse" style={{ animationDuration: '4s' }} />
      </div>

      <div className="absolute bottom-0 left-0 right-0 h-3/4 pointer-events-none">
        {sparks.map((s, i) => (
          <div
            key={i}
            className="absolute rounded-full opacity-0"
            style={{
              '--wobble': s.wobble,
              bottom: '6%',
              left: `${s.left}%`,
              width: `${s.size}px`,
              height: `${s.size}px`,
              background: s.color,
              boxShadow: `0 0 ${s.size + 2}px ${s.size}px rgba(191,54,12,0.6)`,
              animation: `fire-spark-float ${s.duration}s ease-in infinite`,
              animationDelay: `${s.delay}s`,
            } as React.CSSProperties}
          />
        ))}
      </div>

      <div className="relative z-10 flex flex-col items-center justify-center">
        <div className="flex size-16 items-center justify-center rounded-2xl bg-surface/30 shadow-float backdrop-blur-md border border-outline-variant/20">
          <Flame className="size-8 text-primary-container/90 drop-shadow-[0_0_6px_rgba(255,85,64,0.4)]" strokeWidth={1.5} />
        </div>
        <h2 className="mt-6 text-2xl font-semibold tracking-tight text-on-surface/90">
          <AuthGateName />
        </h2>
        <p className="mt-2 text-sm text-on-surface-variant/60">AI Gateway</p>
      </div>

      <div className="absolute inset-y-0 right-0 w-24 bg-gradient-to-l from-[#131313] to-transparent" />
    </div>
  );
}

function LoginScreen() {
  const qc = useQueryClient();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");

  const login = useMutation({
    mutationFn: () => api.login(username, password),
    onSuccess: () => {
      setError("");
      qc.invalidateQueries({ queryKey: ["auth-status"] });
    },
    onError: () => setError("Incorrect username or password"),
  });

  return (
    <div className="flex h-full min-h-dvh">
      <FirePanel />

      {/* Right: login form */}
      <div className="flex w-full items-center justify-center px-6 lg:w-1/2">
        <div className="w-full max-w-sm">
          {/* Mobile-only logo */}
          <div className="mb-8 flex flex-col items-center gap-3 text-center lg:hidden">
            <Flame className="size-12 text-primary-container" strokeWidth={1.2} />
            <h1 className="text-lg font-semibold tracking-tight text-on-surface">
              <AuthGateName />
            </h1>
          </div>

          <h1 className="text-xl font-semibold tracking-tight text-on-surface">Sign in</h1>
          <p className="mt-1.5 text-sm text-on-surface-variant">Enter your credentials to continue.</p>

          <form
            className="mt-8 space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              if (username && password) login.mutate();
            }}
          >
            <div className="flex flex-col gap-1.5">
              <span className="text-xs font-medium text-on-surface-variant">Username</span>
              <Input
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="admin"
                autoFocus
                autoComplete="username"
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <span className="text-xs font-medium text-on-surface-variant">Password</span>
              <Input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••"
                autoComplete="current-password"
              />
            </div>
            {error && <p className="text-xs text-error">{error}</p>}
            <Button
              type="submit"
              className="w-full"
              disabled={login.isPending || !username || !password}
            >
              {login.isPending ? "Signing in…" : "Sign in"}
            </Button>
          </form>

          <p className="mt-6 text-xs text-on-surface-variant">
            First run? Username: <code className="font-mono">admin</code>, password:{" "}
            <code className="font-mono">flamegate</code>.
          </p>
        </div>
      </div>
    </div>
  );
}

function OnboardingScreen() {
  const qc = useQueryClient();
  const status = useQuery({ queryKey: ["auth-status"], queryFn: () => api.authStatus() });
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");

  const save = useMutation({
    mutationFn: async () => {
      await api.changePassword(password);
      await api.completeOnboarding();
    },
    onSuccess: () => {
      setError("");
      qc.invalidateQueries({ queryKey: ["auth-status"] });
    },
    onError: (e: Error) => setError(e.message),
  });

  const skip = useMutation({
    mutationFn: () => api.completeOnboarding(),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["auth-status"] }),
  });

  const valid = password.length >= 6 && password === confirm;

  return (
    <div className="flex h-full items-center justify-center px-4">
      <Card className="w-full max-w-md p-8">
        <Flame className="mb-4 size-14 text-primary-container" strokeWidth={1.2} />
        <h1 className="text-lg font-semibold tracking-tight text-on-surface">Welcome to <AuthGateName /></h1>
        <p className="mt-2 text-sm text-on-surface-variant">
          Signed in as <code className="font-mono text-on-surface">{status.data?.username || "admin"}</code> with the default password. Set a new one to secure your dashboard.
        </p>
        <form
          className="mt-5 space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (valid) save.mutate();
          }}
        >
          <div className="flex flex-col gap-1.5"><span className="text-xs font-medium text-on-surface-variant">New password</span>
            <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoFocus />
          </div>
          <div className="flex flex-col gap-1.5"><span className="text-xs font-medium text-on-surface-variant">Confirm password</span>
            <Input type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} />
          </div>
          {password && password.length < 6 && (
            <p className="text-xs text-on-surface-variant">Use at least 6 characters.</p>
          )}
          {confirm && password !== confirm && (
            <p className="text-xs text-error">Passwords don't match.</p>
          )}
          {error && <p className="text-xs text-error">{error}</p>}
          <div className="flex items-center justify-between">
            <Button variant="ghost" type="button" onClick={() => skip.mutate()} disabled={skip.isPending}>
              Keep default for now
            </Button>
            <Button type="submit" disabled={save.isPending || !valid}>
              {save.isPending ? "Saving…" : "Set password"}
            </Button>
          </div>
        </form>
      </Card>
    </div>
  );
}
