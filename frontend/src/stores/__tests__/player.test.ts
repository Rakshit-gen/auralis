import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/api", () => ({ api: vi.fn().mockResolvedValue({}) }));
vi.mock("@/lib/playback-events", () => ({
  playbackEvents: { start: vi.fn(), stop: vi.fn(), record: vi.fn(), flush: vi.fn() },
}));

import { usePlayer } from "@/stores/player";
import { playbackEvents } from "@/lib/playback-events";

const entry = {
  episodeId: "e1",
  showId: "s1",
  showSlug: "s",
  showTitle: "Show",
  episodeTitle: "Ep",
  episodeNumber: 1,
};

describe("player store", () => {
  beforeEach(() => {
    usePlayer.setState({
      queue: [entry, { ...entry, episodeId: "e2", episodeNumber: 2 }],
      index: 0,
      current: entry,
      authorization: { session_id: "sess" } as never,
      playing: true,
      currentTime: 30,
      duration: 100,
    });
    vi.clearAllMocks();
  });

  it("toggling play emits a PAUSE event with the current position", () => {
    usePlayer.getState().togglePlay();
    expect(usePlayer.getState().playing).toBe(false);
    expect(playbackEvents.record).toHaveBeenCalledWith(
      expect.objectContaining({ type: "PAUSE", episode_id: "e1", position_sec: 30 }),
    );
  });

  it("requesting a seek records from/to and sets a seek request", () => {
    usePlayer.getState().requestSeek(75);
    expect(usePlayer.getState().seekRequest).toBe(75);
    expect(playbackEvents.record).toHaveBeenCalledWith(
      expect.objectContaining({ type: "SEEK", from_sec: 30, to_sec: 75 }),
    );
  });

  it("skip clamps to the episode bounds", () => {
    usePlayer.getState().skip(-100);
    expect(usePlayer.getState().seekRequest).toBe(0);
    usePlayer.setState({ seekRequest: null, currentTime: 90 });
    usePlayer.getState().skip(30);
    expect(usePlayer.getState().seekRequest).toBe(100);
  });

  it("onEnded emits COMPLETE and advances the queue", () => {
    usePlayer.getState().onEnded();
    expect(playbackEvents.record).toHaveBeenCalledWith(expect.objectContaining({ type: "COMPLETE" }));
    expect(usePlayer.getState().playing).toBe(false);
  });
});
