import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        ink: {
          950: "#0a0908",
          900: "#12100e",
          800: "#1b1815",
          700: "#26221d",
          600: "#37312a",
          500: "#4c443a",
        },
        bone: {
          100: "#f4efe6",
          200: "#e4dccb",
          300: "#c9bda4",
          400: "#a89a7d",
        },
        amber: {
          DEFAULT: "#d9963f",
          soft: "#e8b872",
          deep: "#a86a24",
        },
        signal: "#6fb3c9",
      },
      fontFamily: {
        display: ["var(--font-display)", "Georgia", "serif"],
        sans: ["var(--font-sans)", "system-ui", "sans-serif"],
        mono: ["ui-monospace", "SFMono-Regular", "monospace"],
      },
      boxShadow: {
        lift: "0 20px 60px -20px rgba(0,0,0,0.7)",
        glow: "0 0 40px -8px rgba(217,150,63,0.35)",
      },
      backgroundImage: {
        grain: "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='120' height='120'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='3' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='120' height='120' filter='url(%23n)' opacity='0.35'/%3E%3C/svg%3E\")",
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
      },
      animation: {
        "fade-up": "fade-up 0.5s ease-out both",
        "pulse-bar": "pulse-bar 1s ease-in-out infinite",
      },
    },
  },
  plugins: [],
};

export default config;
