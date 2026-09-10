"use client";

import { Suspense, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useAuth } from "@/stores/auth";
import { LogoMark } from "@/components/logo";

function RegisterForm() {
  const router = useRouter();
  const params = useSearchParams();
  const next = params.get("next") || "/home";
  const { register, loading, error, clearError } = useAuth();
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");

  // Drop any error left over from a previous attempt on another auth page.
  useEffect(() => clearError, [clearError]);

  const weak = password.length > 0 && (password.length < 10 || !/[0-9!@#$%^&*]/.test(password));

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (weak) return;
    try {
      await register(email, password, displayName);
      router.replace(next);
    } catch {
      /* error surfaced from the store */
    }
  };

  return (
    <div className="container-page flex min-h-[70vh] items-center justify-center py-12">
      <div className="surface w-full max-w-md p-8 animate-fade-up">
        <div className="mb-6 flex items-center gap-2.5">
          <LogoMark className="h-7 w-7" title="Auralis" />
          <span className="font-display text-xl text-bone-100">Join Auralis</span>
        </div>
        <form onSubmit={submit} className="space-y-4">
          <label className="block text-sm">
            <span className="mb-1 block text-bone-300">Display name</span>
            <input
              required
              maxLength={60}
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              className="field"
            />
          </label>
          <label className="block text-sm">
            <span className="mb-1 block text-bone-300">Email</span>
            <input
              type="email"
              autoComplete="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="field"
            />
          </label>
          <label className="block text-sm">
            <span className="mb-1 block text-bone-300">Password</span>
            <input
              type="password"
              autoComplete="new-password"
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="field"
            />
            <span className={`mt-1 block text-xs ${weak ? "text-red-400" : "text-bone-400"}`}>
              At least 10 characters, with a number or symbol.
            </span>
          </label>
          {error && <p className="text-sm text-red-400">{error}</p>}
          <button type="submit" disabled={loading || weak} className="btn-primary w-full">
            {loading ? "Creating account" : "Create account"}
          </button>
        </form>
        <p className="mt-4 text-sm text-bone-300">
          Already have an account?{" "}
          <Link
            href={`/login${next !== "/home" ? `?next=${encodeURIComponent(next)}` : ""}`}
            className="text-signal-soft"
          >
            Sign in
          </Link>
        </p>
      </div>
    </div>
  );
}

export default function RegisterPage() {
  return (
    <Suspense fallback={<div className="container-page py-24" />}>
      <RegisterForm />
    </Suspense>
  );
}
