"use client";

import { useEffect } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useAuthPrompt } from "@/stores/auth-prompt";
import { LogoMark } from "@/components/logo";

/**
 * The card that stops a signed-out visitor at the point they try to play
 * something. Mounted once at the root; playback entry points flip it on
 * through the auth-prompt store.
 */
export function AuthGate() {
  const open = useAuthPrompt((s) => s.open);
  const reason = useAuthPrompt((s) => s.reason);
  const close = useAuthPrompt((s) => s.close);
  const pathname = usePathname();

  // Close on route change, and let Escape dismiss it.
  useEffect(() => close(), [pathname, close]);
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && close();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, close]);

  if (!open) return null;

  const next = encodeURIComponent(pathname || "/home");

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="auth-gate-title"
    >
      <div
        className="absolute inset-0 bg-ink-950/80 backdrop-blur-sm"
        onClick={close}
      />
      <div className="relative w-full max-w-sm rounded-2xl border border-ink-700 bg-ink-900 p-7 shadow-lift animate-fade-up">
        <div className="mb-4 flex items-center gap-2.5">
          <LogoMark className="h-7 w-7" title="Auralis" />
          <span className="font-display text-lg text-bone-100">Sign in to listen</span>
        </div>
        <p id="auth-gate-title" className="text-sm text-bone-300">
          {reason} Your place in every episode is saved to your account, so you
          can pick up on any device.
        </p>
        <div className="mt-6 flex flex-col gap-2">
          <Link href={`/login?next=${next}`} className="btn-primary w-full" onClick={close}>
            Sign in
          </Link>
          <Link href={`/register?next=${next}`} className="btn-ghost w-full" onClick={close}>
            Create a free account
          </Link>
        </div>
        <button
          onClick={close}
          className="mt-3 w-full text-xs text-bone-400 hover:text-bone-200"
        >
          Keep browsing
        </button>
      </div>
    </div>
  );
}
