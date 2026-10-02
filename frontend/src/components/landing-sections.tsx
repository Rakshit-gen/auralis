"use client";

import { useEffect, useState } from "react";
import Link from "next/link";

function useReducedMotion() {
  const [reduced, setReduced] = useState(false);
  useEffect(() => {
    const m = window.matchMedia("(prefers-reduced-motion: reduce)");
    const sync = () => setReduced(m.matches);
    sync();
    m.addEventListener("change", sync);
    return () => m.removeEventListener("change", sync);
  }, []);
  return reduced;
}

// --- Creator pipeline walkthrough --------------------------------------

// One example brief followed through every stage, so each panel shows the
// kind of thing that stage produces instead of placeholder bars.
const EXAMPLE_BRIEF =
  "A lighthouse keeper starts receiving weather reports for a coast that no longer exists.";

const PIPELINE = [
  {
    label: "Story bible",
    sample: "Concept, cast, world rules and a season arc, built from your one-line brief.",
    example:
      "Mara Venn, 61, keeps the light at Skerry Point. The reports name harbours that were drowned in 1953. Rule: every report she receives comes true somewhere, a week later.",
  },
  {
    label: "Episode outlines",
    sample: "One outline per episode, in three acts, each ending on a turn.",
    example:
      "Ep 3, act two: Mara radios a coastguard station that closed decades ago, and someone answers using her late husband's call sign.",
  },
  {
    label: "Full scripts",
    sample: "Every line of narration and dialogue, checked against the bible and earlier episodes.",
    example:
      "MARA: Gale warning, Holm Sound, force nine. There is no Holm Sound. There hasn't been since I was a girl.",
  },
  {
    label: "Voice synthesis",
    sample: "Each character gets a voice, and you can listen to whole episodes before publishing.",
    example: null,
  },
  {
    label: "Your review",
    sample: "Nothing publishes on its own. You read the drafts and decide what goes live, and when.",
    example: "Episode 3: script approved. Audio ready. Not published.",
  },
];

function PipelinePanel({ index }: { index: number }) {
  if (index === 3) {
    return (
      <div className="flex h-full flex-col justify-center">
        <div className="flex items-end gap-1" aria-hidden>
          {Array.from({ length: 24 }).map((_, i) => (
            <span
              key={i}
              className="w-1.5 origin-bottom rounded-sm bg-signal/70 animate-pulse-bar"
              style={{
                height: 12 + ((i * 37) % 40),
                animationDelay: `${(i % 6) * 90}ms`,
                animationDuration: `${900 + (i % 4) * 140}ms`,
              }}
            />
          ))}
        </div>
        <p className="mt-4 text-sm text-bone-300">{PIPELINE[index].sample}</p>
      </div>
    );
  }
  return (
    <div className="flex h-full flex-col justify-center">
      <p className="border-l border-ink-600 pl-3 font-display text-bone-100">{PIPELINE[index].example}</p>
      <p className="mt-4 text-sm text-bone-300">{PIPELINE[index].sample}</p>
    </div>
  );
}

export function CreatorPipeline() {
  const reduced = useReducedMotion();
  const [held, setHeld] = useState(false);
  const [active, setActive] = useState(0);

  useEffect(() => {
    if (held || reduced) return;
    const t = setTimeout(() => setActive((n) => (n + 1) % PIPELINE.length), 3600);
    return () => clearTimeout(t);
  }, [active, held, reduced]);

  return (
    <section
      className="rounded-2xl border border-signal/25 bg-ink-900/80 p-8 sm:p-12"
      onMouseEnter={() => setHeld(true)}
      onMouseLeave={() => setHeld(false)}
      onFocusCapture={() => setHeld(true)}
      onBlurCapture={() => setHeld(false)}
    >
      <div>
        <h2 className="font-display text-3xl text-bone-100 sm:text-4xl">
          What happens to a brief
        </h2>
        <p className="mt-3 max-w-xl text-sm text-bone-300">
          Five stages, shown here for one example: &ldquo;{EXAMPLE_BRIEF}&rdquo;
        </p>

        <div className="mt-8 grid gap-6 lg:grid-cols-[1fr_0.9fr]">
          <ol className="space-y-2">
            {PIPELINE.map((stage, i) => {
              const on = i === active;
              const done = i < active;
              return (
                <li key={stage.label}>
                  <button
                    onClick={() => setActive(i)}
                    className={`flex w-full items-center gap-3 rounded-lg border px-4 py-3 text-left text-sm transition ${
                      on
                        ? "border-signal/50 bg-signal/10 text-bone-100"
                        : "border-ink-700 text-bone-300 hover:border-ink-600"
                    }`}
                  >
                    <span
                      className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full border text-xs ${
                        on || done ? "border-signal/60 text-signal-soft" : "border-ink-600 text-bone-400"
                      }`}
                    >
                      {i + 1}
                    </span>
                    <span className="font-medium">{stage.label}</span>
                  </button>
                  {on && (
                    <div className="mx-1 mt-1 h-0.5 overflow-hidden rounded-full bg-ink-800">
                      <div
                        key={active}
                        className="h-full w-full bg-signal/60"
                        style={{
                          animation: held || reduced ? "none" : "landing-fill 3600ms linear forwards",
                        }}
                      />
                    </div>
                  )}
                </li>
              );
            })}
          </ol>

          <div className="surface min-h-[220px] p-5">
            <p className="text-sm text-bone-400">{PIPELINE[active].label}</p>
            <div className="mt-4 h-[calc(100%-2rem)]">
              <PipelinePanel index={active} />
            </div>
          </div>
        </div>

        <div className="mt-8">
          <Link href="/discover?ai=true" className="btn-ghost text-sm">
            Explore published series
          </Link>
        </div>
      </div>
    </section>
  );
}
