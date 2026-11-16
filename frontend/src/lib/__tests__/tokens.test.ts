import { afterEach, describe, expect, it, vi } from "vitest";

import { loadTokens, refreshTokens, saveTokens } from "@/lib/api";
import { playbackEvents } from "@/lib/playback-events";

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
});

describe("token refresh", () => {
  it("uses tokens another tab already refreshed instead of spending the old refresh token", async () => {
    saveTokens({ access_token: "a1", refresh_token: "r1" });
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    // Another tab holds the lock and refreshes before this tab gets it.
    vi.stubGlobal("navigator", {
      locks: {
        request: async (_name: string, fn: () => Promise<unknown>) => {
          saveTokens({ access_token: "a2", refresh_token: "r2" });
          return fn();
        },
      },
    });
    const got = await refreshTokens();
    expect(got?.refresh_token).toBe("r2");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("refreshes when nobody else has", async () => {
    saveTokens({ access_token: "a1", refresh_token: "r1" });
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              tokens: { access_token: "a2", refresh_token: "r2" },
            }),
          ),
      ),
    );
    expect((await refreshTokens())?.access_token).toBe("a2");
    expect(loadTokens()?.refresh_token).toBe("r2");
  });
});

describe("playback events on unload", () => {
  it("sends the access token with a keepalive request", () => {
    saveTokens({ access_token: "a1", refresh_token: "r1" });
    const fetchMock = vi.fn(async () => new Response(null, { status: 202 }));
    vi.stubGlobal("fetch", fetchMock);
    playbackEvents.record({
      type: "PROGRESS",
      episode_id: "e",
      show_id: "s",
      session_id: "x",
      position_sec: 5,
    });
    (playbackEvents as unknown as { flushSync: () => void }).flushSync();
    const [, init] = fetchMock.mock.calls[0] as unknown as [
      string,
      RequestInit,
    ];
    expect(init.keepalive).toBe(true);
    expect((init.headers as Record<string, string>).Authorization).toBe(
      "Bearer a1",
    );
  });
});
