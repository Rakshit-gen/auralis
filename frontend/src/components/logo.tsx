type MarkProps = { className?: string; title?: string };

/**
 * The Auralis mark: a breaking wave whose crest is also a rising waveform. The
 * curl is the ocean, the four bars climbing out of it are an episode playing.
 * One shape for the two things the product is about.
 */
export function LogoMark({ className = "h-7 w-7", title }: MarkProps) {
  return (
    <svg viewBox="0 0 40 40" className={className} role={title ? "img" : undefined} aria-hidden={!title}>
      {title ? <title>{title}</title> : null}
      <defs>
        <linearGradient id="auralis-water" x1="0" y1="40" x2="34" y2="4" gradientUnits="userSpaceOnUse">
          <stop offset="0" stopColor="#2b8b88" />
          <stop offset="0.6" stopColor="#54d0cc" />
          <stop offset="1" stopColor="#9bece8" />
        </linearGradient>
      </defs>
      {/* the curl of the wave */}
      <path
        d="M4 27c0-9 7.6-16.5 17-16.5 6 0 9.8 3.4 9.8 8 0 4-3 6.8-7 6.8-3.3 0-5.6-2-5.6-4.8 0-2.3 1.6-4 3.8-4 1.7 0 2.9 1 3.1 2.5"
        fill="none"
        stroke="url(#auralis-water)"
        strokeWidth="3.2"
        strokeLinecap="round"
      />
      {/* spray off the crest, doubling as playback bars */}
      <g fill="#e2a24d">
        <rect x="24.5" y="9" width="2.6" height="7" rx="1.3" />
        <rect x="29" y="5.5" width="2.6" height="11" rx="1.3" />
        <rect x="33.5" y="9.5" width="2.6" height="6" rx="1.3" />
      </g>
      {/* the water line the wave sits on */}
      <path
        d="M3 32c3.5 0 3.5 3 7 3s3.5-3 7-3 3.5 3 7 3 3.5-3 7-3 3.5 3 7 3"
        fill="none"
        stroke="#54d0cc"
        strokeOpacity="0.55"
        strokeWidth="2.4"
        strokeLinecap="round"
      />
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
