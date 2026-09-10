"use client";

import { useEffect } from "react";
import Link from "next/link";
import { usePlayer } from "@/stores/player";
import { formatDuration, coverStyle } from "@/lib/format";
import {
  Back15Icon,
  ChevronDownIcon,
  ChevronUpIcon,
  Fwd30Icon,
  PauseIcon,
  PlayIcon,
  SkipBackIcon,
  SkipFwdIcon,
} from "@/components/icons";
import { QueuePanel } from "@/components/player/queue-panel";

const SPEEDS = [0.75, 1, 1.25, 1.5, 1.75, 2];

/** Progress bar. `edge` hugs the mini player's bottom rim; `full` carries time labels. */
function Scrubber({ variant = "full" }: { variant?: "full" | "edge" }) {
  const currentTime = usePlayer((s) => s.currentTime);
  const duration = usePlayer((s) => s.duration);
  const requestSeek = usePlayer((s) => s.requestSeek);
  const pct = duration ? (currentTime / duration) * 100 : 0;
  const fill = `linear-gradient(to right, #e2a24d ${pct}%, rgba(233,220,197,0.12) ${pct}%)`;

  const input = (
    <input
      type="range"
      min={0}
      max={duration || 0}
      step={1}
      value={currentTime}
      disabled={!duration}
      aria-label="Seek"
      onChange={(e) => requestSeek(Number(e.target.value))}
      className={
        variant === "edge"
          ? "player-scrub absolute inset-x-0 bottom-0 h-1"
          : "player-scrub h-1.5 w-full rounded-full"
      }
      style={{ background: fill }}
    />
  );

  if (variant === "edge") return input;

  return (
    <div className="flex items-center gap-3">
      <span className="w-11 text-right text-xs tabular-nums text-bone-400">{formatDuration(currentTime)}</span>
      {input}
      <span className="w-12 text-xs tabular-nums text-bone-500">
        &minus;{formatDuration(Math.max(0, duration - currentTime))}
      </span>
    </div>
  );
}

function TransportBtn({
  label,
  onClick,
  primary = false,
  children,
}: {
  label: string;
  onClick: () => void;
  primary?: boolean;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={label}
      className={
        primary
          ? "grid h-11 w-11 place-items-center rounded-full bg-amber text-ink-950 transition hover:bg-amber-soft"
          : "grid h-9 w-9 place-items-center rounded-full border border-white/10 text-bone-200 transition hover:border-white/20 hover:text-bone-100"
      }
    >
      {children}
    </button>
  );
}

