"use client";

import Link from "next/link";
import { useGenres } from "@/lib/hooks";
import { Skeleton } from "@/components/ui";

export default function GenresPage() {
  const { data: genres, isLoading } = useGenres();

  return (
    <div className="container-page">
      <p className="eyebrow mb-1">Browse by</p>
      <h1 className="mb-6 font-display text-3xl text-bone-100">Genres</h1>

      {isLoading ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 9 }).map((_, i) => (
            <Skeleton key={i} className="h-28" />
          ))}
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {genres?.map((g, i) => {
            const hue = (i * 47) % 360;
            return (
              <Link
                key={g.id}
                href={`/genres/${g.slug}`}
                className="group relative overflow-hidden rounded-xl border border-ink-700 p-5 transition hover:-translate-y-0.5"
                style={{
                  backgroundImage: `linear-gradient(140deg, hsl(${hue} 40% 18%), #12100e 70%)`,
                }}
              >
                <div className="absolute inset-0 bg-grain opacity-[0.05]" />
                <p className="font-display text-xl text-bone-100">{g.name}</p>
                <p className="mt-1 line-clamp-2 text-sm text-bone-300">{g.description || "Original serialized audio."}</p>
                <span className="mt-3 inline-block text-sm text-amber-soft opacity-0 transition group-hover:opacity-100">
                  Explore
                </span>
              </Link>
            );
          })}
        </div>
      )}
    </div>
  );
}
