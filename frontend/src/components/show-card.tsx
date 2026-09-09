"use client";

import Link from "next/link";
import { coverStyle, formatCount } from "@/lib/format";
import type { Show } from "@/lib/types";
import { SparkIcon, WaveIcon } from "@/components/icons";

export function ShowCard({ show, reason }: { show: Show; reason?: string }) {
  return (
    <Link
      href={`/shows/${show.slug}`}
      className="group block animate-fade-up focus-visible:outline focus-visible:outline-2 focus-visible:outline-amber"
    >
      <div
        className="relative aspect-[3/4] overflow-hidden rounded-xl border border-ink-700 shadow-lift transition duration-200 group-hover:-translate-y-1 group-hover:border-ink-600"
        style={coverStyle(show.accent_color || "#d9963f", show.id, show.cover_image_url)}
      >
        <div className="absolute inset-0 bg-grain opacity-[0.06]" />
        <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-ink-950 via-ink-950/60 to-transparent p-3">
          <p className="line-clamp-2 font-display text-lg leading-tight text-bone-100">{show.title}</p>
          <p className="mt-1 text-xs text-bone-300">
            {show.episode_count} {show.episode_count === 1 ? "episode" : "episodes"}
            {show.rating_count > 0 && ` · ${show.rating_avg.toFixed(1)}★`}
          </p>
        </div>
        <div className="absolute left-2 top-2 flex gap-1.5">
          {show.is_premium && <span className="tag border-amber/60 bg-ink-950/70 text-amber-soft">Premium</span>}
          {show.ai_generated && (
            <span className="tag border-signal/50 bg-ink-950/70 text-signal">
              <SparkIcon className="mr-1 h-3 w-3" /> AI
            </span>
          )}
        </div>
        <div className="absolute right-2 top-2 rounded-full bg-ink-950/60 p-1.5 text-amber opacity-0 transition group-hover:opacity-100">
          <WaveIcon className="h-4 w-4" />
        </div>
      </div>
      {reason && <p className="mt-2 px-0.5 text-xs text-bone-400">{reason}</p>}
    </Link>
  );
}

export function ShowCardCompact({
  title,
  slug,
  meta,
  cover,
  accent,
}: {
  title: string;
  slug: string;
  meta?: string;
  cover?: string | null;
  accent?: string | null;
}) {
  return (
    <Link href={`/shows/${slug}`} className="surface-interactive flex items-center gap-3 p-3">
      <div
        className="h-12 w-12 shrink-0 rounded-lg"
        style={coverStyle(accent || "#d9963f", slug, cover)}
        aria-hidden
      />
      <div className="min-w-0">
        <p className="truncate font-display text-bone-100">{title}</p>
        {meta && <p className="truncate text-xs text-bone-400">{meta}</p>}
      </div>
    </Link>
  );
}

export function ShowRail({ shows, reasons }: { shows: Show[]; reasons?: Record<string, string> }) {
  return (
    <div className="-mx-4 flex gap-4 overflow-x-auto px-4 pb-2 sm:mx-0 sm:grid sm:grid-cols-3 sm:overflow-visible sm:px-0 md:grid-cols-4 lg:grid-cols-5">
      {shows.map((s) => (
        <div key={s.id} className="w-40 shrink-0 sm:w-auto">
          <ShowCard show={s} reason={reasons?.[s.id]} />
        </div>
      ))}
    </div>
  );
}

export { formatCount };
