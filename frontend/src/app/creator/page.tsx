"use client";

import Link from "next/link";
import { RequireAuth } from "@/components/layout/require-auth";
import { useMyShows } from "@/lib/creator";
import { Spinner, EmptyState } from "@/components/ui";
import { SparkIcon } from "@/components/icons";
import { relativeTime } from "@/lib/format";

const STATUS_STYLE: Record<string, string> = {
  draft: "text-bone-400",
  ready_for_review: "text-signal",
  approved: "text-amber-soft",
  published: "text-green-400",
  rejected: "text-red-400",
  archived: "text-bone-500",
};

function CreatorInner() {
  const { data: shows, isLoading } = useMyShows();

  return (
    <div className="container-page space-y-8">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="eyebrow mb-1">Creator</p>
          <h1 className="font-display text-3xl text-bone-100">Your shows</h1>
        </div>
        <div className="flex gap-2">
          <Link href="/generate" className="btn-ghost">
            <SparkIcon className="h-4 w-4 text-signal" /> Generate a series
          </Link>
          <Link href="/creator/analytics" className="btn-quiet">
            Analytics
          </Link>
        </div>
      </div>

      {isLoading ? (
        <Spinner />
      ) : !shows?.length ? (
        <EmptyState
          title="No shows yet"
          hint="Generate an AI series to get a full show with draft episodes, or an admin can grant you upload access."
          action={
            <Link href="/generate" className="btn-primary">
              Generate your first series
            </Link>
          }
        />
      ) : (
        <div className="space-y-3">
          {shows.map((s) => (
            <Link
              key={s.id}
              href={`/creator/shows/${s.id}`}
              className="surface flex items-center justify-between gap-4 p-4 hover:border-amber/50"
            >
              <div className="min-w-0">
                <p className="truncate font-display text-lg text-bone-100">{s.title}</p>
                <p className="text-xs text-bone-400">
                  {s.episode_count} episodes · updated {relativeTime(s.published_at ?? undefined) || "recently"}
                </p>
              </div>
              <span className={`tag ${STATUS_STYLE[s.status] ?? ""}`}>{s.status.replace(/_/g, " ")}</span>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}

export default function CreatorPage() {
  return (
    <RequireAuth role="CREATOR">
      <CreatorInner />
    </RequireAuth>
  );
}
