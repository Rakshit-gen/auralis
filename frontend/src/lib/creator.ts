"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { useAuth } from "@/stores/auth";
import type { Episode, Season, Show } from "@/lib/types";

export function useMyShows() {
  const user = useAuth((s) => s.user);
  return useQuery({
    queryKey: ["my-shows", user?.id],
    enabled: !!user,
    queryFn: () => api<{ shows: Show[] }>("/content/creator/shows").then((r) => r.shows),
  });
}

export function useCreatorShow(showId: string) {
  return useQuery({
    queryKey: ["creator-show", showId],
    enabled: !!showId,
    queryFn: async () => {
      const show = await api<Show>(`/content/internal/shows/${showId}`).catch(() =>
        api<{ show: Show }>(`/content/shows/${showId}`).then((r) => r.show),
      );
      const [seasons, episodes] = await Promise.all([
        api<{ seasons: Season[] }>(`/content/shows/${showId}/seasons`).then((r) => r.seasons).catch(() => []),
        api<{ episodes: Episode[] }>(`/content/shows/${showId}/episodes`).then((r) => r.episodes).catch(() => []),
      ]);
      return { show, seasons, episodes };
    },
  });
}

export function useEpisodeDetail(episodeId: string) {
  return useQuery({
    queryKey: ["creator-episode", episodeId],
    enabled: !!episodeId,
    queryFn: () =>
      api<{ episode: Episode; show: Show }>(`/content/internal/episodes/${episodeId}`),
  });
}

export function useSubmitForReview() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { kind: "show" | "episode"; id: string }) =>
      api(`/content/${input.kind === "show" ? "shows" : "episodes"}/${input.id}/submit`, { method: "POST" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["creator-show"] });
      qc.invalidateQueries({ queryKey: ["my-shows"] });
    },
  });
}

export function useSaveScript() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { episodeId: string; script: string }) =>
      api(`/content/episodes/${input.episodeId}/script`, {
        method: "POST",
        body: { script: input.script },
      }),
    onSuccess: (_d, v) => qc.invalidateQueries({ queryKey: ["creator-episode", v.episodeId] }),
  });
}

export function useReviewHistory(showId: string) {
  return useQuery({
    queryKey: ["review-history", showId],
    enabled: !!showId,
    queryFn: () =>
      api<{ history: { action: string; notes: string; to_status: string; created_at: string }[] }>(
        `/content/shows/${showId}/review-history`,
      ).then((r) => r.history),
  });
}
