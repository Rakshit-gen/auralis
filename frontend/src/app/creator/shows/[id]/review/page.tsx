"use client";

import { use } from "react";
import Link from "next/link";
import { RequireAuth } from "@/components/layout/require-auth";
import { useCreatorShow, useReviewHistory } from "@/lib/creator";
import { Spinner } from "@/components/ui";
import { relativeTime } from "@/lib/format";

const STEPS = ["draft", "ready_for_review", "approved", "published"] as const;

function ReviewStatusInner({ id }: { id: string }) {
  const { data } = useCreatorShow(id);
  const { data: history, isLoading } = useReviewHistory(id);

  const show = data?.show;
  const currentIdx = show ? Math.max(0, STEPS.indexOf(show.status as (typeof STEPS)[number])) : 0;
  const rejected = show?.status === "rejected";

  return (
    <div className="container-page max-w-2xl space-y-8">
      <div>
        <Link href={`/creator/shows/${id}`} className="text-sm text-bone-400 hover:text-bone-200">
          ← Back to show
        </Link>
        <h1 className="mt-2 font-display text-3xl text-bone-100">Review status</h1>
      </div>

      <ol className="space-y-3">
        {STEPS.map((step, i) => {
          const done = i < currentIdx || (i === currentIdx && step === "published");
          const active = i === currentIdx;
          return (
            <li key={step} className="flex items-center gap-3">
              <span
                className={`grid h-7 w-7 place-items-center rounded-full border text-xs ${
                  done
                    ? "border-green-500 bg-green-500/20 text-green-300"
                    : active
                      ? "border-amber bg-amber/20 text-amber-soft"
                      : "border-ink-600 text-bone-500"
                }`}
              >
                {done ? "✓" : i + 1}
              </span>
              <span className={active ? "text-bone-100" : "text-bone-400"}>
                {step.replace(/_/g, " ")}
              </span>
            </li>
          );
        })}
      </ol>

      {rejected && (
        <p className="surface border-red-900/50 p-4 text-sm text-bone-200">
          A reviewer sent this back. Check the notes below, revise the scripts, and resubmit.
        </p>
      )}

      <section>
        <h2 className="mb-3 font-display text-lg text-bone-100">History</h2>
        {isLoading ? (
          <Spinner />
        ) : !history?.length ? (
          <p className="text-sm text-bone-400">No review actions yet. Submit the show to start the process.</p>
        ) : (
          <ul className="space-y-2 text-sm">
            {history.map((h, i) => (
              <li key={i} className="surface p-3">
                <span className="text-bone-200">{h.action}</span> → {h.to_status.replace(/_/g, " ")}
                {h.notes && <p className="mt-1 text-bone-400">{h.notes}</p>}
                <p className="mt-1 text-xs text-bone-500">{relativeTime(h.created_at)}</p>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

export default function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  return (
    <RequireAuth role="CREATOR">
      <ReviewStatusInner id={id} />
    </RequireAuth>
  );
}
