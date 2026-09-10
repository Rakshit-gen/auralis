"use client";

import { useEffect } from "react";
import Link from "next/link";
import { usePlayer } from "@/stores/player";
import { formatDuration, coverStyle } from "@/lib/format";
import { Back15Icon, Fwd30Icon, PauseIcon, PlayIcon, SkipBackIcon, SkipFwdIcon } from "@/components/icons";
import { QueuePanel } from "@/components/player/queue-panel";

const SPEEDS = [0.75, 1, 1.25, 1.5, 1.75, 2];

function Scrubber({ compact = false }: { compact?: boolean }) {
  const currentTime = usePlayer((s) => s.currentTime);
  const duration = usePlayer((s) => s.duration);
  const requestSeek = usePlayer((s) => s.requestSeek);
  const pct = duration ? (currentTime / duration) * 100 : 0;

  return (
    <div className={compact ? "flex items-center gap-2" : "flex items-center gap-3"}>
      {!compact && <span className="w-12 text-right text-xs tabular-nums text-bone-400">{formatDuration(currentTime)}</span>}
      <input
        type="range"
        min={0}
        max={duration || 0}
        step={1}
        value={currentTime}
        disabled={!duration}
        aria-label="Seek"
        onChange={(e) => requestSeek(Number(e.target.value))}
        className="h-1.5 w-full cursor-pointer appearance-none rounded-full bg-ink-700 accent-amber disabled:cursor-default disabled:opacity-60"
        style={{ background: `linear-gradient(to right, #d9963f ${pct}%, #26221d ${pct}%)` }}
      />
      {!compact && <span className="w-12 text-xs tabular-nums text-bone-400">{formatDuration(duration)}</span>}
    </div>
  );
}

