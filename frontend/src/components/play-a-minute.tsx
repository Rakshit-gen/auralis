"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { useGenres, useShows } from "@/lib/hooks";
import { coverStyle, formatDuration, sampleBars } from "@/lib/format";
import { PlayIcon, PauseIcon, Back15Icon } from "@/components/icons";
import { Skeleton } from "@/components/ui";
import type { Genre, Show } from "@/lib/types";

/**
 * "Play a minute. Leave. Come back." — a working taste of the product that
 * replaces the old how-it-works accordion and genre strip. Pick a genre, hear
 * the opening minute of a real show, walk away; your spot is kept in
 * localStorage and is still there when the page reloads. That single
 * interaction demonstrates browse-by-genre, the catalog's range, streaming,
 * and resume-to-the-second.
 *
 * Clips are short static files at public/previews/<slug>.mp3 (the app's own
 * playback path needs an account, and landing visitors don't have one). A
 * missing file degrades to a link into the show.
 */

// Genres we have an opening-minute clip for, with a line about the scene.
const CLIPS: { slug: string; scene: string }[] = [
  { slug: "mystery", scene: "A voicemail plays back from a number that was disconnected years ago." },
  { slug: "science-fiction", scene: "First contact, about forty seconds before the room understands." },
  { slug: "folk-horror", scene: "A newcomer is talked through the rules of the harvest festival." },
  { slug: "thriller", scene: "The call comes in with four minutes left on the shift." },
  { slug: "noir", scene: "A confession, and a narrator who has decided not to believe a word of it." },
];

const KEY = (slug: string) => `auralis.sample.${slug}`;
const LAST_KEY = "auralis.sample.last";

function readNum(key: string): number {
  try {
    const v = Number(localStorage.getItem(key));
    return Number.isFinite(v) && v > 0 ? v : 0;
  } catch {
    return 0;
  }
}
function write(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* private mode: the demo just won't resume across reloads */
  }
}
function remove(key: string) {
  try {
    localStorage.removeItem(key);
  } catch {
    /* ignore */
  }
}

type Item = { slug: string; scene: string; genre?: Genre; show: Show };

