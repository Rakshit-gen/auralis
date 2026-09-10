"use client";

import Link from "next/link";
import type { ButtonHTMLAttributes, ReactNode } from "react";

export function Spinner({ label = "Loading" }: { label?: string }) {
  return (
    <div className="flex items-center gap-3 text-sm text-bone-300" role="status">
      <span className="flex items-end gap-0.5" aria-hidden>
        {[0, 1, 2, 3].map((i) => (
          <span
            key={i}
            className="w-1 origin-bottom rounded bg-signal animate-pulse-bar"
            style={{ height: 16, animationDelay: `${i * 120}ms` }}
          />
        ))}
      </span>
      {label}
    </div>
  );
}

export function EmptyState({
  title,
  hint,
  action,
}: {
  title: string;
  hint?: string;
  action?: ReactNode;
}) {
  return (
    <div className="surface flex flex-col items-center gap-3 px-6 py-14 text-center">
      <p className="font-display text-lg text-bone-100">{title}</p>
      {hint && <p className="max-w-sm text-sm text-bone-300">{hint}</p>}
      {action}
    </div>
  );
}

export function ErrorState({ message, retry }: { message: string; retry?: () => void }) {
  return (
    <div className="surface flex flex-col items-center gap-3 border-amber/30 px-6 py-10 text-center">
      <p className="text-sm text-bone-200">{message}</p>
      {retry && (
        <button className="btn-ghost" onClick={retry}>
          Try again
        </button>
      )}
    </div>
  );
}

export function SectionHeader({
  eyebrow,
  title,
  href,
  linkLabel = "See all",
}: {
  eyebrow?: string;
  title: string;
  href?: string;
  linkLabel?: string;
}) {
  return (
    <div className="mb-4 flex items-end justify-between gap-4">
      <div>
        {eyebrow && <p className="eyebrow mb-1">{eyebrow}</p>}
        <h2 className="font-display text-2xl text-bone-100">{title}</h2>
      </div>
      {href && (
        <Link href={href} className="btn-quiet shrink-0 text-sm text-signal-soft">
          {linkLabel}
        </Link>
      )}
    </div>
  );
}

export function Skeleton({ className = "" }: { className?: string }) {
  return <div className={`animate-pulse rounded-lg bg-ink-800 ${className}`} />;
}

/**
 * A button with a band of light tracing its label and border. Adapted from
 * VengeanceUI's animated-button to the palette; the motion lives in
 * `.btn-shine` (globals.css) and drops out under prefers-reduced-motion.
 */
export function AnimatedButton({
  children,
  className = "",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button {...props} className={`btn-shine ${className}`}>
      <span>{children}</span>
    </button>
  );
}

export function Chip({
  active,
  children,
  onClick,
}: {
  active?: boolean;
  children: ReactNode;
  onClick?: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={`rounded-full border px-3 py-1 text-sm transition ${
        active
          ? "border-amber bg-amber/15 text-amber-soft"
          : "border-ink-600 text-bone-300 hover:border-ink-500 hover:text-bone-100"
      }`}
    >
      {children}
    </button>
  );
}

export function ProgressBar({ value }: { value: number }) {
  return (
    <div className="h-1.5 w-full overflow-hidden rounded-full bg-ink-700">
      <div
        className="h-full rounded-full bg-gradient-to-r from-signal-deep via-signal to-signal-soft transition-[width] duration-500"
        style={{ width: `${Math.min(100, Math.max(0, value * 100))}%` }}
      />
    </div>
  );
}
