"use client";

import Link from "next/link";
import { useTrending, useShows } from "@/lib/hooks";
import { coverStyle, formatCount } from "@/lib/format";
import { Spinner } from "@/components/ui";

export default function TrendingPage() {
  const { data: trending, isLoading } = useTrending();
  const { data: popular } = useShows({ sort: "popular", limit: 12 });

  return (
    <div className="container-page space-y-10">
      <div>
        <p className="eyebrow mb-1">What listeners are on right now</p>
        <h1 className="font-display text-3xl text-bone-100">Trending</h1>
      </div>

      {isLoading && <Spinner />}

      <ol className="space-y-2">
        {(trending ?? []).map((t, i) => (
          <li key={t.show_id}>
            <Link
              href={`/shows/${t.slug}`}
              className="surface flex items-center gap-4 p-4 transition hover:border-amber/50"
            >
              <span className="w-8 text-center font-display text-2xl text-ink-500">{i + 1}</span>
              <div
                className="h-12 w-12 shrink-0 rounded-lg"
                style={coverStyle(t.accent_color || "#d9963f", t.slug, t.cover_image_url)}
                aria-hidden
              />
              <div className="min-w-0 flex-1">
                <p className="truncate font-display text-lg text-bone-100">{t.title}</p>
                <p className="text-xs text-bone-400">{formatCount(t.plays)} plays</p>
              </div>
            </Link>
          </li>
        ))}
      </ol>

      {!!popular?.shows.length && (
        <section>
          <h2 className="mb-4 font-display text-xl text-bone-100">Most episodes</h2>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {popular.shows.map((s) => (
              <Link key={s.id} href={`/shows/${s.slug}`} className="surface p-4 hover:border-amber/50">
                <p className="font-display text-bone-100">{s.title}</p>
                <p className="mt-1 text-xs text-bone-400">{s.episode_count} episodes</p>
              </Link>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
