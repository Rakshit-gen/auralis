"use client";

import { create } from "zustand";
import { api, clearTokens, loadTokens, saveTokens, type Tokens } from "@/lib/api";

export type User = {
  id: string;
  email: string;
  roles: string[];
  display_name: string;
  status: string;
};

type AuthState = {
  user: User | null;
  ready: boolean;
  loading: boolean;
  error: string | null;
  init: () => Promise<void>;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, displayName: string) => Promise<void>;
  logout: () => Promise<void>;
  refreshUser: () => Promise<void>;
  hasRole: (role: string) => boolean;
};

async function persist(tokens: Tokens) {
  saveTokens(tokens);
}

export const useAuth = create<AuthState>((set, get) => ({
  user: null,
  ready: false,
  loading: false,
  error: null,

  init: async () => {
    if (get().ready) return;
    const tokens = loadTokens();
    if (!tokens) {
      set({ ready: true });
      return;
    }
    try {
      const user = await api<User>("/auth/me");
      set({ user, ready: true });
    } catch {
      clearTokens();
      set({ user: null, ready: true });
    }
  },

  login: async (email, password) => {
    set({ loading: true, error: null });
    try {
      const res = await api<{ user: User; tokens: Tokens }>("/auth/login", {
        method: "POST",
        auth: false,
        body: { email, password },
      });
      await persist(res.tokens);
      set({ user: res.user, loading: false, ready: true });
    } catch (e) {
      set({ loading: false, error: (e as Error).message });
      throw e;
    }
  },

  register: async (email, password, displayName) => {
    set({ loading: true, error: null });
    try {
      const res = await api<{ user: User; tokens: Tokens }>("/auth/register", {
        method: "POST",
        auth: false,
        body: { email, password, display_name: displayName },
      });
      await persist(res.tokens);
      set({ user: res.user, loading: false, ready: true });
    } catch (e) {
      set({ loading: false, error: (e as Error).message });
      throw e;
    }
  },

  logout: async () => {
    const tokens = loadTokens();
    try {
      await api("/auth/logout", { method: "POST", body: { refresh_token: tokens?.refresh_token } });
    } catch {
      /* logout is best-effort */
    }
    clearTokens();
    set({ user: null });
  },

  refreshUser: async () => {
    try {
      const user = await api<User>("/auth/me");
      set({ user });
    } catch {
      /* leave current state */
    }
  },

  hasRole: (role) => get().user?.roles.includes(role) ?? false,
}));
