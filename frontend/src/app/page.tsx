"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/stores/auth";
import { useShows, useTrending } from "@/lib/hooks";
import { coverStyle, formatCount } from "@/lib/format";
import { SoundPortal } from "@/components/sound-portal";
import { CreatorPipeline } from "@/components/landing-sections";
import { PlayAMinute } from "@/components/play-a-minute";
import { Skeleton } from "@/components/ui";
import type { Show } from "@/lib/types";

const BRIEF_SAMPLES = [
  "A lighthouse keeper starts receiving weather reports for a coast that no longer exists.",
  "Two rival tide-pool researchers share one field station and one radio.",
  "A salvage diver keeps finding the same shipwreck in different oceans.",
];

export default function Landing() {
  const { user, ready } = useAuth();
  const router = useRouter();
  const { data: trending, isLoading, isError, isFetching } = useTrending();
  const trendingWarming = isLoading || (isError && isFetching);
  const { data: catalog } = useShows({ limit: 24 });
  // Everything the trending rail isn't already showing.
  const pool = useMemo(() => {
    const onTrending = new Set((trending ?? []).map((t) => t.slug));
    return (catalog?.shows ?? []).filter((s) => !onTrending.has(s.slug));
  }, [catalog, trending]);
  // Three of them, shuffled once when the pool first arrives and then left
  // alone. `trending`/`catalog` hand back fresh array identities on every
  // render, so re-picking on each `pool` change would reshuffle forever.
  const [picks, setPicks] = useState<Show[]>([]);
  useEffect(() => {
    if (pool.length && picks.length === 0) {
      setPicks([...pool].sort(() => Math.random() - 0.5).slice(0, 3));
    }
  }, [pool, picks.length]);

  useEffect(() => {
    if (ready && user) router.replace("/home");
  }, [ready, user, router]);

  return (
    <div className="landing-page container-page space-y-20 pb-12 pt-2 sm:space-y-24 lg:pb-16 lg:pt-4">
      <section className="landing-hero relative grid items-center gap-10 pb-10 lg:grid-cols-[1.1fr_0.9fr] lg:gap-12 lg:pb-12 lg:pt-6">
        <div className="relative">
          <h1 className="font-display text-bone-100">
            Audio fiction that remembers exactly where you stopped.
          </h1>
          <p className="mt-7 max-w-md text-lg text-bone-200">
            Original serialized shows, released episode by episode. Stop mid-scene on your phone and
            pick up at the same second on your laptop.
          </p>
          <div className="mt-9 flex flex-wrap gap-3">
            <Link href="/register" className="btn-primary">
              Create a free account
            </Link>
            <Link href="/discover" className="btn-ghost">
              Browse the catalog
            </Link>
          </div>

          <div className="mt-12" hidden={!!catalog && pool.length === 0}>
            <p className="text-sm text-bone-400">Three to start with</p>
            <div className="mt-4 grid gap-3 sm:grid-cols-3">
              {picks.length
                ? picks.map((s) => (
                    <Link
                      key={s.id}
                      href={`/shows/${s.slug}`}
                      className="rounded-xl border border-ink-700 bg-ink-950/60 p-3 transition-colors hover:border-signal/40"
                    >
                      <div
                        className="mb-3 h-20 rounded-lg"
                        style={coverStyle(s.accent_color || "#54d0cc", s.slug, s.cover_image_url)}
                        aria-hidden
                      />
                      <p className="truncate font-display text-bone-100">{s.title}</p>
                      <p className="mt-0.5 text-xs text-bone-400">
                        {s.episode_count} {s.episode_count === 1 ? "episode" : "episodes"}
                      </p>
                    </Link>
                  ))
                : Array.from({ length: 3 }).map((_, i) => (
                    <div key={i} className="rounded-xl border border-ink-800 p-3" aria-hidden>
                      <Skeleton className="mb-3 h-20" />
                      <Skeleton className="h-4 w-3/4" />
                    </div>
                  ))}
            </div>
          </div>
        </div>

        <aside className="hero-listening relative">
          <SoundPortal />
          <div className="trending-panel">
          <div className="flex items-baseline justify-between">
            <p className="font-display text-xl text-bone-100">Trending</p>
            <Link href="/trending" className="text-xs text-signal-soft hover:text-signal">
              All
            </Link>
          </div>
          <p className="mt-0.5 text-xs text-bone-400">Ranked by recent plays</p>
          <ol className="mt-4">
            {trendingWarming
              ? Array.from({ length: 3 }).map((_, i) => (
                  <li
                    key={i}
                    className="flex items-center gap-3 border-t border-white/5 py-3 first:border-t-0"
                    aria-hidden
                  >
                    <Skeleton className="h-4 w-3" />
                    <Skeleton className="h-10 w-10 shrink-0" />
                    <div className="flex-1 space-y-1.5">
                      <Skeleton className="h-3.5 w-2/3" />
                      <Skeleton className="h-3 w-14" />
                    </div>
                  </li>
                ))
              : (trending ?? []).slice(0, 3).map((t, i) => (
                  <li key={t.show_id} className="border-t border-white/5 first:border-t-0">
                    <Link
                      href={`/shows/${t.slug}`}
                      className="group flex items-center gap-3 py-3"
                    >
                      <span className="w-3 shrink-0 text-center font-display text-lg text-signal">
                        {i + 1}
                      </span>
                      <div
                        className="h-10 w-10 shrink-0 rounded-md"
                        style={coverStyle(t.accent_color || "#54d0cc", t.slug, t.cover_image_url)}
                        aria-hidden
                      />
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm text-bone-100 group-hover:text-signal-soft">
                          {t.title}
                        </p>
                        <p className="text-xs tabular-nums text-bone-400">
                          {formatCount(t.plays)} plays
                        </p>
                      </div>
                    </Link>
                  </li>
                ))}
          </ol>
          {!trendingWarming && !trending?.length && (
            <p className="py-8 text-center text-sm text-bone-400">The catalog is warming up.</p>
          )}
          </div>
        </aside>
      </section>

      <PlayAMinute />

      {/* The story studio. Signal-teal border marks it as the AI part of the app. */}
      <section className="rounded-2xl border border-signal/30 bg-ink-900/80 p-8 sm:p-12">
        <div className="grid gap-8 lg:grid-cols-[1fr_0.85fr] lg:items-center">
          <div>
            <p className="eyebrow mb-3">The story studio</p>
            <h2 className="font-display text-3xl text-bone-100 sm:text-4xl">
              Turn a one-line idea into a season you can listen to.
            </h2>
            <p className="mt-4 max-w-md text-bone-300">
              The studio writes a story bible, outlines every episode, drafts the scripts and voices
              each character. Nothing goes live until you have read it and published it yourself.
            </p>
            <div className="mt-6 flex flex-wrap gap-3">
              <Link href="/register" className="btn-tide">
                Create an account to try it
              </Link>
              <Link href="/discover" className="btn-quiet text-signal-soft">
                Hear what people have made
              </Link>
            </div>
          </div>
          <div className="surface space-y-2 p-4">
            <p className="text-sm text-bone-400">Example briefs</p>
            {BRIEF_SAMPLES.map((s) => (
              <Link
                key={s}
                href="/register"
                className="block rounded-lg border border-ink-700 bg-ink-950/60 p-3 text-sm text-bone-200 transition hover:border-signal/40 hover:text-bone-100"
              >
                &ldquo;{s}&rdquo;
              </Link>
            ))}
          </div>
        </div>
      </section>

      <CreatorPipeline />

      <section className="border-t border-ink-800 pt-8">
        <h2 className="font-display text-xl text-bone-100">Also in Auralis</h2>
        <ul className="mt-4 divide-y divide-ink-800">
          {[
            ["Discover", "Filter the catalog by genre, language and mood.", "/discover"],
            ["Continue listening", "Every unfinished episode on one shelf.", "/library"],
            ["Premium", "Premium series, unlocked with a promo code.", "/premium"],
            ["Creator tools", "Build shows, upload your own audio, see per-episode analytics.", "/register"],
          ].map(([title, copy, href]) => (
            <li key={title}>
              <Link
                href={href}
                className="group flex flex-col gap-1 py-3 sm:flex-row sm:items-baseline sm:gap-6"
              >
                <span className="text-bone-100 group-hover:text-amber-soft sm:w-48">{title}</span>
                <span className="text-sm text-bone-400">{copy}</span>
              </Link>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
