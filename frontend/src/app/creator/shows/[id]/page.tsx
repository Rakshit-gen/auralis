"use client";

import { use } from "react";
import Link from "next/link";
import { RequireAuth } from "@/components/layout/require-auth";
import { useCreatorShow, useReviewHistory, useSubmitForReview } from "@/lib/creator";
import { Spinner, ErrorState, ProgressBar } from "@/components/ui";
import { relativeTime, formatDuration } from "@/lib/format";

const PROC_LABEL: Record<string, string> = {
  none: "Not started",
  queued: "Queued",
  generating_script: "Writing script",
  synthesizing: "Synthesizing voices",
  assembling: "Assembling audio",
  packaging: "Packaging HLS",
  ready: "Ready",
  failed: "Failed",
};

const PROC_PCT: Record<string, number> = {
  queued: 0.1,
  generating_script: 0.25,
  synthesizing: 0.5,
  assembling: 0.7,
  packaging: 0.85,
  ready: 1,
};

function ShowManageInner({ id }: { id: string }) {
  const { data, isLoading, error, refetch } = useCreatorShow(id);
  const { data: history } = useReviewHistory(id);
  const submit = useSubmitForReview();

  if (isLoading) return <Spinner />;
  if (error || !data?.show) {
    return <ErrorState message={(error as Error)?.message ?? "Show not found"} retry={() => void refetch()} />;
  }

  const { show, episodes } = data;
  const readyEpisodes = episodes.filter((e) => e.processing === "ready" || e.duration_sec > 0);
  const canSubmitShow = ["draft", "rejected"].includes(show.status) && readyEpisodes.length > 0;

  return (
    <div className="container-page space-y-8">
      <div>
        <Link href="/creator" className="text-sm text-bone-400 hover:text-bone-200">
          ← All shows
        </Link>
        <div className="mt-2 flex flex-wrap items-center justify-between gap-4">
          <h1 className="font-display text-3xl text-bone-100">{show.title}</h1>
          <span className="tag text-amber-soft">{show.status.replace(/_/g, " ")}</span>
        </div>
        <p className="mt-2 max-w-2xl text-sm text-bone-300">{show.synopsis}</p>
      </div>

      <div className="flex flex-wrap gap-3">
        <button
          disabled={!canSubmitShow || submit.isPending}
          onClick={() => submit.mutate({ kind: "show", id: show.id })}
          className="btn-primary"
        >
          {submit.isPending ? "Submitting" : "Submit show for review"}
        </button>
        <Link href={`/creator/shows/${id}/review`} className="btn-ghost">
          Review status
        </Link>
        {show.status === "published" && (
          <Link href={`/shows/${show.slug}`} className="btn-quiet">
            View public page
          </Link>
        )}
      </div>
      {!canSubmitShow && ["draft", "rejected"].includes(show.status) && (
        <p className="text-sm text-bone-400">
          A show needs at least one episode with finished audio before it can go to review.
        </p>
      )}

      <section>
        <h2 className="mb-4 font-display text-2xl text-bone-100">Episodes</h2>
        <div className="space-y-3">
          {episodes.map((e) => {
            const ready = e.processing === "ready" || e.duration_sec > 0;
            const pct = PROC_PCT[e.processing] ?? (ready ? 1 : 0);
            return (
              <div key={e.id} className="surface p-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="text-xs font-semibold text-bone-400">EP {e.number}</p>
                    <p className="font-display text-lg text-bone-100">{e.title}</p>
                    <p className="mt-1 line-clamp-2 text-sm text-bone-300">{e.synopsis}</p>
                  </div>
                  <span className="tag shrink-0">{e.status.replace(/_/g, " ")}</span>
                </div>
                <div className="mt-3 flex items-center gap-3">
                  <div className="flex-1">
                    <ProgressBar value={pct} />
                  </div>
                  <span className="shrink-0 text-xs text-bone-400">
                    {PROC_LABEL[e.processing] ?? e.processing}
                    {ready && ` · ${formatDuration(e.duration_sec)}`}
                  </span>
                </div>
                {e.processing_error && <p className="mt-2 text-xs text-red-400">{e.processing_error}</p>}
                <div className="mt-3 flex gap-2">
                  <Link href={`/creator/script/${e.id}`} className="btn-quiet text-sm">
                    Edit script
                  </Link>
                </div>
              </div>
            );
          })}
        </div>
      </section>

      {!!history?.length && (
        <section>
          <h2 className="mb-3 font-display text-lg text-bone-100">Review history</h2>
          <ul className="space-y-2 text-sm">
            {history.map((h, i) => (
              <li key={i} className="surface p-3">
                <span className="text-bone-200">{h.action}</span> → {h.to_status.replace(/_/g, " ")}
                {h.notes && <p className="mt-1 text-bone-400">{h.notes}</p>}
                <p className="mt-1 text-xs text-bone-500">{relativeTime(h.created_at)}</p>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}

export default function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  return (
    <RequireAuth role="CREATOR">
      <ShowManageInner id={id} />
    </RequireAuth>
  );
}
