"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import { Spinner, EmptyState } from "@/components/ui";
import type { Episode, Show } from "@/lib/types";

type Queue = { shows: Show[]; episodes: Episode[] };

export default function ReviewQueuePage() {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({
    queryKey: ["review-queue"],
    queryFn: () => api<Queue>("/content/admin/review-queue"),
    refetchInterval: 15_000,
  });

  const [notes, setNotes] = useState<Record<string, string>>({});
  const [error, setError] = useState<string | null>(null);

  const review = async (type: "show" | "episode", id: string, action: string) => {
    setError(null);
    try {
      await api(`/content/admin/review/${type}/${id}`, {
        method: "POST",
        body: { action, notes: notes[`${type}:${id}`] ?? "" },
      });
      if (action === "approve") {
        // Move straight to published; the two-step state machine still records both.
        await api(`/content/admin/review/${type}/${id}`, { method: "POST", body: { action: "publish" } });
      }
      qc.invalidateQueries({ queryKey: ["review-queue"] });
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Action failed");
    }
  };

  const act = useMutation({ mutationFn: (v: { type: "show" | "episode"; id: string; action: string }) => review(v.type, v.id, v.action) });

  if (isLoading) return <Spinner />;

  const items: { type: "show" | "episode"; id: string; title: string; sub: string }[] = [
    ...(data?.episodes ?? []).map((e) => ({
      type: "episode" as const,
      id: e.id,
      title: `Episode ${e.number}: ${e.title}`,
      sub: e.synopsis,
    })),
    ...(data?.shows ?? []).map((s) => ({ type: "show" as const, id: s.id, title: s.title, sub: s.synopsis })),
  ];

  if (!items.length) {
    return <EmptyState title="Review queue is clear" hint="Submitted shows and episodes will appear here." />;
  }

  return (
    <div className="space-y-4">
      <p className="text-sm text-bone-400">
        {items.length} item{items.length === 1 ? "" : "s"} waiting. Approving an item runs the
        approve then publish steps; both are recorded in its history.
      </p>
      {error && <p className="text-sm text-red-400">{error}</p>}
      {items.map((it) => {
        const key = `${it.type}:${it.id}`;
        return (
          <div key={key} className="surface p-4">
            <div className="flex items-center gap-2">
              <span className="tag">{it.type}</span>
              <p className="font-display text-lg text-bone-100">{it.title}</p>
            </div>
            {it.sub && <p className="mt-1 line-clamp-3 text-sm text-bone-300">{it.sub}</p>}
            <textarea
              value={notes[key] ?? ""}
              onChange={(e) => setNotes((n) => ({ ...n, [key]: e.target.value }))}
              placeholder="Reviewer notes (required to reject)"
              className="field mt-3 h-16 resize-none"
            />
            <div className="mt-3 flex flex-wrap gap-2">
              <button
                className="btn-primary text-sm"
                disabled={act.isPending}
                onClick={() => act.mutate({ type: it.type, id: it.id, action: "approve" })}
              >
                Approve and publish
              </button>
              <button
                className="btn-ghost text-sm"
                disabled={act.isPending || !(notes[key] ?? "").trim()}
                onClick={() => act.mutate({ type: it.type, id: it.id, action: "reject" })}
              >
                Reject
              </button>
            </div>
          </div>
        );
      })}
    </div>
  );
}
