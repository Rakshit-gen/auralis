"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { useAuth } from "@/stores/auth";
import type {
  ContinueItem,
  Entitlement,
  Episode,
  FeedItem,
  Genre,
  Language,
  Season,
  Show,
} from "@/lib/types";

// --- catalog ---

export function useGenres() {
  return useQuery({
    queryKey: ["genres"],
    queryFn: () => api<{ genres: Genre[] }>("/catalog/genres", { auth: false }).then((r) => r.genres),
    staleTime: 10 * 60_000,
  });
}

export function useLanguages() {
  return useQuery({
    queryKey: ["languages"],
    queryFn: () =>
      api<{ languages: Language[] }>("/catalog/languages", { auth: false }).then((r) => r.languages),
    staleTime: 10 * 60_000,
  });
}

export type ShowQuery = {
  genre?: string;
  language?: string;
  sort?: string;
  q?: string;
  premium?: boolean;
  ai?: boolean;
  limit?: number;
  offset?: number;
};

export function useShows(params: ShowQuery = {}) {
  return useQuery({
    queryKey: ["shows", params],
    queryFn: () =>
      api<{ shows: Show[]; total: number }>("/catalog/shows", { auth: false, query: params }),
  });
}

export function useShow(slug: string) {
  return useQuery({
    queryKey: ["show", slug],
    enabled: !!slug,
    queryFn: () =>
      api<{ show: Show; seasons: Season[]; episodes: Episode[] }>(`/catalog/shows/${slug}`, {
        auth: false,
      }),
  });
}

export function useSearch(q: string) {
  return useQuery({
    queryKey: ["search", q],
    enabled: q.trim().length >= 2,
    queryFn: () =>
      api<{ shows: Show[]; total: number; took_ms: number }>("/catalog/search", {
        auth: false,
        query: { q },
      }),
  });
}

// --- library / me ---

export function useContinueListening() {
  const user = useAuth((s) => s.user);
  return useQuery({
    queryKey: ["continue", user?.id],
    enabled: !!user,
    queryFn: () => api<{ items: ContinueItem[] }>("/playback/continue").then((r) => r.items),
  });
}

export function useLikes() {
  const user = useAuth((s) => s.user);
  return useQuery({
    queryKey: ["likes", user?.id],
    enabled: !!user,
    queryFn: () =>
      api<{ likes: { target_type: string; target_id: string; show_id: string }[] }>("/me/likes").then(
        (r) => r.likes,
      ),
  });
}

export function useBookmarks() {
  const user = useAuth((s) => s.user);
  return useQuery({
    queryKey: ["bookmarks", user?.id],
    enabled: !!user,
    queryFn: () =>
      api<{ bookmarks: { episode_id: string; show_id: string; note: string }[] }>("/me/bookmarks").then(
        (r) => r.bookmarks,
      ),
  });
}

export function useFollows() {
  const user = useAuth((s) => s.user);
  return useQuery({
    queryKey: ["follows", user?.id],
    enabled: !!user,
    queryFn: () => api<{ follows: { show_id: string }[] }>("/me/follows").then((r) => r.follows),
  });
}

export function useHistory() {
  const user = useAuth((s) => s.user);
  return useQuery({
    queryKey: ["history", user?.id],
    enabled: !!user,
    queryFn: () =>
      api<{ history: ContinueItem[] }>("/me/history", { query: { limit: 100 } }).then((r) => r.history),
  });
}

export function usePreferences() {
  const user = useAuth((s) => s.user);
  return useQuery({
    queryKey: ["preferences", user?.id],
    enabled: !!user,
    queryFn: () => api<Record<string, unknown>>("/me/preferences"),
  });
}

export function useEntitlement() {
  const user = useAuth((s) => s.user);
  return useQuery({
    queryKey: ["entitlement", user?.id],
    enabled: !!user,
    queryFn: () => api<Entitlement>("/me/entitlement"),
  });
}

// --- recommendations ---

export function useFeed() {
  const user = useAuth((s) => s.user);
  return useQuery({
    queryKey: ["feed", user?.id],
    enabled: !!user,
    queryFn: () =>
      api<{ items: FeedItem[]; strategy: string }>("/recommendations/feed", { query: { size: 24 } }),
  });
}

export function useTrending() {
  // The recommendation service keeps its own slim show table with no cover art,
  // so fold in cover_image_url and accent_color from the catalog by slug.
  const catalog = useShows({ limit: 100 });
  const q = useQuery({
    queryKey: ["trending"],
    queryFn: () =>
      api<{ items: { show_id: string; title: string; slug: string; plays: number }[] }>(
        "/recommendations/trending",
        { auth: false },
      ).then((r) => r.items),
  });
  const bySlug = new Map((catalog.data?.shows ?? []).map((s) => [s.slug, s]));
  return {
    ...q,
    data: q.data?.map((t) => ({
      ...t,
      cover_image_url: bySlug.get(t.slug)?.cover_image_url ?? null,
      accent_color: bySlug.get(t.slug)?.accent_color ?? null,
    })),
  };
}

export function useSimilar(showId: string) {
  return useQuery({
    queryKey: ["similar", showId],
    enabled: !!showId,
    queryFn: () =>
      api<{ items: { show_id: string; title: string; slug: string }[] }>(
        `/recommendations/similar/${showId}`,
        { auth: false },
      ).then((r) => r.items),
  });
}

// --- mutations ---

export function useToggleLike() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: { type: "show" | "episode"; id: string; showId: string; liked: boolean }) => {
      if (input.liked) {
        await api(`/me/likes/${input.type}/${input.id}`, { method: "DELETE" });
      } else {
        await api("/me/likes", {
          method: "POST",
          body: { type: input.type, id: input.id, show_id: input.showId },
        });
      }
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["likes"] }),
  });
}

export function useToggleBookmark() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: { episodeId: string; showId: string; bookmarked: boolean; note?: string }) => {
      if (input.bookmarked) {
        await api(`/me/bookmarks/${input.episodeId}`, { method: "DELETE" });
      } else {
        await api("/me/bookmarks", {
          method: "POST",
          body: { episode_id: input.episodeId, show_id: input.showId, note: input.note ?? "" },
        });
      }
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["bookmarks"] }),
  });
}

export function useToggleFollow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: { showId: string; following: boolean }) => {
      await api(`/me/follows/${input.showId}`, { method: input.following ? "DELETE" : "POST" });
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["follows"] }),
  });
}
