"use client";

import { create } from "zustand";

/**
 * A tiny global switch for the "sign in to listen" card. Playback entry points
 * call `require()` before they start a stream; if there is no signed-in user it
 * opens the card and returns false, and the caller stops there.
 */
type AuthPromptState = {
  open: boolean;
  reason: string;
  show: (reason?: string) => void;
  close: () => void;
};

export const useAuthPrompt = create<AuthPromptState>((set) => ({
  open: false,
  reason: "Sign in to start listening.",
  show: (reason) =>
    set({ open: true, reason: reason ?? "Sign in to start listening." }),
  close: () => set({ open: false }),
}));
