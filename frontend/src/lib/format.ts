export function formatDuration(totalSeconds: number): string {
  if (!Number.isFinite(totalSeconds) || totalSeconds < 0) return "0:00";
  const s = Math.floor(totalSeconds % 60);
  const m = Math.floor((totalSeconds / 60) % 60);
  const h = Math.floor(totalSeconds / 3600);
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
  return `${m}:${String(s).padStart(2, "0")}`;
}

export function formatRuntime(totalSeconds: number): string {
  const mins = Math.round(totalSeconds / 60);
  if (mins < 60) return `${mins} min`;
  const h = Math.floor(mins / 60);
  const m = mins % 60;
  return m ? `${h} hr ${m} min` : `${h} hr`;
}

export function formatCount(n: number): string {
  if (n < 1000) return String(n);
  if (n < 1_000_000) return `${(n / 1000).toFixed(n < 10_000 ? 1 : 0)}k`;
  return `${(n / 1_000_000).toFixed(1)}M`;
}

export function relativeTime(iso: string | null | undefined): string {
  if (!iso) return "";
  const then = new Date(iso).getTime();
  const diff = Date.now() - then;
  const mins = Math.round(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.round(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.round(hrs / 24);
  if (days < 30) return `${days}d ago`;
  return new Date(iso).toLocaleDateString();
}

// Cover art for a show. When it has a real generated image we use that; without
// one we fall back to a deterministic gradient from the accent color and seed so
// the catalog still has art. The gradient stays underneath as a load fallback.
export function coverStyle(accent: string, seed: string, image?: string | null): React.CSSProperties {
  const hue = [...seed].reduce((a, c) => a + c.charCodeAt(0), 0) % 360;
  const gradient = `linear-gradient(150deg, ${accent} 0%, hsl(${hue} 30% 12%) 55%, #0a0908 100%)`;
  if (image) {
    return {
      backgroundImage: `url(${JSON.stringify(image)}), ${gradient}`,
      backgroundSize: "cover",
      backgroundPosition: "center",
    };
  }
  return { backgroundImage: gradient };
}
