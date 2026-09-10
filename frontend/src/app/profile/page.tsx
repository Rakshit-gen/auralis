"use client";

import { useEffect, useState } from "react";
import { RequireAuth } from "@/components/layout/require-auth";
import { useAuth } from "@/stores/auth";
import { api, ApiError } from "@/lib/api";
import { useQuery } from "@tanstack/react-query";
import { useEntitlement, useHistory } from "@/lib/hooks";

function ProfileInner() {
  const { user, refreshUser } = useAuth();
  const { data: entitlement } = useEntitlement();
  const { data: history } = useHistory();

  const { data: profile } = useQuery({
    queryKey: ["profile", user?.id],
    enabled: !!user?.id,
    queryFn: () => api<{ display_name: string; bio: string; avatar_url: string }>("/me/profile"),
  });

  const [displayName, setDisplayName] = useState("");
  const [bio, setBio] = useState("");
  const [status, setStatus] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (profile) {
      setDisplayName(profile.display_name);
      setBio(profile.bio);
    }
  }, [profile]);

  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setStatus(null);
    try {
      await api("/me/profile", { method: "PUT", body: { display_name: displayName, bio, avatar_url: profile?.avatar_url ?? "" } });
      await refreshUser();
      setStatus("Saved");
    } catch (err) {
      setStatus(err instanceof ApiError ? err.message : "Could not save");
    } finally {
      setBusy(false);
    }
  };

  const [pwCurrent, setPwCurrent] = useState("");
  const [pwNew, setPwNew] = useState("");
  const [pwStatus, setPwStatus] = useState<string | null>(null);
  const [pwBusy, setPwBusy] = useState(false);
  const pwWeak = pwNew.length > 0 && (pwNew.length < 10 || !/[0-9!@#$%^&*]/.test(pwNew));

  const changePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    if (pwWeak) return;
    setPwStatus(null);
    setPwBusy(true);
    try {
      await api("/auth/password", { method: "POST", body: { current_password: pwCurrent, new_password: pwNew } });
      setPwStatus("Password updated");
      setPwCurrent("");
      setPwNew("");
    } catch (err) {
      setPwStatus(err instanceof ApiError ? err.message : "Could not update password");
    } finally {
      setPwBusy(false);
    }
  };

  const finished = history?.filter((h) => h.completed).length ?? 0;

  return (
    <div className="container-page max-w-3xl space-y-8">
      <div>
        <p className="eyebrow mb-1">Account</p>
        <h1 className="font-display text-3xl text-bone-100">Profile</h1>
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        <Stat label="Plan" value={entitlement?.plan === "premium" ? "Premium" : "Free"} />
        <Stat label="Episodes finished" value={String(finished)} />
        <Stat label="Roles" value={user?.roles.join(", ") ?? "USER"} />
      </div>

      <form onSubmit={save} className="surface space-y-4 p-6">
        <h2 className="font-display text-lg text-bone-100">Details</h2>
        <label className="block text-sm">
          <span className="mb-1 block text-bone-300">Display name</span>
          <input className="field" maxLength={60} value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
        </label>
        <label className="block text-sm">
          <span className="mb-1 block text-bone-300">Bio</span>
          <textarea className="field h-24 resize-none" maxLength={400} value={bio} onChange={(e) => setBio(e.target.value)} />
        </label>
        <div className="flex items-center gap-3">
          <button disabled={busy} className="btn-primary">
            {busy ? "Saving" : "Save changes"}
          </button>
          {status && <span className="text-sm text-bone-400">{status}</span>}
        </div>
        <p className="text-xs text-bone-500">Signed in as {user?.email}</p>
      </form>

      <form onSubmit={changePassword} className="surface space-y-4 p-6">
        <h2 className="font-display text-lg text-bone-100">Password</h2>
        <label className="block text-sm">
          <span className="mb-1 block text-bone-300">Current password</span>
          <input type="password" className="field" value={pwCurrent} onChange={(e) => setPwCurrent(e.target.value)} />
        </label>
        <label className="block text-sm">
          <span className="mb-1 block text-bone-300">New password</span>
          <input type="password" className="field" value={pwNew} onChange={(e) => setPwNew(e.target.value)} />
          <span className={`mt-1 block text-xs ${pwWeak ? "text-red-400" : "text-bone-400"}`}>
            At least 10 characters, with a number or symbol.
          </span>
        </label>
        <div className="flex items-center gap-3">
          <button disabled={pwBusy || pwWeak || !pwCurrent || !pwNew} className="btn-ghost">
            {pwBusy ? "Updating" : "Update password"}
          </button>
          {pwStatus && <span className="text-sm text-bone-400">{pwStatus}</span>}
        </div>
      </form>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="surface p-4">
      <p className="text-xs uppercase tracking-wide text-bone-500">{label}</p>
      <p className="mt-1 font-display text-xl text-bone-100">{value}</p>
    </div>
  );
}

export default function ProfilePage() {
  return (
    <RequireAuth>
      <ProfileInner />
    </RequireAuth>
  );
}
