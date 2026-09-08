"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { SectionHeader } from "@/components/ui";
import { SparkIcon } from "@/components/icons";

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

// --- How it works: a plain click-to-open accordion ------------------------

const LISTEN_STEPS = [
  {
    title: "Follow a show",
    body: "Browse by genre, language, or mood. Follow anything and its new episodes turn up in your feed on their own.",
  },
  {
    title: "Listen anywhere",
    body: "The web player and your phone share one account. Audio streams as adaptive HLS at 64, 128, or 256 kbps, so it holds together on a train.",
  },
  {
    title: "Pick up where you stopped",
    body: "Your position is saved to the second and follows you between devices. Come back a month later and press play without hunting for the spot.",
  },
];

export function HowItWorks() {
  const [open, setOpen] = useState(0);

  return (
    <section>
      <SectionHeader eyebrow="How listening works" title="Start an episode. Leave. Come back." />
      <ol className="grid gap-2 sm:grid-cols-3">
        {LISTEN_STEPS.map((step, i) => {
          const on = i === open;
          return (
            <li key={step.title}>
              <button
                onClick={() => setOpen(i)}
                aria-expanded={on}
                className={`flex h-full w-full flex-col rounded-xl border p-4 text-left transition ${
                  on ? "border-signal/50 bg-signal/[0.06]" : "border-ink-700 hover:border-ink-600"
                }`}
              >
                <div className="flex items-center gap-3">
                  <span
                    className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-full border text-sm font-semibold ${
                      on ? "border-signal/60 text-signal-soft" : "border-ink-600 text-bone-400"
                    }`}
                  >
                    {i + 1}
                  </span>
                  <span className="font-display text-lg text-bone-100">{step.title}</span>
                </div>
                <div
                  className="grid transition-[grid-template-rows] duration-300"
                  style={{ gridTemplateRows: on ? "1fr" : "0fr" }}
                >
                  <p className="overflow-hidden text-sm text-bone-300">
                    <span className="block pt-3">{step.body}</span>
                  </p>
                </div>
              </button>
            </li>
          );
        })}
      </ol>
    </section>
  );
}

// --- Browse by genre -----------------------------------------------------

// Reference data, seeded in services/content/migrations/0002_reference_data.sql.
// Held here so the landing page renders the strip with no extra request.
const GENRES: { slug: string; name: string; description: string }[] = [
  { slug: "mystery", name: "Mystery", description: "Investigations, missing pieces, and answers that cost something." },
  { slug: "science-fiction", name: "Science Fiction", description: "Near and far futures, and the rules that hold them together." },
  { slug: "thriller", name: "Thriller", description: "Pressure that does not let up until the last minute." },
  { slug: "drama", name: "Drama", description: "People, decisions, and the weight they carry afterward." },
  { slug: "folk-horror", name: "Folk Horror", description: "Places that remember, and traditions that were never really abandoned." },
  { slug: "adventure", name: "Adventure", description: "Movement, stakes, and somewhere that has to be reached." },
  { slug: "noir", name: "Noir", description: "Bad options, worse people, and a narrator who has stopped pretending." },
  { slug: "romance", name: "Romance", description: "Two people, the space between them, and whether it closes." },
  { slug: "history", name: "History", description: "Real periods, fictional lives, carefully kept apart from the record." },
  { slug: "comedy", name: "Comedy", description: "Timing, and the relief of being allowed to laugh." },
  { slug: "fantasy", name: "Fantasy", description: "Systems of magic with a price, and worlds that run on them." },
  { slug: "slice-of-life", name: "Slice of Life", description: "Small scale, close attention, nothing exploding." },
];

export function GenreStrip() {
  return (
    <section>
      <SectionHeader
        eyebrow="The catalog"
        title="Find your genre"
        href="/discover"
        linkLabel="Open discover"
      />
      <div className="-mx-4 flex snap-x gap-3 overflow-x-auto px-4 pb-2 sm:mx-0 sm:px-0">
        {GENRES.map((g) => (
          <Link
            key={g.slug}
            href={`/discover?genre=${g.slug}`}
            className="group w-52 shrink-0 snap-start rounded-xl border border-ink-700 bg-ink-950/40 p-4 transition hover:-translate-y-0.5 hover:border-signal/40"
          >
            <p className="font-display text-lg text-bone-100">{g.name}</p>
            <p className="mt-1 line-clamp-2 text-xs text-bone-400">{g.description}</p>
          </Link>
        ))}
      </div>
    </section>
  );
}

// --- Creator pipeline walkthrough --------------------------------------

const PIPELINE = [
  {
    label: "Story bible",
    sample: "Concept, cast, world rules, and a season-long arc, all built out from your one-line brief.",
  },
  {
    label: "Episode outlines",
    sample: "One outline per episode, shaped into three acts across the season with a cliffhanger each.",
  },
  {
    label: "Full scripts",
    sample: "Every line of narration and dialogue, checked against the bible and the episodes before it.",
  },
  {
    label: "Voice synthesis",
    sample: "A distinct neural voice per character, rendered offline, then packaged as streaming audio.",
  },
  {
    label: "Your review",
    sample: "Nothing publishes on its own. You read the drafts and decide what goes live, and when.",
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
      <div className="space-y-1.5" aria-hidden>
        {[92, 78, 96, 61, 84].slice(0, index === 4 ? 3 : 5).map((w, i) => (
          <div key={i} className="h-2 rounded-full bg-ink-700" style={{ width: `${w}%` }} />
        ))}
      </div>
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
      className="relative overflow-hidden rounded-2xl border border-signal/25 bg-ink-900/70 p-8 backdrop-blur sm:p-12"
      onMouseEnter={() => setHeld(true)}
      onMouseLeave={() => setHeld(false)}
      onFocusCapture={() => setHeld(true)}
      onBlurCapture={() => setHeld(false)}
    >
      <div className="pointer-events-none absolute -left-20 -top-20 h-64 w-64 rounded-full bg-signal/10 blur-3xl" />
      <div className="relative">
        <p className="eyebrow mb-3 flex items-center gap-2">
          <SparkIcon className="h-4 w-4" /> Inside the studio
        </p>
        <h2 className="font-display text-3xl text-bone-100 sm:text-4xl">
          One brief moves through five stages
        </h2>
        <p className="mt-3 max-w-xl text-sm text-bone-300">
          The same pipeline that runs on the generate page. Tap a stage to hold it.
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
            <p className="text-xs uppercase tracking-[0.2em] text-bone-400">{PIPELINE[active].label}</p>
            <div className="mt-4 h-[calc(100%-2rem)]">
              <PipelinePanel index={active} />
            </div>
          </div>
        </div>

        <div className="mt-8">
          <Link href="/discover?ai=true" className="btn-ghost text-sm">
            Hear finished AI series
          </Link>
        </div>
      </div>
    </section>
  );
}
