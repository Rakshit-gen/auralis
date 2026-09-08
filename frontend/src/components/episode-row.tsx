"use client";

import { useState } from "react";
import { formatDuration } from "@/lib/format";
import { usePlayer } from "@/stores/player";
import type { Episode, Show } from "@/lib/types";
import { BookmarkIcon, PlayIcon, PauseIcon } from "@/components/icons";
import { ProgressBar } from "@/components/ui";
import { useToggleBookmark, useBookmarks } from "@/lib/hooks";
import { useAuth } from "@/stores/auth";

export function EpisodeRow({
  episode,
  show,
  queue,
  resumeSec,
}: {
  episode: Episode;
  show: Show;
  queue?: Episode[];
  resumeSec?: number;
}) {
  const user = useAuth((s) => s.user);
  const current = usePlayer((s) => s.current);
  const playing = usePlayer((s) => s.playing);
  const togglePlay = usePlayer((s) => s.togglePlay);
  const setQueueAction = usePlayer((s) => s.setQueue);
  const [busy, setBusy] = useState(false);

  const isCurrent = current?.episodeId === episode.id;
  const ready = episode.processing === "ready" || episode.duration_sec > 0;

  const { data: bookmarks } = useBookmarks();
  const toggleBookmark = useToggleBookmark();
  const bookmarked = !!bookmarks?.some((b) => b.episode_id === episode.id);

  const play = async () => {
    if (isCurrent) {
      togglePlay();
      return;
    }
    setBusy(true);
    try {
      const list = (queue ?? [episode]).map((e) => ({
        episodeId: e.id,
        showId: show.id,
        showSlug: show.slug,
        showTitle: show.title,
        episodeTitle: e.title,
        episodeNumber: e.number,
      }));
      const startIndex = Math.max(0, list.findIndex((e) => e.episodeId === episode.id));
      await setQueueAction(list, startIndex);
    } finally {
      setBusy(false);
    }
  };

  const progress = resumeSec && episode.duration_sec ? resumeSec / episode.duration_sec : 0;

  return (
    <div
      className={`surface flex items-start gap-4 p-4 transition ${
        isCurrent ? "border-amber/60 bg-amber/[0.04]" : "hover:border-ink-600"
      }`}
    >
      <button
        onClick={play}
        disabled={!ready || busy}
        aria-label={isCurrent && playing ? "Pause" : `Play episode ${episode.number}`}
        className="mt-0.5 grid h-11 w-11 shrink-0 place-items-center rounded-full border border-ink-600 text-amber transition hover:border-amber hover:bg-amber/10 disabled:opacity-40"
      >
        {isCurrent && playing ? <PauseIcon className="h-5 w-5" /> : <PlayIcon className="h-5 w-5" />}
      </button>

      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="text-xs font-semibold text-bone-400">EP {episode.number}</span>
          {episode.is_premium && <span className="tag border-amber/50 text-amber-soft">Premium</span>}
          {!ready && <span className="tag text-signal">Processing</span>}
        </div>
        <p className="mt-0.5 font-display text-lg text-bone-100">{episode.title}</p>
        {episode.synopsis && <p className="mt-1 line-clamp-2 text-sm text-bone-300">{episode.synopsis}</p>}
        <div className="mt-2 flex items-center gap-3 text-xs text-bone-400">
          <span>{formatDuration(episode.duration_sec)}</span>
          {episode.free_preview_sec > 0 && episode.is_premium && (
            <span>{Math.round(episode.free_preview_sec / 60)} min free preview</span>
          )}
        </div>
        {progress > 0 && progress < 0.99 && (
          <div className="mt-2 max-w-xs">
            <ProgressBar value={progress} />
          </div>
        )}
      </div>

      {user && (
        <button
          onClick={() => toggleBookmark.mutate({ episodeId: episode.id, showId: show.id, bookmarked })}
          aria-label={bookmarked ? "Remove bookmark" : "Bookmark episode"}
          className={`mt-0.5 shrink-0 rounded-lg p-2 transition ${
            bookmarked ? "text-amber" : "text-bone-400 hover:text-bone-100"
          }`}
        >
          <BookmarkIcon filled={bookmarked} className="h-5 w-5" />
        </button>
      )}
    </div>
  );
}
