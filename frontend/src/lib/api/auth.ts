import { request } from "./client";
import type { AuthStatus } from "./types";

export const authStatus = () => request<AuthStatus>("GET", "/auth/status");

export const login = (username: string, password: string) =>
  request<{ ok: boolean; using_default: boolean; onboarding_complete: boolean }>(
    "POST",
    "/auth/login",
    { username, password },
  );

export const logout = () => request<{ ok: boolean }>("POST", "/auth/logout");

export const changePassword = (newPassword: string) =>
  request<{ ok: boolean }>("POST", "/auth/password", { new_password: newPassword });

export const completeOnboarding = () => request<{ ok: boolean }>("POST", "/auth/onboarding/complete");
