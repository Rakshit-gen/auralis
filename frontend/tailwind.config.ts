import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        // Deep-water darks. Still near-black, now with a cold blue cast so the
        // whole app reads like light coming up through the ocean at night.
        ink: {
          950: "#04070c",
          900: "#080f18",
          800: "#101d2b",
          700: "#1b2f42",
          600: "#294657",
          500: "#3c6072",
        },
        // Warm paper tones for text. Kept from the original reading-room theme so
        // long passages of copy still feel like something you sit down to read.
        bone: {
          100: "#f4efe6",
          200: "#e4dccb",
          300: "#c9bda4",
          400: "#a89a7d",
        },
        // The lantern. A single warm accent against the cold water, used for the
        // primary action and anything that should feel lit from within.
        amber: {
          DEFAULT: "#e2a24d",
          soft: "#f0c37e",
          deep: "#b0742a",
        },
        // Bioluminescence. The cool counter-accent: AI features, live state,
        // things that glow on their own.
        signal: {
          DEFAULT: "#54d0cc",
          soft: "#9bece8",
          deep: "#2b8b88",
        },
      },
      fontFamily: {
        display: ["var(--font-display)", "Georgia", "serif"],
        sans: ["var(--font-sans)", "system-ui", "sans-serif"],
        mono: ["ui-monospace", "SFMono-Regular", "monospace"],
      },
      boxShadow: {
        lift: "0 20px 60px -20px rgba(0,0,0,0.7)",
        glow: "0 0 40px -8px rgba(226,162,77,0.35)",
        tide: "0 0 44px -10px rgba(84,208,204,0.4)",
      },
      backgroundImage: {
        grain:
          "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='120' height='120'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='3' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='120' height='120' filter='url(%23n)' opacity='0.35'/%3E%3C/svg%3E\")",
      },
      keyframes: {
        "fade-up": {
          "0%": { opacity: "0", transform: "translateY(12px)" },
          "100%": { opacity: "1", transform: "translateY(0)" },
        },
        "pulse-bar": {
          "0%, 100%": { transform: "scaleY(0.4)" },
          "50%": { transform: "scaleY(1)" },
        },
        drift: {
          "0%, 100%": { transform: "translateY(0)" },
          "50%": { transform: "translateY(-6px)" },
        },
        surface: {
          "0%, 100%": { transform: "translateY(0) rotate(-0.4deg)" },
          "50%": { transform: "translateY(-10px) rotate(0.4deg)" },
        },
        shimmer: {
          "0%": { backgroundPosition: "-200% 0" },
          "100%": { backgroundPosition: "200% 0" },
        },
        "tide-in": {
          "0%": { opacity: "0", transform: "translateY(24px) scale(0.98)" },
          "100%": { opacity: "1", transform: "translateY(0) scale(1)" },
        },
      },
      animation: {
        "fade-up": "fade-up 0.5s ease-out both",
        "pulse-bar": "pulse-bar 1s ease-in-out infinite",
        drift: "drift 6s ease-in-out infinite",
        surface: "surface 9s ease-in-out infinite",
        shimmer: "shimmer 2.4s linear infinite",
        "tide-in": "tide-in 0.6s cubic-bezier(0.22, 1, 0.36, 1) both",
      },
    },
  },
  plugins: [],
};

export default config;
