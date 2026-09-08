"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import {
  useBookmarks,
  useContinueListening,
  useFollows,
  useHistory,
  useLikes,
} from "@/lib/hooks";
import { ContinueRow } from "@/components/continue-row";
import { EmptyState, Spinner } from "@/components/ui";
import { coverStyle, formatDuration, relativeTime } from "@/lib/format";
import type { Episode } from "@/lib/types";

const TABS = [
  { key: "continue", label: "Continue" },
  { key: "bookmarks", label: "Bookmarks" },
  { key: "likes", label: "Likes" },
  { key: "history", label: "History" },
  { key: "following", label: "Following" },
] as const;

type Tab = (typeof TABS)[number]["key"];

function EpisodeTitle({ id, showId }: { id: string; showId: string }) {
  const { data } = useQuery({
    queryKey: ["episode", id],
    queryFn: () => api<Episode>(`/catalog/episodes/${id}`, { auth: false }),
    staleTime: 5 * 60_000,
  });
  return (
    <Link href={`/shows/${showId}`} className="min-w-0 flex-1">
      <p className="truncate font-display text-bone-100">{data?.title ?? "Episode"}</p>
      {data && <p className="text-xs text-bone-400">Episode {data.number}</p>}
    </Link>
  );
}

export function LibraryView({ initial }: { initial: Tab }) {
  const cont = useContinueListening();
  const bookmarks = useBookmarks();
  const likes = useLikes();
  const history = useHistory();
  const follows = useFollows();

  return (
    <div className="container-page">
      <p className="eyebrow mb-1">Your listening</p>
      <h1 className="mb-6 font-display text-3xl text-bone-100">Library</h1>

      <div className="mb-6 flex gap-1 overflow-x-auto border-b border-ink-800">
        {TABS.map((t) => (
          <Link
            key={t.key}
            href={`/library/${t.key === "continue" ? "" : t.key}`}
            className={`shrink-0 border-b-2 px-4 py-2 text-sm transition ${
              initial === t.key
                ? "border-amber text-amber-soft"
                : "border-transparent text-bone-300 hover:text-bone-100"
            }`}
          >
            {t.label}
          </Link>
        ))}
      </div>

      {initial === "continue" && (
        <Section loading={cont.isLoading}>
          {cont.data?.length ? (
            <div className="grid gap-3 sm:grid-cols-2">
              {cont.data.map((item) => (
                <ContinueRow key={item.episode_id} item={item} />
              ))}
            </div>
          ) : (
            <EmptyState title="Nothing in progress" hint="Start an episode and it will appear here." />
          )}
        </Section>
      )}

      {initial === "bookmarks" && (
        <Section loading={bookmarks.isLoading}>
          {bookmarks.data?.length ? (
            <div className="space-y-2">
              {bookmarks.data.map((b) => (
                <div key={b.episode_id} className="surface flex items-center gap-3 p-3">
                  <div className="h-11 w-11 shrink-0 rounded-lg" style={coverStyle("#d9963f", b.show_id)} />
                  <EpisodeTitle id={b.episode_id} showId={b.show_id} />
                  {b.note && <span className="hidden text-xs text-bone-400 sm:block">{b.note}</span>}
                </div>
              ))}
            </div>
          ) : (
            <EmptyState title="No bookmarks" hint="Bookmark an episode from a show page to save it." />
          )}
        </Section>
      )}

      {initial === "likes" && (
        <Section loading={likes.isLoading}>
          {likes.data?.length ? (
            <div className="space-y-2">
              {likes.data.map((l) => (
                <div key={`${l.target_type}-${l.target_id}`} className="surface flex items-center gap-3 p-3">
                  <div className="h-11 w-11 shrink-0 rounded-lg" style={coverStyle("#d9963f", l.show_id)} />
                  {l.target_type === "episode" ? (
                    <EpisodeTitle id={l.target_id} showId={l.show_id} />
                  ) : (
                    <Link href={`/shows/${l.show_id}`} className="flex-1 font-display text-bone-100">
                      Show
                    </Link>
                  )}
                  <span className="tag">{l.target_type}</span>
                </div>
              ))}
            </div>
          ) : (
            <EmptyState title="No likes yet" hint="Tap the heart on any show or episode." />
          )}
        </Section>
      )}

      {initial === "history" && (
        <Section loading={history.isLoading}>
          {history.data?.length ? (
            <div className="space-y-2">
              {history.data.map((h) => (
                <div key={h.episode_id} className="surface flex items-center gap-3 p-3">
                  <div className="h-11 w-11 shrink-0 rounded-lg" style={coverStyle("#d9963f", h.show_id)} />
                  <EpisodeTitle id={h.episode_id} showId={h.show_id} />
                  <div className="text-right text-xs text-bone-400">
                    <p>{h.completed ? "Finished" : `${formatDuration(h.position_sec)} in`}</p>
                    <p>{relativeTime(h.updated_at)}</p>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <EmptyState title="No history" hint="Episodes you play show up here." />
          )}
        </Section>
      )}

      {initial === "following" && (
        <Section loading={follows.isLoading}>
          {follows.data?.length ? (
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {follows.data.map((f) => (
                <Link
                  key={f.show_id}
                  href={`/shows/${f.show_id}`}
                  className="surface flex items-center gap-3 p-3 hover:border-amber/50"
                >
                  <div className="h-12 w-12 shrink-0 rounded-lg" style={coverStyle("#d9963f", f.show_id)} />
                  <span className="font-display text-bone-100">Followed show</span>
                </Link>
              ))}
            </div>
          ) : (
            <EmptyState title="Not following anything" hint="Follow a show to get its new episodes in your feed." />
          )}
        </Section>
      )}
    </div>
  );
}

function Section({ loading, children }: { loading: boolean; children: React.ReactNode }) {
  if (loading) return <Spinner />;
  return <>{children}</>;
}
