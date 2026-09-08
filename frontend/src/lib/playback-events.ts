"use client";

import { api } from "@/lib/api";

export type PlaybackEventType =
  | "PLAY"
  | "PAUSE"
  | "SEEK"
  | "PROGRESS"
  | "COMPLETE"
  | "SKIP"
  | "BUFFER_START"
  | "BUFFER_END";

type QueuedEvent = {
  type: PlaybackEventType;
  episode_id: string;
  show_id: string;
  session_id: string;
  position_sec: number;
  from_sec?: number;
  to_sec?: number;
  client_event_id: string;
  occurred_at: string;
};

const IMMEDIATE: PlaybackEventType[] = ["PLAY", "PAUSE", "COMPLETE", "SEEK", "SKIP"];
const FLUSH_INTERVAL_MS = 15_000;

/**
 * Batches playback events and flushes them to the gateway. PROGRESS events are
 * held and sent on an interval; important events flush immediately. Each event
 * carries a client id so a retry is idempotent on the server.
 */
class PlaybackEventBatcher {
  private buffer: QueuedEvent[] = [];
  private timer: ReturnType<typeof setInterval> | null = null;

  start() {
    if (this.timer) return;
    this.timer = setInterval(() => void this.flush(), FLUSH_INTERVAL_MS);
    if (typeof window !== "undefined") {
      window.addEventListener("beforeunload", this.flushSync);
      document.addEventListener("visibilitychange", this.onVisibility);
    }
  }

  stop() {
    if (this.timer) clearInterval(this.timer);
    this.timer = null;
    if (typeof window !== "undefined") {
      window.removeEventListener("beforeunload", this.flushSync);
      document.removeEventListener("visibilitychange", this.onVisibility);
    }
  }

  private onVisibility = () => {
    if (document.visibilityState === "hidden") void this.flush();
  };

  private flushSync = () => {
    if (!this.buffer.length) return;
    const events = this.buffer.splice(0);
    const base = process.env.NEXT_PUBLIC_API_BASE?.replace(/\/$/, "") || "/api";
    try {
      navigator.sendBeacon?.(`${base}/playback/events`, JSON.stringify({ events }));
    } catch {
      /* best effort on unload */
    }
  };

  record(ev: Omit<QueuedEvent, "client_event_id" | "occurred_at">) {
    const full: QueuedEvent = {
      ...ev,
      client_event_id: `${ev.session_id}:${ev.type}:${Math.round(ev.position_sec)}:${Date.now()}`,
      occurred_at: new Date().toISOString(),
    };
    this.buffer.push(full);
    if (IMMEDIATE.includes(ev.type) || this.buffer.length >= 25) {
      void this.flush();
    }
  }

  async flush() {
    if (!this.buffer.length) return;
    const events = this.buffer.splice(0);
    try {
      await api("/playback/events", { method: "POST", body: { events } });
    } catch {
      // Re-queue on failure, capped so a long outage does not grow unbounded.
      this.buffer = [...events.slice(-50), ...this.buffer];
    }
  }
}

export const playbackEvents = new PlaybackEventBatcher();
