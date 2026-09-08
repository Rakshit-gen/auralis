"use client";

import { useEffect, useState } from "react";

const SHORTCUTS: [string, string][] = [
  ["Space / K", "Play or pause"],
  ["J / ←", "Back 15 seconds"],
  ["L / →", "Forward 30 seconds"],
  ["N", "Next episode"],
  ["P", "Previous episode"],
  ["↑ / ↓", "Volume up / down"],
  ["?", "Toggle this help"],
];

export function ShortcutsHelp() {
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable) return;
      if (e.key === "?") {
        e.preventDefault();
        setOpen((o) => !o);
      }
      if (e.key === "Escape") setOpen(false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 grid place-items-center bg-ink-950/70 p-4"
      onClick={() => setOpen(false)}
      role="dialog"
      aria-label="Keyboard shortcuts"
    >
      <div className="surface w-full max-w-sm p-6" onClick={(e) => e.stopPropagation()}>
        <h2 className="mb-4 font-display text-xl text-bone-100">Keyboard shortcuts</h2>
        <dl className="space-y-2 text-sm">
          {SHORTCUTS.map(([key, label]) => (
            <div key={key} className="flex items-center justify-between">
              <dd className="text-bone-300">{label}</dd>
              <dt className="rounded border border-ink-600 bg-ink-950 px-2 py-0.5 font-mono text-xs text-bone-200">
                {key}
              </dt>
            </div>
          ))}
        </dl>
        <button className="btn-quiet mt-5 w-full" onClick={() => setOpen(false)}>
          Close
        </button>
      </div>
    </div>
  );
}
