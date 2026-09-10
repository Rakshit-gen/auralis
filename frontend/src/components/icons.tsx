type IconProps = { className?: string };

const base = "h-5 w-5";

export const PlayIcon = ({ className = base }: IconProps) => (
  <svg viewBox="0 0 24 24" fill="currentColor" className={className} aria-hidden>
    <path d="M8 5.14v13.72a1 1 0 0 0 1.54.84l10.28-6.86a1 1 0 0 0 0-1.68L9.54 4.3A1 1 0 0 0 8 5.14Z" />
  </svg>
);

export const PauseIcon = ({ className = base }: IconProps) => (
  <svg viewBox="0 0 24 24" fill="currentColor" className={className} aria-hidden>
    <path d="M7 4h4v16H7zM13 4h4v16h-4z" />
  </svg>
);

export const SkipBackIcon = ({ className = base }: IconProps) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className={className} aria-hidden>
    <path d="M11 6 4 12l7 6M20 6l-7 6 7 6" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

export const SkipFwdIcon = ({ className = base }: IconProps) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className={className} aria-hidden>
    <path d="M13 6l7 6-7 6M4 6l7 6-7 6" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

export const Back15Icon = ({ className = base }: IconProps) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" className={className} aria-hidden>
    <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" />
    <path d="M3 3v5h5" />
    <text x="12.5" y="15.6" textAnchor="middle" fill="currentColor" stroke="none" fontSize="8.5" fontWeight="700" fontFamily="ui-sans-serif, system-ui, sans-serif">15</text>
  </svg>
);

export const Fwd30Icon = ({ className = base }: IconProps) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" className={className} aria-hidden>
    <path d="M21 12a9 9 0 1 1-9-9 9.75 9.75 0 0 1 6.74 2.74L21 8" />
    <path d="M21 3v5h-5" />
    <text x="11.5" y="15.6" textAnchor="middle" fill="currentColor" stroke="none" fontSize="8.5" fontWeight="700" fontFamily="ui-sans-serif, system-ui, sans-serif">30</text>
  </svg>
);

export const HeartIcon = ({ className = base, filled = false }: IconProps & { filled?: boolean }) => (
  <svg
    viewBox="0 0 24 24"
    fill={filled ? "currentColor" : "none"}
    stroke="currentColor"
    strokeWidth="1.8"
    className={className}
    aria-hidden
  >
    <path d="M12 21s-7.5-4.6-10-9.2C.5 8.5 2 5 5.5 5 8 5 9.5 6.7 12 9c2.5-2.3 4-4 6.5-4C22 5 23.5 8.5 22 11.8 19.5 16.4 12 21 12 21Z" />
  </svg>
);

export const BookmarkIcon = ({ className = base, filled = false }: IconProps & { filled?: boolean }) => (
  <svg
    viewBox="0 0 24 24"
    fill={filled ? "currentColor" : "none"}
    stroke="currentColor"
    strokeWidth="1.8"
    className={className}
    aria-hidden
  >
    <path d="M6 3h12v18l-6-4-6 4V3Z" strokeLinejoin="round" />
  </svg>
);

export const PlusIcon = ({ className = base }: IconProps) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className={className} aria-hidden>
    <path d="M12 5v14M5 12h14" strokeLinecap="round" />
  </svg>
);

export const SearchIcon = ({ className = base }: IconProps) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className={className} aria-hidden>
    <circle cx="11" cy="11" r="7" />
    <path d="m20 20-3.5-3.5" strokeLinecap="round" />
  </svg>
);

export const SparkIcon = ({ className = base }: IconProps) => (
  <svg viewBox="0 0 24 24" fill="currentColor" className={className} aria-hidden>
    <path d="M12 2c.5 4.5 3 7 7.5 7.5C15 10 12.5 12.5 12 17c-.5-4.5-3-7-7.5-7.5C9 9 11.5 6.5 12 2Z" />
    <path d="M19 14c.2 2 1.3 3.1 3 3.3-1.7.2-2.8 1.3-3 3.3-.2-2-1.3-3.1-3-3.3 1.7-.2 2.8-1.3 3-3.3Z" />
  </svg>
);

export const WaveIcon = ({ className = base }: IconProps) => (
  <svg viewBox="0 0 24 24" fill="currentColor" className={className} aria-hidden>
    <rect x="3" y="9" width="2.5" height="6" rx="1" />
    <rect x="8" y="5" width="2.5" height="14" rx="1" />
    <rect x="13" y="8" width="2.5" height="8" rx="1" />
    <rect x="18" y="4" width="2.5" height="16" rx="1" />
  </svg>
);

export const ChevronUpIcon = ({ className = base }: IconProps) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className={className} aria-hidden>
    <path d="m6 15 6-6 6 6" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

export const ChevronDownIcon = ({ className = base }: IconProps) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className={className} aria-hidden>
    <path d="m6 9 6 6 6-6" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);
