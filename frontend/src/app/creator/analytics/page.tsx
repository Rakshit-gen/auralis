"use client";

import { RequireAuth } from "@/components/layout/require-auth";
import { useMyShows } from "@/lib/creator";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { Spinner } from "@/components/ui";
import { formatCount } from "@/lib/format";

type ShowPerf = {
  show_id: string;
  title: string;
  plays: number;
  completes: number;
  unique_listeners: number;
  listening_minutes: number;
  completion_rate: number;
  likes: number;
  follows: number;
};

function ShowAnalyticsCard({ showId, title }: { showId: string; title: string }) {
  const { data, isLoading } = useQuery({
    queryKey: ["creator-analytics", showId],
    queryFn: () => api<ShowPerf>(`/analytics/shows/${showId}`),
    retry: false,
  });

  return (
    <div className="surface p-5">
      <p className="font-display text-lg text-bone-100">{title}</p>
      {isLoading ? (
        <div className="mt-3">
          <Spinner label="Loading" />
        </div>
      ) : !data ? (
        <p className="mt-2 text-sm text-bone-400">No plays recorded yet.</p>
      ) : (
        <dl className="mt-3 grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
          <Metric label="Plays" value={formatCount(data.plays)} />
          <Metric label="Unique listeners" value={formatCount(data.unique_listeners)} />
          <Metric label="Completion" value={`${Math.round(data.completion_rate * 100)}%`} />
          <Metric label="Minutes" value={formatCount(data.listening_minutes)} />
          <Metric label="Likes" value={formatCount(data.likes)} />
          <Metric label="Follows" value={formatCount(data.follows)} />
        </dl>
      )}
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs uppercase tracking-wide text-bone-500">{label}</dt>
      <dd className="font-display text-xl text-bone-100">{value}</dd>
    </div>
  );
}

function CreatorAnalyticsInner() {
  const { data: shows, isLoading } = useMyShows();

  return (
    <div className="container-page space-y-6">
      <div>
        <p className="eyebrow mb-1">Creator</p>
        <h1 className="font-display text-3xl text-bone-100">Analytics</h1>
        <p className="mt-2 text-sm text-bone-300">
          Per-show performance from the analytics aggregates. Numbers update within a minute of a
          listen.
        </p>
      </div>
      {isLoading ? (
        <Spinner />
      ) : !shows?.length ? (
        <p className="text-sm text-bone-400">Publish a show to see analytics.</p>
      ) : (
        <div className="space-y-4">
          {shows
            .filter((s) => s.status === "published")
            .map((s) => (
              <ShowAnalyticsCard key={s.id} showId={s.id} title={s.title} />
            ))}
          {shows.every((s) => s.status !== "published") && (
            <p className="text-sm text-bone-400">None of your shows are published yet.</p>
          )}
        </div>
      )}
    </div>
  );
}

export default function CreatorAnalyticsPage() {
  return (
    <RequireAuth role="CREATOR">
      <CreatorAnalyticsInner />
    </RequireAuth>
  );
}
