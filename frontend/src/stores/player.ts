"use client";

import { create } from "zustand";
import { api } from "@/lib/api";
import { playbackEvents } from "@/lib/playback-events";
import type { AuthorizeResponse } from "@/lib/types";

export type QueueEntry = {
  episodeId: string;
  showId: string;
  showSlug: string;
  showTitle: string;
  episodeTitle: string;
  episodeNumber: number;
};

type PlayerState = {
  queue: QueueEntry[];
  index: number;
  current: QueueEntry | null;
  authorization: AuthorizeResponse | null;

  playing: boolean;
  currentTime: number;
  duration: number;
  buffering: boolean;
  volume: number;
  rate: number;
  error: string | null;
  expanded: boolean;

  // set by the AudioEngine component
  seekRequest: number | null;

  authorize: (episodeId: string, meta?: Partial<QueueEntry>) => Promise<void>;
  playNow: (entry: QueueEntry) => Promise<void>;
  setQueue: (entries: QueueEntry[], startIndex?: number) => Promise<void>;
  enqueue: (entry: QueueEntry) => void;
  next: () => Promise<void>;
  previous: () => Promise<void>;

  togglePlay: () => void;
  setPlaying: (v: boolean) => void;
  requestSeek: (sec: number) => void;
  clearSeek: () => void;
  skip: (deltaSec: number) => void;
  setRate: (r: number) => void;
  setVolume: (v: number) => void;
  setExpanded: (v: boolean) => void;

  // called by AudioEngine
  onTimeUpdate: (t: number, d: number) => void;
  onBuffering: (v: boolean) => void;
  onEnded: () => void;
};

const PROGRESS_EVERY_SEC = 15;

export const usePlayer = create<PlayerState>((set, get) => {
  let lastProgressAt = 0;

  const emit = (
    type: Parameters<typeof playbackEvents.record>[0]["type"],
    extra: { from_sec?: number; to_sec?: number } = {},
  ) => {
    const { current, authorization, currentTime } = get();
    if (!current || !authorization) return;
    playbackEvents.record({
      type,
      episode_id: current.episodeId,
      show_id: current.showId,
      session_id: authorization.session_id,
      position_sec: Math.round(currentTime),
      ...extra,
    });
  };

  const persistProgress = async (completed = false) => {
    const { current, authorization, currentTime, duration } = get();
    if (!current || !authorization) return;
    try {
      await api("/playback/progress", {
        method: "POST",
        body: {
          episode_id: current.episodeId,
          show_id: current.showId,
          session_id: authorization.session_id,
          position_sec: Math.round(currentTime),
          duration_sec: Math.round(duration),
          completed,
          client_event_id: `${authorization.session_id}:progress:${Math.round(currentTime)}`,
          occurred_at: new Date().toISOString(),
        },
      });
    } catch {
      /* progress is retried on the next tick */
    }
  };

  return {
    queue: [],
    index: -1,
    current: null,
    authorization: null,
    playing: false,
    currentTime: 0,
    duration: 0,
    buffering: false,
    volume: 1,
    rate: 1,
    error: null,
    expanded: false,
    seekRequest: null,

    authorize: async (episodeId, meta) => {
      set({ error: null, buffering: true });
      try {
        const auth = await api<AuthorizeResponse>("/playback/authorize", {
          method: "POST",
          body: { episode_id: episodeId },
        });
        if (!auth?.episode || !auth.hls_master_url) {
          throw new Error("playback authorization returned no media");
        }
        const entry: QueueEntry = {
          episodeId,
          showId: auth.episode.show_id,
          showSlug: auth.episode.show_slug,
          showTitle: auth.episode.show_title,
          episodeTitle: auth.episode.episode_title,
          episodeNumber: auth.episode.episode_number,
          ...meta,
        };
        set({
          authorization: auth,
          current: entry,
          currentTime: auth.resume_position_sec || 0,
          duration: auth.episode.duration_sec,
          playing: true,
          buffering: false,
        });
        playbackEvents.start();
        emit("PLAY");
      } catch (e) {
        set({ error: (e as Error).message, buffering: false, playing: false });
        throw e;
      }
    },

    playNow: async (entry) => {
      set({ queue: [entry], index: 0 });
      await get().authorize(entry.episodeId, entry);
    },

    setQueue: async (entries, startIndex = 0) => {
      set({ queue: entries, index: startIndex });
      const start = entries[startIndex];
      if (start) await get().authorize(start.episodeId, start);
    },

    enqueue: (entry) => set((s) => ({ queue: [...s.queue, entry] })),

    next: async () => {
      const { queue, index } = get();
      if (index + 1 < queue.length) {
        set({ index: index + 1 });
        await get()
          .authorize(queue[index + 1].episodeId, queue[index + 1])
          .catch(() => set({ playing: false }));
      } else {
        set({ playing: false });
      }
    },

    previous: async () => {
      const { queue, index, currentTime } = get();
      if (currentTime > 3) {
        get().requestSeek(0);
        return;
      }
      if (index - 1 >= 0) {
        set({ index: index - 1 });
        await get().authorize(queue[index - 1].episodeId, queue[index - 1]);
      }
    },

    togglePlay: () => {
      const playing = !get().playing;
      set({ playing });
      emit(playing ? "PLAY" : "PAUSE");
      void persistProgress();
    },

    setPlaying: (v) => set({ playing: v }),

    requestSeek: (sec) => {
      const from = get().currentTime;
      set({ seekRequest: Math.max(0, sec) });
      emit("SEEK", { from_sec: Math.round(from), to_sec: Math.round(sec) });
    },
    clearSeek: () => set({ seekRequest: null }),

    skip: (delta) => {
      const { currentTime, duration } = get();
      const target = Math.min(Math.max(0, currentTime + delta), duration || currentTime + delta);
      set({ seekRequest: target });
      emit("SKIP", { from_sec: Math.round(currentTime), to_sec: Math.round(target) });
    },

    setRate: (r) => set({ rate: r }),
    setVolume: (v) => set({ volume: Math.min(1, Math.max(0, v)) }),
    setExpanded: (v) => set({ expanded: v }),

    onTimeUpdate: (t, d) => {
      set({ currentTime: t, duration: d || get().duration });
      const now = Date.now() / 1000;
      if (now - lastProgressAt >= PROGRESS_EVERY_SEC) {
        lastProgressAt = now;
        emit("PROGRESS");
        void persistProgress();
      }
    },

    onBuffering: (v) => {
      set({ buffering: v });
      emit(v ? "BUFFER_START" : "BUFFER_END");
    },

    onEnded: () => {
      set({ playing: false });
      emit("COMPLETE");
      void persistProgress(true);
      void get().next();
    },
  };
});
