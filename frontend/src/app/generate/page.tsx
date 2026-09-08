"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { RequireAuth } from "@/components/layout/require-auth";
import { api, ApiError } from "@/lib/api";
import { useQuery } from "@tanstack/react-query";
import { useGenres, useLanguages } from "@/lib/hooks";
import { SparkIcon } from "@/components/icons";
import { LogoMark } from "@/components/logo";
import { ProgressBar } from "@/components/ui";
import type { GenerationJob } from "@/lib/types";

const STAGES = [
  { key: "bible", label: "Story bible", at: 0 },
  { key: "outline", label: "Episode outlines", at: 20 },
  { key: "script", label: "Full scripts", at: 40 },
  { key: "audio", label: "Voice synthesis", at: 70 },
  { key: "package", label: "Packaging", at: 90 },
];

// Languages the generator can actually write and voice natively. Mirrors
// services/ai_media/auralis_ai_media/languages.py; anything else generates in
// English until its models land.
const NATIVE_LANGUAGES = new Set(["en", "hi", "es"]);

const SAMPLE_BRIEFS = [
  "A night-shift paramedic in a coastal city starts getting radio calls from addresses that do not exist yet.",
  "Two marine biologists share a research station on a shrinking island and one working radio.",
  "A deep-sea salvage crew keeps pulling up pieces of a ship that was never built.",
  "Every winter a small harbour town votes on which one memory to forget together.",
];

function StageTrack({ progress, status }: { progress: number; status: string }) {
  const done = status === "completed";
  const failed = status === "failed";
  return (
    <ol className="grid gap-2 sm:grid-cols-5">
      {STAGES.map((s) => {
        const active = !failed && (done || progress >= s.at);
        const current = !done && !failed && progress >= s.at && progress < (STAGES[STAGES.indexOf(s) + 1]?.at ?? 101);
        return (
          <li
            key={s.key}
            className={`rounded-lg border px-3 py-2 text-xs transition ${
              active
                ? "border-signal/50 bg-signal/10 text-signal-soft"
                : "border-ink-700 text-bone-400"
            } ${current ? "animate-drift" : ""}`}
          >
            <span className="block font-semibold">{s.label}</span>
            <span className="text-[11px] opacity-70">
              {failed ? "held" : active ? (current ? "working" : "done") : "queued"}
            </span>
          </li>
        );
      })}
    </ol>
  );
}

function JobStatus({ jobId, onSettled }: { jobId: string; onSettled: () => void }) {
  const { data: job } = useQuery({
    queryKey: ["job", jobId],
    queryFn: () => api<GenerationJob>(`/generate/jobs/${jobId}`),
    refetchInterval: (q) => {
      const s = (q.state.data as GenerationJob | undefined)?.status;
      return s === "completed" || s === "failed" ? false : 2500;
    },
  });

  const settled = job?.status === "completed" || job?.status === "failed";
  useEffect(() => {
    if (settled) onSettled();
  }, [settled, onSettled]);

  if (!job) return null;

  return (
    <div className="surface mt-8 space-y-4 p-5 animate-tide-in">
      <div className="flex items-center justify-between">
        <p className="font-display text-lg text-bone-100">Your series is being built</p>
        <span className={`tag ${job.status === "failed" ? "text-red-400" : "text-signal"}`}>{job.status}</span>
      </div>

      <ProgressBar value={job.progress / 100} />
      <StageTrack progress={job.progress} status={job.status} />

      {job.error && <p className="text-sm text-red-400">{job.error}</p>}

      {!!job.events?.length && (
        <ul className="space-y-1 border-t border-ink-700 pt-3 font-mono text-xs text-bone-400">
          {job.events.slice(-8).map((e, i) => (
            <li key={i} className="flex gap-2">
              <span className="text-signal-soft">{e.status}</span>
              <span>{e.note}</span>
            </li>
          ))}
        </ul>
      )}

      {job.status === "completed" && job.show_id && (
        <div className="flex flex-wrap gap-3 pt-1">
          <Link href={`/creator/shows/${job.show_id}`} className="btn-primary text-sm">
            Review the scripts
          </Link>
          <Link href={`/creator/shows/${job.show_id}/review`} className="btn-ghost text-sm">
            Submit for publishing
          </Link>
        </div>
      )}
    </div>
  );
}

