"use client";

import { use, useEffect } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { usePlayer } from "@/stores/player";
import { useAuth } from "@/stores/auth";
import { useAuthPrompt } from "@/stores/auth-prompt";
import { Spinner, ErrorState } from "@/components/ui";
import type { Episode, Show } from "@/lib/types";

/**
 * Deep link to a single episode: start playback, then send the listener to the
 * show page where the full episode list lives.
 */
function EpisodeDeepLink({ id }: { id: string }) {
  const router = useRouter();
  const playNow = usePlayer((s) => s.playNow);
  const user = useAuth((s) => s.user);
  const authReady = useAuth((s) => s.ready);
  const promptSignIn = useAuthPrompt((s) => s.show);

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ["episode-deep", id],
    queryFn: async () => {
      const episode = await api<Episode>(`/catalog/episodes/${id}`, { auth: false });
      const show = await api<{ show: Show }>(`/catalog/shows/${episode.show_id}`, { auth: false })
        .then((r) => r.show)
        .catch(() => null);
      return { episode, show };
    },
  });

  useEffect(() => {
    if (!data?.episode || !authReady) return;
    if (!user) {
      promptSignIn("Sign in to play this episode.");
      router.replace(data.show?.slug ? `/shows/${data.show.slug}` : "/discover");
      return;
    }
    void playNow({
      episodeId: data.episode.id,
      showId: data.episode.show_id,
      showSlug: data.show?.slug ?? "",
      showTitle: data.show?.title ?? "",
      episodeTitle: data.episode.title,
      episodeNumber: data.episode.number,
    });
    router.replace(data.show?.slug ? `/shows/${data.show.slug}` : "/discover");
  }, [data, playNow, router, user, authReady, promptSignIn]);

  if (isLoading) return <Spinner label="Starting playback" />;
  if (error || !data?.episode) {
    return <ErrorState message={(error as Error)?.message ?? "Episode not found"} retry={() => void refetch()} />;
  }
  return <Spinner label="Opening the show" />;
}

export default function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  return (
    <div className="container-page py-24">
      <EpisodeDeepLink id={id} />
    </div>
  );
}
