"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/stores/auth";
import { useShows, useTrending } from "@/lib/hooks";
import { coverStyle, formatCount } from "@/lib/format";
import { LogoMark } from "@/components/logo";
import { SparkIcon } from "@/components/icons";
import { CreatorPipeline, GenreStrip, HowItWorks } from "@/components/landing-sections";
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
  // alone — `trending`/`catalog` hand back fresh array identities on every
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
    <div className="container-page space-y-20 pb-12 pt-2 sm:space-y-24 lg:pb-16 lg:pt-4">
      <section className="relative grid items-start gap-12 pb-10 lg:grid-cols-[1.15fr_0.85fr] lg:gap-10 lg:pb-20 lg:pt-6">
        {/* A cool glow low-left and a warm one behind the headline, so the
            bioluminescent backdrop reads as depth rather than noise. */}
        <div
          aria-hidden
          className="pointer-events-none absolute -left-32 -top-24 h-80 w-80 rounded-full bg-signal/10 blur-3xl"
        />
        <div
          aria-hidden
          className="pointer-events-none absolute right-0 top-40 h-72 w-72 rounded-full bg-amber/[0.07] blur-3xl"
        />

        <div className="relative animate-fade-up">
          <p className="eyebrow mb-5">Serial audio fiction</p>
          <h1 className="font-display text-5xl leading-[1.02] tracking-tight text-bone-100 sm:text-6xl lg:text-[4.5rem]">
            Stories that keep going after the headphones come off.
          </h1>
          <p className="mt-7 max-w-md text-lg text-bone-200">
            Follow a show. It remembers the exact second you stopped, on every device you own.
          </p>
          <div className="mt-9 flex flex-wrap gap-3">
            <Link href="/register" className="btn-primary">
              Start listening &mdash; free
            </Link>
            <Link href="/discover" className="btn-ghost">
              Browse the catalog
            </Link>
          </div>

          <div
            className="mt-12 animate-fade-up"
            style={{ animationDelay: "80ms" }}
            hidden={!!catalog && pool.length === 0}
          >
            <p className="text-sm text-bone-400">You&rsquo;ll love</p>
            <div className="mt-4 grid gap-3 sm:grid-cols-3">
              {picks.length
                ? picks.map((s) => (
                    <Link
                      key={s.id}
                      href={`/shows/${s.slug}`}
                      className="rounded-xl border border-signal/15 bg-ink-950/50 p-3 shadow-[0_0_28px_-12px_rgba(84,208,204,0.4)] backdrop-blur transition hover:border-signal/40 hover:-translate-y-0.5"
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

        <aside
          className="relative animate-fade-up rounded-2xl border border-white/10 bg-ink-950/60 p-5 backdrop-blur-md lg:sticky lg:top-24"
          style={{ animationDelay: "120ms" }}
        >
          <div className="flex items-baseline justify-between">
            <p className="font-display text-xl text-bone-100">Trending tonight</p>
            <Link href="/trending" className="text-xs text-signal-soft hover:text-signal">
              All
            </Link>
          </div>
          <p className="mt-0.5 text-[11px] uppercase tracking-[0.15em] text-bone-500">
            What listeners are on right now
          </p>
          <ol className="mt-4">
            {trendingWarming
              ? Array.from({ length: 6 }).map((_, i) => (
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
              : (trending ?? []).slice(0, 6).map((t, i) => (
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
        </aside>
      </section>

      <HowItWorks />
      <GenreStrip />

      {/* The AI studio: the loudest thing on the page after the headline. */}
      <section className="relative overflow-hidden rounded-2xl border border-signal/30 bg-ink-900/70 p-8 backdrop-blur animate-tide-in sm:p-12">
        <div className="pointer-events-none absolute -right-16 -top-16 h-64 w-64 rounded-full bg-signal/10 blur-3xl" />
        <div className="relative grid gap-8 lg:grid-cols-[1fr_0.85fr] lg:items-center">
          <div>
            <p className="eyebrow mb-3 flex items-center gap-2">
              <LogoMark className="h-4 w-4" /> Build it yourself
            </p>
            <h2 className="font-display text-3xl text-bone-100 sm:text-4xl">
              Give it <span className="text-tide">one sentence</span>. Get back a whole season.
            </h2>
            <p className="mt-4 max-w-md text-bone-300">
              The pipeline drafts every script and voices the audio. You review it and decide what
              gets published.
            </p>
            <div className="mt-6 flex flex-wrap gap-3">
              <Link href="/register" className="btn-tide">
                <SparkIcon className="h-4 w-4" /> Start with a free account
              </Link>
              <Link href="/discover" className="btn-quiet text-signal-soft">
                Hear what people have made
              </Link>
            </div>
          </div>
          <div className="surface space-y-2 p-4">
            <p className="text-xs uppercase tracking-[0.2em] text-bone-400">Try a brief</p>
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

      <section>
        <div className="grid gap-8 sm:grid-cols-2 lg:grid-cols-4">
          {[
            ["Discover", "Filter by genre, language, and mood.", "/discover"],
            ["Continue Listening", "Every unfinished episode, in one shelf.", "/library"],
            ["Premium", "Unlock premium series with a promo code.", "/premium"],
            ["Creator tools", "Build shows, upload audio, watch analytics.", "/register"],
          ].map(([title, copy, href]) => (
            <Link key={title} href={href} className="surface-interactive p-5">
              <p className="font-display text-lg text-bone-100">{title}</p>
              <p className="mt-1 text-sm text-bone-300">{copy}</p>
            </Link>
          ))}
        </div>
      </section>
    </div>
  );
}