export function PlayerBar() {
  const current = usePlayer((s) => s.current);
  const playing = usePlayer((s) => s.playing);
  const buffering = usePlayer((s) => s.buffering);
  const rate = usePlayer((s) => s.rate);
  const volume = usePlayer((s) => s.volume);
  const expanded = usePlayer((s) => s.expanded);
  const authorization = usePlayer((s) => s.authorization);

  const togglePlay = usePlayer((s) => s.togglePlay);
  const skip = usePlayer((s) => s.skip);
  const next = usePlayer((s) => s.next);
  const previous = usePlayer((s) => s.previous);
  const setRate = usePlayer((s) => s.setRate);
  const setVolume = usePlayer((s) => s.setVolume);
  const setExpanded = usePlayer((s) => s.setExpanded);

  // Global keyboard controls, disabled while typing.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable) return;
      if (!current) return;
      if (e.key === "Escape" && expanded) {
        setExpanded(false);
        return;
      }
      switch (e.key) {
        case " ":
        case "k":
          e.preventDefault();
          togglePlay();
          break;
        case "ArrowLeft":
          e.preventDefault();
          skip(-15);
          break;
        case "ArrowRight":
          e.preventDefault();
          skip(30);
          break;
        case "j":
          skip(-15);
          break;
        case "l":
          skip(30);
          break;
        case "n":
          void next();
          break;
        case "p":
          void previous();
          break;
        case "ArrowUp":
          e.preventDefault();
          setVolume(Math.min(1, volume + 0.1));
          break;
        case "ArrowDown":
          e.preventDefault();
          setVolume(Math.max(0, volume - 0.1));
          break;
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [current, expanded, setExpanded, togglePlay, skip, next, previous, setVolume, volume]);

  if (!current) return null;

  const controls = (
    <div className="flex items-center gap-1">
      <button onClick={() => void previous()} className="btn-quiet p-2" aria-label="Previous episode">
        <SkipBackIcon />
      </button>
      <button onClick={() => skip(-15)} className="btn-quiet p-2" aria-label="Back 15 seconds">
        <Back15Icon />
      </button>
      <button
        onClick={togglePlay}
        aria-label={playing ? "Pause" : "Play"}
        className="grid h-11 w-11 place-items-center rounded-full bg-amber text-ink-950 transition hover:bg-amber-soft"
      >
        {buffering ? (
          <span className="h-4 w-4 animate-spin rounded-full border-2 border-ink-950 border-t-transparent" />
        ) : playing ? (
          <PauseIcon className="h-5 w-5" />
        ) : (
          <PlayIcon className="h-5 w-5" />
        )}
      </button>
      <button onClick={() => skip(30)} className="btn-quiet p-2" aria-label="Forward 30 seconds">
        <Fwd30Icon />
      </button>
      <button onClick={() => void next()} className="btn-quiet p-2" aria-label="Next episode">
        <SkipFwdIcon />
      </button>
    </div>
  );

  return (
    <>
      {expanded && (
        <div className="fixed inset-0 z-40 flex flex-col overflow-y-auto bg-ink-950/95 backdrop-blur-lg">
          <div className="container-page flex min-h-full flex-1 flex-col items-center justify-center gap-8 py-16">
            <button onClick={() => setExpanded(false)} className="btn-quiet self-end" aria-label="Close full player">
              Collapse
            </button>
            <div
              className="h-56 w-56 rounded-2xl border border-ink-700 shadow-lift"
              style={coverStyle("#d9963f", current.showId)}
            />
            <div className="text-center">
              <Link href={`/shows/${current.showSlug}`} className="eyebrow hover:text-amber">
                {current.showTitle}
              </Link>
              <h1 className="mt-1 font-display text-3xl text-bone-100">{current.episodeTitle}</h1>
              <p className="text-sm text-bone-400">Episode {current.episodeNumber}</p>
              {authorization?.preview_only && (
                <p className="mt-2 text-sm text-amber-soft">
                  Free preview: first {Math.round((authorization.preview_limit_sec || 0) / 60)} minutes
                </p>
              )}
            </div>
            <div className="w-full max-w-xl">
              <Scrubber />
            </div>
            {controls}
            <QueuePanel />
            <p className="text-xs text-bone-500">Press ? for keyboard shortcuts</p>
            <div className="flex items-center gap-6 text-sm text-bone-300">
              <label className="flex items-center gap-2">
                Speed
                <select
                  value={rate}
                  onChange={(e) => setRate(Number(e.target.value))}
                  className="field w-auto py-1"
                >
                  {SPEEDS.map((s) => (
                    <option key={s} value={s}>
                      {s}x
                    </option>
                  ))}
                </select>
              </label>
              <label className="flex items-center gap-2">
                Volume
                <input
                  type="range"
                  min={0}
                  max={1}
                  step={0.05}
                  value={volume}
                  onChange={(e) => setVolume(Number(e.target.value))}
                  className="w-28 accent-amber"
                  aria-label="Volume"
                />
              </label>
            </div>
          </div>
        </div>
      )}

      <div className="fixed inset-x-0 bottom-0 z-30 border-t border-ink-700 bg-ink-900/95 backdrop-blur">
        <div className="container-page flex items-center gap-4 py-2.5">
          <button
            onClick={() => setExpanded(true)}
            className="flex min-w-0 items-center gap-3 text-left"
            aria-label="Open full player"
          >
            <div className="h-11 w-11 shrink-0 rounded-lg" style={coverStyle("#d9963f", current.showId)} />
            <div className="min-w-0">
              <p className="truncate text-sm font-medium text-bone-100">{current.episodeTitle}</p>
              <p className="truncate text-xs text-bone-400">
                {current.showTitle} · EP {current.episodeNumber}
              </p>
            </div>
          </button>

          <div className="hidden flex-1 flex-col gap-1.5 sm:flex">
            <div className="mx-auto">{controls}</div>
            <Scrubber />
          </div>

          <div className="flex sm:hidden">{controls}</div>

          <select
            value={rate}
            onChange={(e) => setRate(Number(e.target.value))}
            aria-label="Playback speed"
            className="hidden shrink-0 rounded border border-ink-600 bg-ink-950 px-2 py-1 text-xs text-bone-200 md:block"
          >
            {SPEEDS.map((s) => (
              <option key={s} value={s}>
                {s}x
              </option>
            ))}
          </select>
        </div>
      </div>
    </>
  );
}
