"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useAuth } from "@/stores/auth";
import { WaveIcon } from "@/components/icons";

export default function RegisterPage() {
  const router = useRouter();
  const { register, loading, error } = useAuth();
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");

  const weak = password.length > 0 && (password.length < 10 || !/[0-9!@#$%^&*]/.test(password));

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (weak) return;
    try {
      await register(email, password, displayName);
      router.replace("/home");
    } catch {
      /* error surfaced from the store */
    }
  };

  return (
    <div className="container-page flex min-h-[70vh] items-center justify-center py-12">
      <div className="surface w-full max-w-md p-8 animate-fade-up">
        <div className="mb-6 flex items-center gap-2">
          <WaveIcon className="h-6 w-6 text-amber" />
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
          <Link href="/login" className="text-amber-soft">
            Sign in
          </Link>
        </p>
      </div>
    </div>
  );
}
