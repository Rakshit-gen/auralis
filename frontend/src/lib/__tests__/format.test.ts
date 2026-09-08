import { describe, expect, it } from "vitest";
import { formatCount, formatDuration, formatRuntime, relativeTime } from "@/lib/format";

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
});

describe("relativeTime", () => {
  it("handles empty and recent", () => {
    expect(relativeTime(null)).toBe("");
    expect(relativeTime(new Date().toISOString())).toBe("just now");
    expect(relativeTime(new Date(Date.now() - 3 * 60_000).toISOString())).toBe("3m ago");
  });
});
