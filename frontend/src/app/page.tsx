"use client";

import Link from "next/link";
import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/stores/auth";
import { useTrending } from "@/lib/hooks";
import { ShowCardCompact } from "@/components/show-card";
import { LogoMark } from "@/components/logo";
import { SparkIcon, WaveIcon } from "@/components/icons";

const BRIEF_SAMPLES = [
  "A lighthouse keeper starts receiving weather reports for a coast that no longer exists.",
  "Two rival tide-pool researchers share one field station and one radio.",
  "A salvage diver keeps finding the same shipwreck in different oceans.",
];

export default function Landing() {
  const { user, ready } = useAuth();
  const router = useRouter();
  const { data: trending } = useTrending();

  useEffect(() => {
    if (ready && user) router.replace("/home");
  }, [ready, user, router]);

  return (
    <div className="container-page">
      <section className="grid gap-10 py-16 lg:grid-cols-[1.1fr_0.9fr] lg:py-24">
        <div className="animate-fade-up">
          <p className="eyebrow mb-4">Serialized audio, one episode at a time</p>
          <h1 className="font-display text-4xl leading-[1.05] text-bone-100 sm:text-5xl lg:text-6xl">
            Stories that keep going after you put the headphones down.
          </h1>
          <p className="mt-6 max-w-lg text-lg text-bone-300">
            Auralis is a home for original serialized fiction in audio. Follow a show, stream an
            episode, and pick up exactly where you stopped on any device. Some series are written and
            voiced by our creators. Others you build yourself, from a sentence.
          </p>
          <div className="mt-8 flex flex-wrap gap-3">
            <Link href="/register" className="btn-primary">
              Create a free account
            </Link>
            <Link href="/discover" className="btn-ghost">
              Browse the catalog
            </Link>
          </div>
          <ul className="mt-10 grid gap-4 text-sm text-bone-300 sm:grid-cols-3">
            <li className="surface p-4">
              <WaveIcon className="mb-2 h-5 w-5 text-amber" />
              Resume playback across devices, down to the second.
            </li>
            <li className="surface p-4">
              <SparkIcon className="mb-2 h-5 w-5 text-signal" />
              Generate a full AI series, edit the scripts, then publish.
            </li>
            <li className="surface p-4">
              <span className="mb-2 block text-lg text-amber">&#9670;</span>
              A recommendation feed that actually changes as you listen.
            </li>
          </ul>
        </div>

        <aside className="surface animate-fade-up p-5" style={{ animationDelay: "120ms" }}>
          <p className="eyebrow mb-3">Trending right now</p>
          <div className="space-y-2">
            {(trending ?? []).slice(0, 6).map((t) => (
              <ShowCardCompact key={t.show_id} title={t.title} slug={t.slug} meta={`${t.plays} plays`} />
            ))}
            {!trending?.length && (
              <p className="px-1 py-8 text-center text-sm text-bone-400">
                The catalog is warming up. Check back in a moment.
              </p>
            )}
          </div>
        </aside>
      </section>

      {/* The AI studio: the loudest thing on the page after the headline. */}
      <section className="relative overflow-hidden rounded-2xl border border-signal/30 bg-ink-900/70 p-8 backdrop-blur animate-tide-in sm:p-12">
        <div className="pointer-events-none absolute -right-16 -top-16 h-64 w-64 rounded-full bg-signal/10 blur-3xl" />
        <div className="relative grid gap-8 lg:grid-cols-[1fr_0.85fr] lg:items-center">
          <div>
            <p className="eyebrow mb-3 flex items-center gap-2">
              <LogoMark className="h-4 w-4" /> Build it yourself
            </p>
            <h2 className="font-display text-3xl text-bone-100 sm:text-4xl">
              Give it <span className="text-tide">one sentence</span>. Get back a whole season.
            </h2>
            <p className="mt-4 max-w-lg text-bone-300">
              The generation pipeline writes a story bible, outlines every episode, drafts the full
              scripts, then synthesizes and packages the audio. You review the scripts and decide
              what gets published. Nothing goes live on its own.
            </p>
            <div className="mt-6 flex flex-wrap gap-3">
              <Link href="/register" className="btn-tide">
                <SparkIcon className="h-4 w-4" /> Start with a free account
              </Link>
              <Link href="/discover" className="btn-quiet text-signal-soft">
                Hear what people have made
              </Link>
            </div>
          </div>
          <div className="surface space-y-2 p-4">
            <p className="text-xs uppercase tracking-[0.2em] text-bone-400">Try a brief</p>
            {BRIEF_SAMPLES.map((s) => (
              <Link
                key={s}
                href="/register"
                className="block rounded-lg border border-ink-700 bg-ink-950/60 p-3 text-sm text-bone-200 transition hover:border-signal/40 hover:text-bone-100"
              >
                &ldquo;{s}&rdquo;
              </Link>
            ))}
          </div>
        </div>
      </section>

      <section className="py-12">
        <div className="grid gap-8 sm:grid-cols-2 lg:grid-cols-4">
          {[
            ["Discover", "Filter by genre, language, and mood.", "/discover"],
            ["Continue Listening", "Every unfinished episode, in one shelf.", "/library"],
            ["Premium", "Unlock premium series with a promo code.", "/premium"],
            ["Creator tools", "Build shows, upload audio, watch analytics.", "/register"],
          ].map(([title, copy, href]) => (
            <Link key={title} href={href} className="surface-interactive p-5">
              <p className="font-display text-lg text-bone-100">{title}</p>
              <p className="mt-1 text-sm text-bone-300">{copy}</p>
            </Link>
          ))}
        </div>
      </section>
    </div>
  );
}
