"use client";

import { Suspense, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useSearch } from "@/lib/hooks";
import { ShowGrid } from "@/components/show-grid";
import { SearchIcon } from "@/components/icons";

function SearchInner() {
  const params = useSearchParams();
  const router = useRouter();
  const initial = params.get("q") ?? "";
  const [q, setQ] = useState(initial);
  const [debouncedQ, setDebouncedQ] = useState(initial);

  useEffect(() => setQ(initial), [initial]);

  // Hold off on querying until typing pauses.
  useEffect(() => {
    const id = setTimeout(() => setDebouncedQ(q), 250);
    return () => clearTimeout(id);
  }, [q]);

  const { data, isLoading, error, refetch, isFetched } = useSearch(debouncedQ);

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    router.replace(`/search?q=${encodeURIComponent(q.trim())}`);
  };

  return (
    <div className="container-page">
      <form onSubmit={submit} className="mb-6">
        <div className="relative max-w-xl">
          <SearchIcon className="pointer-events-none absolute left-3 top-1/2 h-5 w-5 -translate-y-1/2 text-ink-500" />
          <input
            autoFocus
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Search titles, tags, synopses"
            aria-label="Search"
            className="field py-3 pl-11 text-base"
          />
        </div>
      </form>

      {q.trim().length < 2 ? (
        <p className="text-sm text-bone-400">Type at least two characters to search the catalog.</p>
      ) : (
        <>
          {data && (
            <p className="mb-4 text-sm text-bone-400">
              {data.total} {data.total === 1 ? "result" : "results"} for &ldquo;{debouncedQ}&rdquo;
              {typeof data.took_ms === "number" && ` · ${data.took_ms}ms`}
            </p>
          )}
          <ShowGrid
            shows={data?.shows}
            loading={isLoading && !isFetched}
            error={error ? (error as Error).message : null}
            retry={() => void refetch()}
            emptyTitle="No matches"
            emptyHint="Try a different word or browse by genre."
          />
        </>
      )}
    </div>
  );
}

export default function SearchPage() {
  return (
    <Suspense fallback={<div className="container-page py-24" />}>
      <SearchInner />
    </Suspense>
  );
}
