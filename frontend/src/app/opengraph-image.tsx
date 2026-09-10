import { ImageResponse } from "next/og";
import { sampleBars } from "@/lib/format";

// The card that unfurls when an Auralis link is shared on X, Slack, iMessage,
// etc. Next attaches it to both Open Graph and Twitter automatically.
export const alt = "Auralis — serialized audio fiction that remembers where you stopped";
export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

export default function OpengraphImage() {
  const bars = sampleBars("auralis-open-graph", 48);

  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          flexDirection: "column",
          justifyContent: "space-between",
          padding: 72,
          background:
            "radial-gradient(900px 500px at 12% 100%, rgba(84,208,204,0.18), transparent 60%), linear-gradient(160deg, #04070c, #060d16 55%, #04070c)",
          color: "#e4dccb",
          fontFamily: "Georgia, serif",
        }}
      >
        <div
          style={{
            fontSize: 24,
            letterSpacing: 8,
            textTransform: "uppercase",
            color: "#54d0cc",
            fontFamily: "system-ui, sans-serif",
          }}
        >
          Auralis
        </div>

        <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
          <div style={{ fontSize: 62, lineHeight: 1.08, maxWidth: 940 }}>
            Serialized audio fiction that remembers exactly where you stopped.
          </div>
          <div
            style={{
              fontSize: 26,
              color: "#9aa7ac",
              fontFamily: "system-ui, sans-serif",
              maxWidth: 820,
            }}
          >
            Follow a show, listen on any device, pick up to the second.
          </div>
        </div>

        <div style={{ display: "flex", alignItems: "flex-end", gap: 6, height: 90 }}>
          {bars.map((b, i) => (
            <div
              key={i}
              style={{
                width: 16,
                height: Math.round(b * 90),
                borderRadius: 3,
                background: i < bars.length * 0.42 ? "#54d0cc" : "rgba(84,208,204,0.28)",
              }}
            />
          ))}
        </div>
      </div>
    ),
    size,
  );
}
