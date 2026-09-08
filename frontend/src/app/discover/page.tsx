"use client";

import { useState } from "react";
import { useGenres, useLanguages, useShows } from "@/lib/hooks";
import { ShowGrid } from "@/components/show-grid";
import { Chip } from "@/components/ui";

const SORTS = [
  { key: "recent", label: "Recently published" },
  { key: "popular", label: "Most episodes" },
  { key: "rating", label: "Highest rated" },
  { key: "title", label: "A to Z" },
];

export default function DiscoverPage() {
  const [genre, setGenre] = useState<string>("");
  const [language, setLanguage] = useState<string>("");
  const [sort, setSort] = useState("recent");
  const [aiOnly, setAiOnly] = useState<boolean | undefined>(undefined);

  const { data: genres } = useGenres();
  const { data: languages } = useLanguages();
  const { data, isLoading, error, refetch } = useShows({
    genre: genre || undefined,
    language: language || undefined,
    sort,
    ai: aiOnly,
    limit: 40,
  });

  return (
    <div className="container-page">
      <p className="eyebrow mb-1">Catalog</p>
      <h1 className="mb-6 font-display text-3xl text-bone-100">Discover</h1>

      <div className="mb-4 flex flex-wrap gap-2">
        <Chip active={!genre} onClick={() => setGenre("")}>
          All genres
        </Chip>
        {genres?.map((g) => (
          <Chip key={g.id} active={genre === g.slug} onClick={() => setGenre(g.slug)}>
            {g.name}
          </Chip>
        ))}
      </div>

      <div className="mb-6 flex flex-wrap items-center gap-3">
        <select value={language} onChange={(e) => setLanguage(e.target.value)} className="field w-auto">
          <option value="">All languages</option>
          {languages?.map((l) => (
            <option key={l.code} value={l.code}>
              {l.name}
            </option>
          ))}
        </select>
        <select value={sort} onChange={(e) => setSort(e.target.value)} className="field w-auto">
          {SORTS.map((s) => (
            <option key={s.key} value={s.key}>
              {s.label}
            </option>
          ))}
        </select>
        <div className="flex gap-2">
          <Chip active={aiOnly === undefined} onClick={() => setAiOnly(undefined)}>
            Everything
          </Chip>
          <Chip active={aiOnly === false} onClick={() => setAiOnly(false)}>
            Hand-authored
          </Chip>
          <Chip active={aiOnly === true} onClick={() => setAiOnly(true)}>
            AI series
          </Chip>
        </div>
      </div>

      {data && (
        <p className="mb-3 text-sm text-bone-400">
          {data.total} {data.total === 1 ? "show" : "shows"}
        </p>
      )}

      <ShowGrid
        shows={data?.shows}
        loading={isLoading}
        error={error ? (error as Error).message : null}
        retry={() => void refetch()}
        emptyTitle="No shows match these filters"
        emptyHint="Try widening the genre or language."
      />
    </div>
  );
}
