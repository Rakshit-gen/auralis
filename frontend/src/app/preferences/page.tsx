"use client";

import { useEffect, useState } from "react";
import { RequireAuth } from "@/components/layout/require-auth";
import { api, ApiError } from "@/lib/api";
import { useGenres, useLanguages, usePreferences } from "@/lib/hooks";
import { Chip, Spinner } from "@/components/ui";

function PreferencesInner() {
  const { data: prefs, isLoading, refetch } = usePreferences();
  const { data: genres } = useGenres();
  const { data: languages } = useLanguages();

  const [genreSlugs, setGenreSlugs] = useState<string[]>([]);
  const [languageCodes, setLanguageCodes] = useState<string[]>([]);
  const [autoplay, setAutoplay] = useState(true);
  const [speed, setSpeed] = useState(1);
  const [explicitOk, setExplicitOk] = useState(true);
  const [emailUpdates, setEmailUpdates] = useState(true);
  const [status, setStatus] = useState<string | null>(null);

  useEffect(() => {
    if (!prefs) return;
    setGenreSlugs((prefs.genre_slugs as string[]) ?? []);
    setLanguageCodes((prefs.language_codes as string[]) ?? []);
    setAutoplay((prefs.autoplay as boolean) ?? true);
    setSpeed((prefs.playback_speed as number) ?? 1);
    setExplicitOk((prefs.explicit_ok as boolean) ?? true);
    setEmailUpdates((prefs.email_updates as boolean) ?? true);
  }, [prefs]);

  const toggle = (list: string[], set: (v: string[]) => void, value: string) =>
    set(list.includes(value) ? list.filter((x) => x !== value) : [...list, value]);

  const save = async () => {
    setStatus(null);
    try {
      await api("/me/preferences", {
        method: "PUT",
        body: {
          genre_slugs: genreSlugs,
          language_codes: languageCodes,
          autoplay,
          playback_speed: speed,
          explicit_ok: explicitOk,
          email_updates: emailUpdates,
        },
      });
      await refetch();
      setStatus("Preferences saved. Your feed will update as you listen.");
    } catch (err) {
      setStatus(err instanceof ApiError ? err.message : "Could not save");
    }
  };

  if (isLoading) return <Spinner />;

  return (
    <div className="container-page max-w-3xl space-y-8">
      <div>
        <p className="eyebrow mb-1">Personalization</p>
        <h1 className="font-display text-3xl text-bone-100">Preferences</h1>
        <p className="mt-2 text-sm text-bone-300">
          These seed your recommendations before you have much listening history.
        </p>
      </div>

      <section className="surface p-6">
        <h2 className="mb-3 font-display text-lg text-bone-100">Genres you like</h2>
        <div className="flex flex-wrap gap-2">
          {genres?.map((g) => (
            <Chip key={g.id} active={genreSlugs.includes(g.slug)} onClick={() => toggle(genreSlugs, setGenreSlugs, g.slug)}>
              {g.name}
            </Chip>
          ))}
        </div>
      </section>

      <section className="surface p-6">
        <h2 className="mb-3 font-display text-lg text-bone-100">Languages</h2>
        <div className="flex flex-wrap gap-2">
          {languages?.map((l) => (
            <Chip
              key={l.code}
              active={languageCodes.includes(l.code)}
              onClick={() => toggle(languageCodes, setLanguageCodes, l.code)}
            >
              {l.name}
            </Chip>
          ))}
        </div>
      </section>

      <section className="surface space-y-4 p-6">
        <h2 className="font-display text-lg text-bone-100">Playback</h2>
        <label className="flex items-center justify-between text-sm">
          <span className="text-bone-200">Autoplay the next episode</span>
          <input type="checkbox" checked={autoplay} onChange={(e) => setAutoplay(e.target.checked)} className="h-4 w-4 accent-amber" />
        </label>
        <label className="flex items-center justify-between text-sm">
          <span className="text-bone-200">Default speed</span>
          <select className="field w-auto py-1" value={speed} onChange={(e) => setSpeed(Number(e.target.value))}>
            {[0.75, 1, 1.25, 1.5, 1.75, 2].map((s) => (
              <option key={s} value={s}>
                {s}x
              </option>
            ))}
          </select>
        </label>
        <label className="flex items-center justify-between text-sm">
          <span className="text-bone-200">Include mature content in recommendations</span>
          <input type="checkbox" checked={explicitOk} onChange={(e) => setExplicitOk(e.target.checked)} className="h-4 w-4 accent-amber" />
        </label>
        <label className="flex items-center justify-between text-sm">
          <span className="text-bone-200">Email me about new episodes from shows I follow</span>
          <input type="checkbox" checked={emailUpdates} onChange={(e) => setEmailUpdates(e.target.checked)} className="h-4 w-4 accent-amber" />
        </label>
      </section>

      <div className="flex items-center gap-3">
        <button onClick={save} className="btn-primary">
          Save preferences
        </button>
        {status && <span className="text-sm text-bone-400">{status}</span>}
      </div>
    </div>
  );
}

export default function PreferencesPage() {
  return (
    <RequireAuth>
      <PreferencesInner />
    </RequireAuth>
  );
}
