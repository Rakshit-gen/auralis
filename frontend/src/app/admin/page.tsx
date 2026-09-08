"use client";

import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { Spinner } from "@/components/ui";
import { formatCount } from "@/lib/format";

type Overview = {
  dau: number;
  mau: number;
  unique_listeners: number;
  listening_minutes_30d: number;
  plays_30d: number;
  completion_rate: number;
  skip_rate: number;
  avg_listen_minutes: number;
  daily: { date: string; listeners: number; plays: number; completes: number }[];
};

export default function AdminOverviewPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ["admin-overview"],
    queryFn: () => api<Overview>("/analytics/overview"),
  });
  const { data: top } = useQuery({
    queryKey: ["admin-top-shows"],
    queryFn: () =>
      api<{ shows: { show_id: string; title: string; plays: number; completion_rate: number }[] }>(
        "/analytics/shows/top",
      ).then((r) => r.shows),
  });

  if (isLoading) return <Spinner />;
  if (error) return <p className="text-sm text-red-400">{(error as Error).message}</p>;

  const maxPlays = Math.max(1, ...(data?.daily ?? []).map((d) => d.plays));

  return (
    <div className="space-y-8">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Card label="Daily active" value={formatCount(data?.dau ?? 0)} />
        <Card label="Monthly active" value={formatCount(data?.mau ?? 0)} />
        <Card label="Unique listeners" value={formatCount(data?.unique_listeners ?? 0)} />
        <Card label="Listening minutes (30d)" value={formatCount(data?.listening_minutes_30d ?? 0)} />
        <Card label="Plays (30d)" value={formatCount(data?.plays_30d ?? 0)} />
        <Card label="Completion rate" value={`${Math.round((data?.completion_rate ?? 0) * 100)}%`} />
        <Card label="Skip rate" value={`${Math.round((data?.skip_rate ?? 0) * 100)}%`} />
        <Card label="Avg listen" value={`${data?.avg_listen_minutes ?? 0} min`} />
      </div>

      <section className="surface p-5">
        <h2 className="mb-4 font-display text-lg text-bone-100">Plays, last 14 days</h2>
        {data?.daily.length ? (
          <div className="flex h-40 items-end gap-1">
            {data.daily.map((d) => (
              <div key={d.date} className="flex flex-1 flex-col items-center gap-1">
                <div
                  className="w-full rounded-t bg-amber/70"
                  style={{ height: `${(d.plays / maxPlays) * 100}%` }}
                  title={`${d.date}: ${d.plays} plays`}
                />
                <span className="text-[10px] text-bone-500">{d.date.slice(5)}</span>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-bone-400">No daily data yet.</p>
        )}
      </section>

      <section>
        <h2 className="mb-3 font-display text-lg text-bone-100">Top shows</h2>
        <div className="space-y-2">
          {(top ?? []).map((s, i) => (
            <div key={s.show_id} className="surface flex items-center gap-3 p-3 text-sm">
              <span className="w-6 text-center text-ink-500">{i + 1}</span>
              <span className="flex-1 truncate text-bone-100">{s.title}</span>
              <span className="text-bone-400">{formatCount(s.plays)} plays</span>
              <span className="text-bone-500">{Math.round(s.completion_rate * 100)}% done</span>
            </div>
          ))}
          {!top?.length && <p className="text-sm text-bone-400">No plays recorded yet.</p>}
        </div>
      </section>
    </div>
  );
}

function Card({ label, value }: { label: string; value: string }) {
  return (
    <div className="surface p-4">
      <p className="text-xs uppercase tracking-wide text-bone-500">{label}</p>
      <p className="mt-1 font-display text-2xl text-bone-100">{value}</p>
    </div>
  );
}
