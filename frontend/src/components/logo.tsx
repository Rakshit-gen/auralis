type MarkProps = { className?: string; title?: string };

/**
 * The Auralis mark: a whale curved into a dive, back arched over the top, tail
 * fluke lifted. The spout off the blowhole is drawn as three rising bars, so it
 * doubles as an episode playing: the one creature carries the ocean and the
 * audio at once.
 */
export function LogoMark({ className = "h-7 w-7", title }: MarkProps) {
  return (
    <svg viewBox="0 0 40 40" className={className} role={title ? "img" : undefined} aria-hidden={!title}>
      {title ? <title>{title}</title> : null}
      <defs>
        <linearGradient id="auralis-whale" x1="6" y1="30" x2="36" y2="6" gradientUnits="userSpaceOnUse">
          <stop offset="0" stopColor="#2b8b88" />
          <stop offset="0.55" stopColor="#54d0cc" />
          <stop offset="1" stopColor="#9bece8" />
        </linearGradient>
      </defs>

      {/* curved diving whale: arched back from the head up over the hump to a
          raised fluke, one shape */}
      <path
        d="M8.5 28.2C6.6 19 11 9.5 21 9 28 8.7 32.2 12.6 34 17.4 35.2 14.8 37 12.3 39.2 10.2 38.4 14 37.2 16.6 35.4 18.8 36.7 20.7 37.6 22.9 38.4 25.8 35.6 24.2 33 21.9 31.2 20 28.6 24.2 23.4 26.6 17.6 25.9 13.2 25.4 9.4 25.8 8.5 28.2Z"
        fill="url(#auralis-whale)"
      />
      {/* pectoral fin, tucked under */}
      <path
        d="M12.3 25C13.2 28.2 16 29.6 18.7 28.2 16.3 28 13.6 26.8 12.3 25Z"
        fill="#248481"
      />
      {/* jaw line, upturned at the snout */}
      <path
        d="M9 26.4C9.4 24.6 11 23.8 12.8 24.5 16 25.7 19.2 25.5 22 24.1"
        fill="none"
        stroke="#06202a"
        strokeOpacity="0.24"
        strokeWidth="1.1"
        strokeLinecap="round"
      />
      {/* eye */}
      <circle cx="12.6" cy="21.7" r="1.1" fill="#04070c" />
      <circle cx="12.2" cy="21.3" r="0.32" fill="#f4efe6" />

      {/* spout off the blowhole, doubling as playback bars */}
      <g fill="#e2a24d">
        <rect x="15.2" y="4.6" width="1.8" height="4.4" rx="0.9" />
        <rect x="18.1" y="2.1" width="1.8" height="7" rx="0.9" />
        <rect x="21" y="4" width="1.8" height="5.1" rx="0.9" />
      </g>
    </svg>
  );
}

export function Logo({ className = "", markClassName = "h-7 w-7" }: { className?: string; markClassName?: string }) {
  return (
    <span className={`flex items-center gap-2 ${className}`}>
      <LogoMark className={markClassName} title="Auralis" />
      <span className="font-display text-xl tracking-tight text-bone-100">Auralis</span>
    </span>
  );
}