export function PlayerBar() {
  const current = usePlayer((s) => s.current);
  const playing = usePlayer((s) => s.playing);
  const buffering = usePlayer((s) => s.buffering);
  const currentTime = usePlayer((s) => s.currentTime);
  const duration = usePlayer((s) => s.duration);
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

  const remaining = Math.max(0, (duration || 0) - currentTime);

  const playButton = (
    <TransportBtn label={playing ? "Pause" : "Play"} onClick={togglePlay} primary>
      {buffering ? (
        <span className="h-4 w-4 animate-spin rounded-full border-2 border-ink-950 border-t-transparent" />
      ) : playing ? (
        <PauseIcon className="h-5 w-5" />
      ) : (
        <PlayIcon className="h-5 w-5" />
      )}
    </TransportBtn>
  );

  return (
    <>
      {expanded && (
        <div className="fixed inset-0 z-40 flex items-end justify-center sm:p-4">
          <button
            type="button"
            aria-label="Close player"
            onClick={() => setExpanded(false)}
            className="absolute inset-0 bg-ink-950/70 backdrop-blur-sm"
          />
          <div className="relative w-full max-w-[440px] animate-tide-in overflow-hidden rounded-t-2xl border border-white/10 bg-ink-950/95 shadow-lift backdrop-blur-md sm:rounded-2xl">
            <div className="space-y-5 p-5">
              <div className="flex items-start gap-3.5">
                <span
                  className="h-16 w-16 shrink-0 rounded-xl ring-1 ring-white/10"
                  style={coverStyle("#e2a24d", current.showId)}
                />
                <div className="min-w-0 flex-1">
                  <Link
                    href={`/shows/${current.showSlug}`}
                    className="text-xs uppercase tracking-[0.18em] text-bone-400 hover:text-amber"
                  >
                    {current.showTitle}
                  </Link>
                  <h2 className="mt-1 font-display text-xl leading-tight text-bone-100">{current.episodeTitle}</h2>
                  <p className="mt-0.5 text-xs text-bone-500">Episode {current.episodeNumber}</p>
                  {authorization?.preview_only && (
                    <p className="mt-1.5 inline-block rounded-full border border-amber/40 px-2 py-0.5 text-[10px] uppercase tracking-[0.12em] text-amber">
                      Preview &middot; first {Math.round((authorization.preview_limit_sec || 0) / 60)} min
                    </p>
                  )}
                </div>
                <button
                  type="button"
                  onClick={() => setExpanded(false)}
                  aria-label="Collapse player"
                  className="grid h-8 w-8 shrink-0 place-items-center rounded-lg border border-white/10 text-bone-400 transition hover:text-bone-100"
                >
                  <ChevronDownIcon className="h-4 w-4" />
                </button>
              </div>

              <Scrubber />

              <div className="flex items-center justify-center gap-2">
                <TransportBtn label="Previous episode" onClick={() => void previous()}>
                  <SkipBackIcon className="h-[18px] w-[18px]" />
                </TransportBtn>
                <TransportBtn label="Back 15 seconds" onClick={() => skip(-15)}>
                  <Back15Icon className="h-[18px] w-[18px]" />
                </TransportBtn>
                {playButton}
                <TransportBtn label="Forward 30 seconds" onClick={() => skip(30)}>
                  <Fwd30Icon className="h-[18px] w-[18px]" />
                </TransportBtn>
                <TransportBtn label="Next episode" onClick={() => void next()}>
                  <SkipFwdIcon className="h-[18px] w-[18px]" />
                </TransportBtn>
              </div>

              <QueuePanel />

              <div className="flex flex-wrap items-center gap-x-5 gap-y-3 border-t border-white/10 pt-4 text-sm text-bone-300">
                <label className="flex items-center gap-2">
                  Speed
                  <select
                    value={rate}
                    onChange={(e) => setRate(Number(e.target.value))}
                    className="field w-auto py-1"
                  >
                    {SPEEDS.map((s) => (
                      <option key={s} value={s}>
                        {s}&times;
                      </option>
                    ))}
                  </select>
                </label>
                <label className="flex flex-1 items-center gap-2">
                  Volume
                  <input
                    type="range"
                    min={0}
                    max={1}
                    step={0.05}
                    value={volume}
                    onChange={(e) => setVolume(Number(e.target.value))}
                    aria-label="Volume"
                    className="player-scrub h-1.5 w-full max-w-[8rem] rounded-full"
                    style={{
                      background: `linear-gradient(to right, #e2a24d ${volume * 100}%, rgba(233,220,197,0.12) ${volume * 100}%)`,
                    }}
                  />
                </label>
                <p className="text-xs text-bone-500">Press ? for shortcuts</p>
              </div>
            </div>
          </div>
        </div>
      )}

      <div className="pointer-events-none fixed inset-x-0 bottom-0 z-30 px-3 pb-3">
        <div className="pointer-events-auto relative mx-auto max-w-[560px] overflow-hidden rounded-2xl border border-white/10 bg-ink-950/90 shadow-lift backdrop-blur-md">
          <div className="px-4 pb-4 pt-3.5">
            <div className="flex items-center gap-3">
              <button
                type="button"
                onClick={() => setExpanded(true)}
                className="flex min-w-0 flex-1 items-center gap-3 text-left"
                aria-label="Open full player"
              >
                <span
                  className="h-12 w-12 shrink-0 rounded-xl ring-1 ring-white/10"
                  style={coverStyle("#e2a24d", current.showId)}
                />
                <span className="min-w-0">
                  <span className="block truncate font-display text-[15px] leading-tight text-bone-100">
                    {current.episodeTitle}
                  </span>
                  <span className="mt-0.5 block truncate text-xs text-bone-400">
                    {current.showTitle} &middot; Ep {current.episodeNumber}
                  </span>
                </span>
              </button>

              <div className="flex shrink-0 items-center gap-2">
                <select
                  value={rate}
                  onChange={(e) => setRate(Number(e.target.value))}
                  aria-label="Playback speed"
                  className="hidden h-8 rounded-full border border-white/10 bg-transparent pl-2.5 pr-1 text-xs text-bone-400 focus:border-signal focus:outline-none sm:block"
                >
                  {SPEEDS.map((s) => (
                    <option key={s} value={s}>
                      {s}&times;
                    </option>
                  ))}
                </select>
                <button
                  type="button"
                  onClick={() => setExpanded(true)}
                  aria-label="Open full player"
                  className="grid h-8 w-8 place-items-center rounded-lg border border-white/10 text-bone-400 transition hover:text-bone-100"
                >
                  <ChevronUpIcon className="h-4 w-4" />
                </button>
              </div>
            </div>

            <div className="mt-3 flex items-center gap-1.5">
              <TransportBtn label="Back 15 seconds" onClick={() => skip(-15)}>
                <Back15Icon className="h-[18px] w-[18px]" />
              </TransportBtn>
              {playButton}
              <TransportBtn label="Forward 30 seconds" onClick={() => skip(30)}>
                <Fwd30Icon className="h-[18px] w-[18px]" />
              </TransportBtn>
              <span className="ml-auto flex items-baseline gap-2 text-xs tabular-nums">
                <span className="text-bone-300">{formatDuration(currentTime)}</span>
                <span className="text-bone-500">&minus;{formatDuration(remaining)}</span>
              </span>
            </div>
          </div>

          <Scrubber variant="edge" />
        </div>
      </div>
    </>
  );
}
