import type { Metadata, Viewport } from "next";
import "./globals.css";
import { Providers } from "@/lib/providers";
import { Nav } from "@/components/layout/nav";
import { AuthGate } from "@/components/auth-gate";
import { OceanWaves } from "@/components/ocean-waves";
import { AudioEngine } from "@/components/player/audio-engine";
import { PlayerBar } from "@/components/player/player-bar";
import { ShortcutsHelp } from "@/components/player/shortcuts";

export const metadata: Metadata = {
  title: { default: "Auralis", template: "%s · Auralis" },
  description: "Serialized audio stories you can follow, stream, and pick up anywhere.",
};

export const viewport: Viewport = {
  themeColor: "#04070c",
  width: "device-width",
  initialScale: 1,
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className="min-h-screen">
        <Providers>
          <OceanWaves />
          <a
            href="#main"
            className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-50 focus:rounded focus:bg-amber focus:px-3 focus:py-2 focus:text-ink-950"
          >
            Skip to content
          </a>
          <Nav />
          <main id="main" className="pb-28 pt-6">
            {children}
          </main>
          <AudioEngine />
          <PlayerBar />
          <ShortcutsHelp />
          <AuthGate />
        </Providers>
      </body>
    </html>
  );
}
