"use client";

import { ShowCard } from "@/components/show-card";
import { Skeleton, EmptyState, ErrorState } from "@/components/ui";
import type { Show } from "@/lib/types";

export function ShowGrid({
  shows,
  loading,
  error,
  retry,
  emptyTitle = "Nothing here yet",
  emptyHint,
}: {
  shows?: Show[];
  loading?: boolean;
  error?: string | null;
  retry?: () => void;
  emptyTitle?: string;
  emptyHint?: string;
}) {
  if (error) return <ErrorState message={error} retry={retry} />;

  if (loading) {
    return (
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
        {Array.from({ length: 10 }).map((_, i) => (
          <Skeleton key={i} className="aspect-[3/4]" />
        ))}
      </div>
    );
  }

  if (!shows?.length) return <EmptyState title={emptyTitle} hint={emptyHint} />;

  return (
    <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
      {shows.map((s) => (
        <ShowCard key={s.id} show={s} />
      ))}
    </div>
  );
}
