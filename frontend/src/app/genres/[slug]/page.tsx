"use client";

import { use } from "react";
import { useGenres, useShows } from "@/lib/hooks";
import { ShowGrid } from "@/components/show-grid";

export default function GenrePage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = use(params);
  const { data: genres } = useGenres();
  const genre = genres?.find((g) => g.slug === slug);
  const { data, isLoading, error, refetch } = useShows({ genre: slug, sort: "recent", limit: 40 });

  return (
    <div className="container-page">
      <p className="eyebrow mb-1">Genre</p>
      <h1 className="font-display text-3xl text-bone-100">{genre?.name ?? slug}</h1>
      {genre?.description && <p className="mb-6 mt-2 max-w-xl text-sm text-bone-300">{genre.description}</p>}
      <div className="mt-6">
        <ShowGrid
          shows={data?.shows}
          loading={isLoading}
          error={error ? (error as Error).message : null}
          retry={() => void refetch()}
          emptyTitle="No published shows in this genre yet"
        />
      </div>
    </div>
  );
}
