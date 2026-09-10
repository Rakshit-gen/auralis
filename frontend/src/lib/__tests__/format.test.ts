import { describe, expect, it } from "vitest";
import { coverStyle, formatCount, formatDuration, formatRuntime, relativeTime, sampleBars } from "@/lib/format";

describe("formatDuration", () => {
  it("shows m:ss under an hour", () => {
    expect(formatDuration(0)).toBe("0:00");
    expect(formatDuration(65)).toBe("1:05");
    expect(formatDuration(600)).toBe("10:00");
  });
  it("shows h:mm:ss over an hour", () => {
    expect(formatDuration(3661)).toBe("1:01:01");
  });
  it("guards bad input", () => {
    expect(formatDuration(-5)).toBe("0:00");
    expect(formatDuration(Number.NaN)).toBe("0:00");
  });
});

describe("formatRuntime", () => {
  it("uses minutes then hours", () => {
    expect(formatRuntime(120)).toBe("2 min");
    expect(formatRuntime(3600)).toBe("1 hr");
    expect(formatRuntime(5400)).toBe("1 hr 30 min");
  });
});

describe("formatCount", () => {
  it("abbreviates thousands and millions", () => {
    expect(formatCount(999)).toBe("999");
    expect(formatCount(1500)).toBe("1.5k");
    expect(formatCount(23000)).toBe("23k");
    expect(formatCount(2_400_000)).toBe("2.4M");
  });
  it("rolls up to millions instead of a four-digit k", () => {
    expect(formatCount(999_999)).toBe("1.0M");
  });
});

describe("coverStyle", () => {
  it("uses the gradient alone when there is no image", () => {
    const s = coverStyle("#c98a3c", "the-long-room");
    expect(s.backgroundImage).toMatch(/^linear-gradient\(/);
    expect(s.backgroundSize).toBeUndefined();
  });
  it("layers a real image over the gradient", () => {
    const s = coverStyle("#c98a3c", "the-long-room", "https://cdn.example/covers/shows/x.webp");
    expect(s.backgroundImage).toBe(
      'url("https://cdn.example/covers/shows/x.webp"), linear-gradient(150deg, #c98a3c 0%, hsl(208 30% 12%) 55%, #0a0908 100%)',
    );
    expect(s.backgroundSize).toBe("cover");
  });
  it("treats empty string as no image", () => {
    expect(coverStyle("#c98a3c", "x", "").backgroundSize).toBeUndefined();
  });
});

describe("sampleBars", () => {
  it("is deterministic for a seed and length", () => {
    expect(sampleBars("mystery", 12)).toEqual(sampleBars("mystery", 12));
  });
  it("differs by seed and honours the count", () => {
    expect(sampleBars("mystery", 8)).not.toEqual(sampleBars("noir", 8));
    expect(sampleBars("noir", 20)).toHaveLength(20);
  });
  it("keeps every bar within 0.15–1", () => {
    for (const h of sampleBars("science-fiction", 64)) {
      expect(h).toBeGreaterThanOrEqual(0.15);
      expect(h).toBeLessThanOrEqual(1);
    }
  });
});

describe("relativeTime", () => {
  it("handles empty and recent", () => {
    expect(relativeTime(null)).toBe("");
    expect(relativeTime(new Date().toISOString())).toBe("just now");
    expect(relativeTime(new Date(Date.now() - 3 * 60_000).toISOString())).toBe("3m ago");
  });
  it("returns empty for an unparseable date", () => {
    expect(relativeTime("not-a-date")).toBe("");
  });
  it("treats a future timestamp as just now", () => {
    expect(relativeTime(new Date(Date.now() + 60 * 60_000).toISOString())).toBe("just now");
  });
});
