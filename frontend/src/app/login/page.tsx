"use client";

import { Suspense, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useAuth } from "@/stores/auth";
import { WaveIcon } from "@/components/icons";

function LoginForm() {
  const router = useRouter();
  const params = useSearchParams();
  const next = params.get("next") || "/home";
  const { login, loading, error } = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await login(email, password);
      router.replace(next);
    } catch {
      /* error surfaced from the store */
    }
  };

  return (
    <div className="container-page flex min-h-[70vh] items-center justify-center py-12">
      <div className="surface w-full max-w-md p-8 animate-fade-up">
        <div className="mb-6 flex items-center gap-2">
          <WaveIcon className="h-6 w-6 text-amber" />
          <span className="font-display text-xl text-bone-100">Welcome back</span>
        </div>
        <form onSubmit={submit} className="space-y-4">
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
              autoComplete="current-password"
              required
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="field"
            />
          </label>
          {error && <p className="text-sm text-red-400">{error}</p>}
          <button type="submit" disabled={loading} className="btn-primary w-full">
            {loading ? "Signing in" : "Sign in"}
          </button>
        </form>
        <p className="mt-4 text-sm text-bone-300">
          New here?{" "}
          <Link href="/register" className="text-amber-soft">
            Create an account
          </Link>
        </p>
      </div>
    </div>
  );
}

export default function LoginPage() {
  return (
    <Suspense fallback={<div className="container-page py-24" />}>
      <LoginForm />
    </Suspense>
  );
}