function GenerateInner() {
  const { data: genres } = useGenres();
  const { data: languages } = useLanguages();
  const [brief, setBrief] = useState("");
  const [episodeCount, setEpisodeCount] = useState("8");
  const [language, setLanguage] = useState("en");
  const [isPremium, setIsPremium] = useState(false);
  const [jobId, setJobId] = useState<string | null>(null);
  const [jobRunning, setJobRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const handleJobSettled = useCallback(() => setJobRunning(false), []);

  const EPISODE_MIN = 3;
  const EPISODE_MAX = 24;
  const episodeCountNum = (() => {
    const n = parseInt(episodeCount, 10);
    if (Number.isNaN(n)) return 8;
    return Math.min(EPISODE_MAX, Math.max(EPISODE_MIN, n));
  })();

  const canSubmit = brief.trim().length >= 10 && !busy && !jobRunning;

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!canSubmit) return;
    setBusy(true);
    setError(null);
    try {
      const res = await api<{ job_id: string }>("/generate/series", {
        method: "POST",
        body: {
          brief: brief.trim(),
          episode_count: episodeCountNum,
          language_code: language,
          is_premium: isPremium,
        },
      });
      setJobId(res.job_id);
      setJobRunning(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not start generation");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="container-page max-w-3xl">
      <div className="relative overflow-hidden rounded-2xl border border-signal/30 bg-ink-900/70 p-6 backdrop-blur sm:p-8">
        <div className="pointer-events-none absolute -right-12 -top-12 h-48 w-48 rounded-full bg-signal/10 blur-3xl" />
        <p className="eyebrow mb-2 flex items-center gap-2">
          <LogoMark className="h-4 w-4" /> AI story studio
        </p>
        <h1 className="font-display text-3xl text-bone-100 sm:text-4xl">
          Describe a show. <span className="text-tide">Get back a season.</span>
        </h1>
        <p className="mt-3 max-w-xl text-sm text-bone-300">
          The pipeline writes a story bible, outlines every episode, drafts the full scripts, then
          synthesizes and packages the audio. Nothing is published automatically. You review the
          scripts and submit them yourself. It runs whether or not an external model is configured.
        </p>
      </div>

      <form onSubmit={submit} className="mt-6 space-y-5">
        <label className="block text-sm">
          <span className="mb-1 block text-bone-300">Your brief</span>
          <textarea
            className="field h-32 resize-none"
            maxLength={2000}
            value={brief}
            onChange={(e) => setBrief(e.target.value)}
            placeholder="A night-shift paramedic in a coastal city starts getting radio calls from addresses that do not exist yet."
          />
          <span className="mt-1 block text-xs text-bone-500">{brief.length}/2000</span>
        </label>

        <div>
          <p className="mb-2 text-xs uppercase tracking-[0.2em] text-bone-400">Need a starting point?</p>
          <div className="flex flex-wrap gap-2">
            {SAMPLE_BRIEFS.map((s) => (
              <button
                key={s}
                type="button"
                onClick={() => setBrief(s)}
                className="max-w-full rounded-full border border-ink-600 px-3 py-1 text-left text-xs text-bone-300 transition hover:border-signal/40 hover:text-bone-100"
              >
                {s.length > 68 ? s.slice(0, 66).trimEnd() + "…" : s}
              </button>
            ))}
          </div>
        </div>

        <div className="grid gap-4 sm:grid-cols-3">
          <label className="block text-sm">
            <span className="mb-1 block text-bone-300">Episodes</span>
            <input
              type="number"
              inputMode="numeric"
              min={EPISODE_MIN}
              max={EPISODE_MAX}
              value={episodeCount}
              onChange={(e) => setEpisodeCount(e.target.value.replace(/[^0-9]/g, ""))}
              onBlur={() => setEpisodeCount(String(episodeCountNum))}
              className="field"
            />
          </label>
          <label className="block text-sm">
            <span className="mb-1 block text-bone-300">Language</span>
            <select className="field" value={language} onChange={(e) => setLanguage(e.target.value)}>
              {(languages ?? [{ code: "en", name: "English" }]).map((l) => (
                <option key={l.code} value={l.code}>
                  {l.name}
                </option>
              ))}
            </select>
            {!NATIVE_LANGUAGES.has(language) && (
              <span className="mt-1 block text-xs text-bone-500">
                Scripts and voices for this language are still in progress. This series will be
                written and voiced in English for now.
              </span>
            )}
          </label>
          <label className="flex items-end gap-2 pb-2 text-sm text-bone-200">
            <input
              type="checkbox"
              checked={isPremium}
              onChange={(e) => setIsPremium(e.target.checked)}
              className="h-4 w-4 accent-signal"
            />
            Premium series
          </label>
        </div>

        {genres && (
          <p className="text-xs text-bone-500">
            The pipeline picks genres and tags from your brief. Available genres:{" "}
            {genres.map((g) => g.name).join(", ")}.
          </p>
        )}

        {error && <p className="text-sm text-red-400">{error}</p>}

        <div className="space-y-2">
          <button disabled={!canSubmit} className="btn-tide w-full sm:w-auto">
            <SparkIcon className="h-4 w-4" />
            {busy ? "Starting the pipeline" : jobRunning ? "A series is being built" : "Generate the series"}
          </button>
          {jobRunning && (
            <p className="text-xs text-bone-500">
              One series builds at a time. You can start another once this run finishes.
            </p>
          )}
        </div>
      </form>

      {jobId && <JobStatus jobId={jobId} onSettled={handleJobSettled} />}
    </div>
  );
}

export default function GeneratePage() {
  return (
    <RequireAuth>
      <GenerateInner />
    </RequireAuth>
  );
}
