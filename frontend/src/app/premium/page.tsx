"use client";

import { useState } from "react";
import { RequireAuth } from "@/components/layout/require-auth";
import { api, ApiError } from "@/lib/api";
import { useEntitlement, useShows } from "@/lib/hooks";
import { ShowGrid } from "@/components/show-grid";
import { relativeTime } from "@/lib/format";

function PremiumInner() {
  const { data: entitlement, refetch } = useEntitlement();
  const { data: premiumShows } = useShows({ premium: true, limit: 12 });
  const [code, setCode] = useState("");
  const [status, setStatus] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const redeem = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setStatus(null);
    try {
      await api("/me/entitlement/redeem", { method: "POST", body: { code: code.trim() } });
      await refetch();
      setStatus("Premium unlocked. Enjoy the full catalog.");
      setCode("");
    } catch (err) {
      setStatus(err instanceof ApiError ? err.message : "Could not redeem that code");
    } finally {
      setBusy(false);
    }
  };

  const active = entitlement?.premium_active;

  return (
    <div className="container-page max-w-4xl space-y-10">
      <div>
        <p className="eyebrow mb-1">Auralis Premium</p>
        <h1 className="font-display text-3xl text-bone-100">
          {active ? "You have Premium" : "Unlock every premium series"}
        </h1>
        <p className="mt-2 max-w-xl text-sm text-bone-300">
          Premium shows are marked with a badge across the catalog. This portfolio build has no
          payment provider; access is granted with a promo code or by an admin. The seeded code{" "}
          <code className="rounded bg-ink-800 px-1.5 py-0.5 text-amber-soft">AURALIS-PREMIUM</code>{" "}
          works for demonstrations.
        </p>
      </div>

      <div className="surface p-6">
        {active ? (
          <div className="text-sm text-bone-200">
            <p>
              Plan: <span className="text-amber-soft">Premium</span> · granted via {entitlement?.source.replace(/_/g, " ")}
            </p>
            {entitlement?.expires_at && (
              <p className="mt-1 text-bone-400">Renews or expires {relativeTime(entitlement.expires_at)}</p>
            )}
          </div>
        ) : (
          <form onSubmit={redeem} className="flex flex-wrap items-end gap-3">
            <label className="text-sm">
              <span className="mb-1 block text-bone-300">Promo code</span>
              <input
                className="field w-64"
                value={code}
                onChange={(e) => setCode(e.target.value.toUpperCase())}
                placeholder="AURALIS-PREMIUM"
              />
            </label>
            <button disabled={busy || !code.trim()} className="btn-primary">
              {busy ? "Redeeming" : "Redeem"}
            </button>
          </form>
        )}
        {status && <p className="mt-3 text-sm text-bone-400">{status}</p>}
      </div>

      <section>
        <h2 className="mb-4 font-display text-2xl text-bone-100">In Premium</h2>
        <ShowGrid shows={premiumShows?.shows} emptyTitle="No premium shows published yet" />
      </section>
    </div>
  );
}

export default function PremiumPage() {
  return (
    <RequireAuth>
      <PremiumInner />
    </RequireAuth>
  );
}
