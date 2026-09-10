"use client";

import Link from "next/link";
import { RequireAuth } from "@/components/layout/require-auth";
import {
  useContinueListening,
  useFeed,
  useFollows,
  useShows,
  useTrending,
} from "@/lib/hooks";
import { useAuth } from "@/stores/auth";
import { ShowRail, ShowCardCompact } from "@/components/show-card";
import { formatCount } from "@/lib/format";
import { ContinueRow } from "@/components/continue-row";
import { SectionHeader, Spinner } from "@/components/ui";
import type { Show } from "@/lib/types";

const SOURCE_LABEL: Record<string, string> = {
  genre_affinity: "Because you like this genre",
  collaborative: "Listeners like you enjoyed this",
  trending: "Trending",
  your_preferences: "From your preferences",
  popular_and_recent: "Popular and recent",
};

function HomeInner() {
  const user = useAuth((s) => s.user);
  const { data: cont } = useContinueListening();
  const { data: feed, isLoading: feedLoading } = useFeed();
  const { data: trending } = useTrending();
  const { data: follows } = useFollows();
  const { data: recent } = useShows({ sort: "recent", limit: 15 });

  const feedShowIds = feed?.items.map((i) => i.show_id) ?? [];
  // Share the limit:100 catalog page that useTrending/useCoverLookup already
  // load; limit:40 dropped feed picks that sat outside the first 40 shows.
  const { data: feedShows } = useShows({ limit: 100 });
  const byId = new Map((feedShows?.shows ?? []).map((s) => [s.id, s]));
  const feedResolved: Show[] = feedShowIds.map((id) => byId.get(id)).filter(Boolean) as Show[];
  const reasons: Record<string, string> = {};
  feed?.items.forEach((i) => {
    reasons[i.show_id] = SOURCE_LABEL[i.sources[0]] ?? "Recommended for you";
  });

  return (
    <div className="container-page space-y-12">
      <div>
        <p className="eyebrow mb-1">Good to have you back</p>
        <h1 className="font-display text-3xl text-bone-100">{user?.display_name}</h1>
      </div>

      {!!cont?.length && (
        <section>
          <SectionHeader title="Continue listening" href="/library" />
          <div className="grid gap-3 sm:grid-cols-2">
            {cont.slice(0, 4).map((item) => (
              <ContinueRow key={item.episode_id} item={item} />
            ))}
          </div>
        </section>
      )}

      <section>
        <SectionHeader eyebrow={feed?.strategy === "personalized_hybrid" ? "Tuned to your listening" : "To get you started"} title="Your feed" href="/discover" />
        {feedLoading && <Spinner label="Building your feed" />}
        {feedResolved.length > 0 && <ShowRail shows={feedResolved} reasons={reasons} />}
        {!feedLoading && feed && feed.items.length === 0 && (
          <p className="text-sm text-bone-400">
            Listen to a few episodes and this feed will start reshaping itself.
          </p>
        )}
      </section>

      {!!recent?.shows.length && (
        <section>
          <SectionHeader title="Recently published" href="/discover" />
          <ShowRail shows={recent.shows.slice(0, 10)} />
        </section>
      )}

      <div className="grid gap-8 lg:grid-cols-2">
        <section>
          <SectionHeader title="Trending" href="/trending" />
          <div className="space-y-2">
            {(trending ?? []).slice(0, 5).map((t, i) => (
              <div key={t.show_id} className="flex items-center gap-3">
                <span className="w-6 text-right font-display text-lg text-ink-500">{i + 1}</span>
                <div className="flex-1">
                  <ShowCardCompact
                    title={t.title}
                    slug={t.slug}
                    meta={`${formatCount(t.plays)} plays`}
                    cover={t.cover_image_url}
                    accent={t.accent_color}
                  />
                </div>
              </div>
            ))}
          </div>
        </section>

        <section>
          <SectionHeader title="Shows you follow" href="/library" />
          {follows?.length ? (
            <p className="text-sm text-bone-300">
              You follow {follows.length} {follows.length === 1 ? "show" : "shows"}. New episodes show
              up in your feed first.
            </p>
          ) : (
            <p className="text-sm text-bone-400">
              Follow a show from its page to be notified in your feed when it publishes.{" "}
              <Link href="/discover" className="text-amber-soft">
                Find one
              </Link>
              .
            </p>
          )}
        </section>
      </div>
    </div>
  );
}

export default function HomePage() {
  return (
    <RequireAuth>
      <HomeInner />
    </RequireAuth>
  );
}