export function PlayAMinute() {
  const { data: genres } = useGenres();
  const { data: catalog } = useShows({ limit: 100 });

  // One clip per genre, but only where a real show backs it.
  const items = useMemo<Item[]>(() => {
    if (!catalog) return [];
    return CLIPS.flatMap((c) => {
      const genre = genres?.find((g) => g.slug === c.slug);
      const show = catalog.shows.find(
        (s) => s.genres?.some((g) => g.slug === c.slug) || (genre ? s.genre_ids.includes(genre.id) : false),
      );
      return show ? [{ ...c, genre, show }] : [];
    });
  }, [catalog, genres]);

  const [active, setActive] = useState(0);
  const [pos, setPos] = useState(0);
  const [dur, setDur] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [ended, setEnded] = useState(false);
  const [missing, setMissing] = useState(false);
  const [hydrated, setHydrated] = useState(false);
  const audioRef = useRef<HTMLAudioElement>(null);
  const lastSaveRef = useRef(0);

  useEffect(() => setHydrated(true), []);

  const item = items[active];
  const bars = useMemo(() => sampleBars(item?.slug ?? "", 56), [item?.slug]);

  // Restore the last genre the visitor was on.
  useEffect(() => {
    if (!items.length) return;
    let last: string | null = null;
    try {
      last = localStorage.getItem(LAST_KEY);
    } catch {
      /* ignore */
    }
    const i = items.findIndex((it) => it.slug === last);
    if (i > 0) setActive(i);
  }, [items]);

  const save = useCallback(
    (slug: string, seconds: number) => {
      write(KEY(slug), String(Math.floor(seconds)));
      write(LAST_KEY, slug);
    },
    [],
  );

  // Persist on tab close mid-play.
  useEffect(() => {
    const onLeave = () => {
      const el = audioRef.current;
      if (el && item && el.currentTime > 0 && !el.ended) save(item.slug, el.currentTime);
    };
    window.addEventListener("pagehide", onLeave);
    return () => window.removeEventListener("pagehide", onLeave);
  }, [item, save]);

  const selectGenre = (i: number) => {
    const el = audioRef.current;
    if (el && item && !el.ended && el.currentTime > 0) save(item.slug, el.currentTime);
    setActive(i);
    setPlaying(false);
    setEnded(false);
    setMissing(false);
    setPos(0);
    setDur(0);
  };

  const toggle = () => {
    const el = audioRef.current;
    if (!el || missing) return;
    if (el.paused) {
      void el.play().then(() => setPlaying(true)).catch(() => setPlaying(false));
    } else {
      el.pause();
      setPlaying(false);
      save(item.slug, el.currentTime);
    }
  };

  const leave = () => {
    const el = audioRef.current;
    if (!el) return;
    el.pause();
    setPlaying(false);
    if (!el.ended) save(item.slug, el.currentTime);
  };

  const seekTo = (ratio: number) => {
    const el = audioRef.current;
    if (!el || !dur) return;
    const t = Math.min(dur, Math.max(0, ratio * dur));
    el.currentTime = t;
    setPos(t);
    setEnded(false);
  };

  if (!items.length) {
    return (
      <section>
        <p className="eyebrow mb-3">Try it</p>
        <div className="surface p-5">
          <Skeleton className="h-6 w-64" />
          <Skeleton className="mt-4 h-24 w-full" />
        </div>
      </section>
    );
  }

  const saved = hydrated && pos < 1 && !playing ? readNum(KEY(item.slug)) : 0;
  const progress = dur ? pos / dur : 0;

  return (
    <section>
      <p className="eyebrow mb-3">Try it, no account</p>
      <h2 className="font-display text-3xl text-bone-100 sm:text-4xl">
        Play a minute. Leave. Come back to the second.
      </h2>
      <p className="mt-3 max-w-xl text-sm text-bone-300">
        The opening minute of a real show. Wander off and reload the page &mdash; your spot is
        still here.
      </p>

      <div className="mt-6 flex flex-wrap gap-2">
        {items.map((it, i) => (
          <button
            key={it.slug}
            type="button"
            onClick={() => selectGenre(i)}
            aria-pressed={i === active}
            className={`rounded-full border px-3 py-1 text-sm transition ${
              i === active
                ? "border-signal/60 bg-signal/10 text-signal-soft"
                : "border-ink-600 text-bone-300 hover:border-ink-500 hover:text-bone-100"
            }`}
          >
            {it.genre?.name ?? it.slug}
          </button>
        ))}
      </div>

      <div className="surface mt-4 grid gap-5 p-5 sm:grid-cols-[minmax(0,1fr)_1.4fr] sm:p-6">
        <div className="flex gap-4">
          <div
            className="h-20 w-20 shrink-0 rounded-lg"
            style={coverStyle(item.show.accent_color || "#54d0cc", item.show.slug, item.show.cover_image_url)}
            aria-hidden
          />
          <div className="min-w-0">
            <p className="truncate font-display text-lg text-bone-100">{item.show.title}</p>
            <p className="text-xs uppercase tracking-[0.15em] text-bone-500">
              Episode 1 &middot; {item.genre?.name ?? item.slug}
            </p>
            <p className="mt-2 text-sm text-bone-300">{item.scene}</p>
          </div>
        </div>

        <div className="flex flex-col justify-center">
          {missing ? (
            <div className="text-sm text-bone-300">
              <p>The opening-minute clip for this one is on its way.</p>
              <Link
                href={`/shows/${item.show.slug}`}
                className="mt-2 inline-block text-signal-soft hover:text-signal"
              >
                Open the show &rarr;
              </Link>
            </div>
          ) : (
            <>
              {/* Waveform doubles as the scrubber. */}
              <button
                type="button"
                className="flex h-14 w-full items-end gap-[3px]"
                aria-label="Seek"
                onClick={(e) => {
                  const r = e.currentTarget.getBoundingClientRect();
                  seekTo((e.clientX - r.left) / r.width);
                }}
              >
                {bars.map((b, i) => (
                  <span
                    key={i}
                    className={`w-full rounded-sm transition-colors ${
                      i / bars.length <= progress ? "bg-signal/80" : "bg-ink-700"
                    }`}
                    style={{ height: `${Math.round(b * 100)}%` }}
                  />
                ))}
              </button>

              <div className="mt-3 flex items-center gap-3">
                <button
                  type="button"
                  onClick={() => seekTo((pos - 15) / (dur || 1))}
                  className="btn-quiet h-9 w-9 rounded-full p-0"
                  aria-label="Back 15 seconds"
                >
                  <Back15Icon className="h-4 w-4" />
                </button>
                <button
                  type="button"
                  onClick={toggle}
                  className="btn-primary h-10 w-10 rounded-full p-0"
                  aria-label={playing ? "Pause" : "Play"}
                >
                  {playing ? <PauseIcon className="h-4 w-4" /> : <PlayIcon className="h-4 w-4" />}
                </button>
                <button type="button" onClick={leave} className="btn-ghost text-sm">
                  Leave
                </button>
                <span className="ml-auto text-xs tabular-nums text-bone-400">
                  {formatDuration(pos)} / {formatDuration(dur)}
                </span>
              </div>

              <p className="mt-2 min-h-[1.25rem] text-xs text-bone-400">
                {ended ? (
                  <>
                    That&rsquo;s the minute.{" "}
                    <Link href={`/shows/${item.show.slug}`} className="text-signal-soft hover:text-signal">
                      Hear the rest &rarr;
                    </Link>
                  </>
                ) : saved > 0 ? (
                  <>Saved at {formatDuration(saved)} &mdash; press play to pick up.</>
                ) : (
                  " "
                )}
              </p>
            </>
          )}

          <audio
            key={item.slug}
            ref={audioRef}
            src={`/previews/${item.slug}.mp3`}
            preload="metadata"
            onLoadedMetadata={(e) => {
              const el = e.currentTarget;
              setDur(el.duration || 0);
              const resume = readNum(KEY(item.slug));
              if (resume > 0 && resume < el.duration) {
                el.currentTime = resume;
                setPos(resume);
              }
            }}
            onTimeUpdate={(e) => {
              const el = e.currentTarget;
              setPos(el.currentTime);
              const now = Date.now();
              if (playing && now - lastSaveRef.current > 5000) {
                lastSaveRef.current = now;
                save(item.slug, el.currentTime);
              }
            }}
            onEnded={() => {
              setPlaying(false);
              setEnded(true);
              remove(KEY(item.slug));
            }}
            onError={() => setMissing(true)}
          />
        </div>
      </div>
    </section>
  );
}
