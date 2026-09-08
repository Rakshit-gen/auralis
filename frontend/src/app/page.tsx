"use client";

import Link from "next/link";
import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/stores/auth";
import { useTrending } from "@/lib/hooks";
import { ShowCardCompact } from "@/components/show-card";
import { SparkIcon, WaveIcon } from "@/components/icons";

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
            voiced by our creators; others are built by an AI pipeline you can steer yourself.
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
              <span className="mb-2 block text-lg text-amber">◆</span>
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

      <section className="border-t border-ink-800 py-12">
        <div className="grid gap-8 sm:grid-cols-2 lg:grid-cols-4">
          {[
            ["Discover", "Filter by genre, language, and mood.", "/discover"],
            ["Continue Listening", "Every unfinished episode, in one shelf.", "/library"],
            ["Premium", "Unlock premium series with a promo code.", "/premium"],
            ["Creator tools", "Build shows, upload audio, watch analytics.", "/register"],
          ].map(([title, copy, href]) => (
            <Link key={title} href={href} className="surface p-5 transition hover:border-amber/50">
              <p className="font-display text-lg text-bone-100">{title}</p>
              <p className="mt-1 text-sm text-bone-300">{copy}</p>
            </Link>
          ))}
        </div>
      </section>
    </div>
  );
}
