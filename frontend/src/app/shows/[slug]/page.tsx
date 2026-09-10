"use client";

import { use, useMemo } from "react";
import Link from "next/link";
import { useShow, useSimilar, useFollows, useLikes, useToggleFollow, useToggleLike, useContinueListening } from "@/lib/hooks";
import { useAuth } from "@/stores/auth";
import { EpisodeRow } from "@/components/episode-row";
import { ShowCardCompact } from "@/components/show-card";
import { Spinner, ErrorState } from "@/components/ui";
import { HeartIcon, PlusIcon, SparkIcon } from "@/components/icons";
import { coverStyle, formatRuntime } from "@/lib/format";

export default function ShowPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = use(params);
  const user = useAuth((s) => s.user);
  const { data, isLoading, error, refetch } = useShow(slug);
  const { data: similar } = useSimilar(data?.show.id ?? "");
  const { data: follows } = useFollows();
  const { data: likes } = useLikes();
  const { data: cont } = useContinueListening();
  const toggleFollow = useToggleFollow();
  const toggleLike = useToggleLike();

  const resumeByEpisode = useMemo(() => {
    const m = new Map<string, number>();
    cont?.forEach((c) => m.set(c.episode_id, c.position_sec));
    return m;
  }, [cont]);

  if (isLoading) {
    return (
      <div className="container-page py-16">
        <Spinner label="Loading show" />
      </div>
    );
  }
  if (error || !data) {
    return (
      <div className="container-page py-16">
        <ErrorState message={(error as Error)?.message ?? "Show not found"} retry={() => void refetch()} />
      </div>
    );
  }

  const { show, seasons, episodes } = data;
  const following = !!follows?.some((f) => f.show_id === show.id);
  const liked = !!likes?.some((l) => l.target_type === "show" && l.target_id === show.id);
  const bySeasons = seasons.length > 1;

  return (
    <div>
      <div
        className="relative border-b border-ink-800"
        style={coverStyle(show.accent_color || "#d9963f", show.id, show.cover_image_url)}
      >
        <div className="absolute inset-0 bg-grain opacity-[0.06]" />
        <div className="absolute inset-0 bg-gradient-to-t from-ink-950 via-ink-950/70 to-ink-950/20" />
        <div className="container-page relative py-12 lg:py-16">
          <div className="flex flex-col gap-8 lg:flex-row lg:items-end">
            <div
              className="hidden h-56 w-44 shrink-0 rounded-xl border border-ink-700 shadow-lift sm:block"
              style={coverStyle(show.accent_color || "#d9963f", show.id, show.cover_image_url)}
            />
            <div className="min-w-0">
              <div className="mb-2 flex flex-wrap items-center gap-2">
                {show.genres?.map((g) => (
                  <Link key={g.id} href={`/genres/${g.slug}`} className="tag hover:border-amber/50">
                    {g.name}
                  </Link>
                ))}
                {show.is_premium && <span className="tag border-amber/60 text-amber-soft">Premium</span>}
                {show.ai_generated && (
                  <span className="tag border-signal/50 text-signal">
                    <SparkIcon className="mr-1 h-3 w-3" /> AI series
                  </span>
                )}
              </div>
              <h1 className="font-display text-3xl text-bone-100 sm:text-4xl">{show.title}</h1>
              <p className="mt-2 max-w-2xl text-bone-200">{show.synopsis}</p>
              <p className="mt-3 text-sm text-bone-400">
                {show.creator_name && <>By {show.creator_name} · </>}
                {show.episode_count} episodes · {formatRuntime(show.total_duration_sec)}
                {show.rating_count > 0 && <> · {show.rating_avg.toFixed(1)}★ ({show.rating_count})</>}
              </p>

              {user && (
                <div className="mt-5 flex flex-wrap gap-3">
                  <button
                    onClick={() => toggleFollow.mutate({ showId: show.id, following })}
                    className={following ? "btn-ghost" : "btn-primary"}
                  >
                    <PlusIcon className="h-4 w-4" />
                    {following ? "Following" : "Follow"}
                  </button>
                  <button
                    onClick={() => toggleLike.mutate({ type: "show", id: show.id, showId: show.id, liked })}
                    className="btn-ghost"
                  >
                    <HeartIcon filled={liked} className="h-4 w-4" />
                    {liked ? "Liked" : "Like"}
                  </button>
                </div>
              )}
            </div>
          </div>
        </div>
      </div>

      <div className="container-page grid gap-10 py-10 lg:grid-cols-[1fr_300px]">
        <div>
          {show.description && (
            <div className="surface mb-6 p-5">
              <p className="whitespace-pre-line text-sm text-bone-200">{show.description}</p>
            </div>
          )}

          <h2 className="mb-4 font-display text-2xl text-bone-100">Episodes</h2>

          {episodes.length === 0 && (
            <p className="text-sm text-bone-400">No episodes have been published for this show yet.</p>
          )}

          {bySeasons
            ? seasons.map((season) => {
                const eps = episodes.filter((e) => e.season_id === season.id);
                if (!eps.length) return null;
                return (
                  <div key={season.id} className="mb-8">
                    <h3 className="mb-3 font-display text-lg text-amber-soft">{season.title}</h3>
                    <div className="space-y-3">
                      {eps.map((e) => (
                        <EpisodeRow
                          key={e.id}
                          episode={e}
                          show={show}
                          queue={episodes}
                          resumeSec={resumeByEpisode.get(e.id)}
                        />
                      ))}
                    </div>
                  </div>
                );
              })
            : (
              <div className="space-y-3">
                {episodes.map((e) => (
                  <EpisodeRow
                    key={e.id}
                    episode={e}
                    show={show}
                    queue={episodes}
                    resumeSec={resumeByEpisode.get(e.id)}
                  />
                ))}
              </div>
            )}
        </div>

        <aside className="space-y-6">
          {!!show.tags.length && (
            <div>
              <p className="eyebrow mb-2">Tags</p>
              <div className="flex flex-wrap gap-1.5">
                {show.tags.map((t) => (
                  <span key={t} className="tag">
                    {t}
                  </span>
                ))}
              </div>
            </div>
          )}
          {!!similar?.length && (
            <div>
              <p className="eyebrow mb-2">Listeners also enjoyed</p>
              <div className="space-y-2">
                {similar.slice(0, 5).map((s) => (
                  <ShowCardCompact key={s.show_id} title={s.title} slug={s.slug} />
                ))}
              </div>
            </div>
          )}
        </aside>
      </div>
    </div>
  );
}
