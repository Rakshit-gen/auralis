"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { api } from "@/lib/api";
import { usePlayer } from "@/stores/player";
import { ProgressBar } from "@/components/ui";
import { PlayIcon } from "@/components/icons";
import { formatDuration, coverStyle } from "@/lib/format";
import { useCoverLookup } from "@/lib/hooks";
import type { ContinueItem, Episode } from "@/lib/types";

export function ContinueRow({ item }: { item: ContinueItem }) {
  const authorize = usePlayer((s) => s.playNow);
  const cover = useCoverLookup().get(item.show_id);

  const { data: episode } = useQuery({
    queryKey: ["episode", item.episode_id],
    queryFn: () => api<Episode>(`/catalog/episodes/${item.episode_id}`, { auth: false }),
    staleTime: 5 * 60_000,
  });

  const pct = item.duration_sec ? item.position_sec / item.duration_sec : 0;
  const remaining = Math.max(0, (item.duration_sec || episode?.duration_sec || 0) - item.position_sec);

  const resume = () =>
    void authorize({
      episodeId: item.episode_id,
      showId: item.show_id,
      showSlug: cover?.slug ?? "",
      showTitle: cover?.title ?? "",
      episodeTitle: episode?.title ?? "Episode",
      episodeNumber: episode?.number ?? 0,
    });

  return (
    <div className="surface flex items-center gap-3 p-3">
      <div
        className="h-14 w-14 shrink-0 rounded-lg"
        style={coverStyle(cover?.accent || "#d9963f", item.show_id, cover?.cover)}
        aria-hidden
      />
      <div className="min-w-0 flex-1">
        <p className="truncate font-display text-bone-100">{episode?.title ?? "Loading episode"}</p>
        <p className="text-xs text-bone-400">
          {remaining > 0 ? `${formatDuration(remaining)} left` : "Ready to replay"}
        </p>
        <div className="mt-2">
          <ProgressBar value={pct} />
        </div>
      </div>
      <button
        onClick={resume}
        className="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-amber text-ink-950 hover:bg-amber-soft"
        aria-label="Resume"
      >
        <PlayIcon className="h-4 w-4" />
      </button>
      {episode && (
        <Link
          href={`/shows/${cover?.slug || episode.show_id}`}
          className="hidden max-w-[8rem] shrink-0 truncate text-xs text-bone-400 hover:text-bone-200 sm:block"
        >
          {cover?.title || "Show"}
        </Link>
      )}
    </div>
  );
}
