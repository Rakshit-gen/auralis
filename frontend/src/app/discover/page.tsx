"use client";

import { Suspense, useEffect, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
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
  return (
    <Suspense fallback={<div className="container-page py-24" />}>
      <DiscoverInner />
    </Suspense>
  );
}

function DiscoverInner() {
  const params = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const [genre, setGenre] = useState<string>(params.get("genre") ?? "");
  const [language, setLanguage] = useState<string>(params.get("language") ?? "");
  const [sort, setSort] = useState(params.get("sort") ?? "recent");
  const [aiOnly, setAiOnly] = useState<boolean | undefined>(
    params.get("ai") === "true" ? true : params.get("ai") === "false" ? false : undefined,
  );

  // Keep the URL in step with the filters so the view is shareable and the
  // back button restores it.
  useEffect(() => {
    const q = new URLSearchParams();
    if (genre) q.set("genre", genre);
    if (language) q.set("language", language);
    if (sort !== "recent") q.set("sort", sort);
    if (aiOnly !== undefined) q.set("ai", String(aiOnly));
    const qs = q.toString();
    router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
  }, [genre, language, sort, aiOnly, pathname, router]);

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
