"use client";

import { useState } from "react";
import Link from "next/link";
import { RequireAuth } from "@/components/layout/require-auth";
import { api, ApiError } from "@/lib/api";
import { useQuery } from "@tanstack/react-query";
import { useGenres, useLanguages } from "@/lib/hooks";
import { SparkIcon } from "@/components/icons";
import { ProgressBar } from "@/components/ui";
import type { GenerationJob } from "@/lib/types";

function JobStatus({ jobId }: { jobId: string }) {
  const { data: job } = useQuery({
    queryKey: ["job", jobId],
    queryFn: () => api<GenerationJob>(`/generate/jobs/${jobId}`),
    refetchInterval: (q) => {
      const s = (q.state.data as GenerationJob | undefined)?.status;
      return s === "completed" || s === "failed" ? false : 2500;
    },
  });

  if (!job) return null;

  return (
    <div className="surface mt-6 space-y-3 p-5">
      <div className="flex items-center justify-between">
        <p className="font-display text-lg text-bone-100">Generation job</p>
        <span className={`tag ${job.status === "failed" ? "text-red-400" : "text-signal"}`}>{job.status}</span>
      </div>
      <ProgressBar value={job.progress / 100} />
      {job.error && <p className="text-sm text-red-400">{job.error}</p>}
      <ul className="space-y-1 text-xs text-bone-400">
        {job.events?.slice(-6).map((e, i) => (
          <li key={i}>
            <span className="text-bone-300">{e.status}</span> {e.note}
          </li>
        ))}
      </ul>
      {job.status === "completed" && job.show_id && (
        <Link href={`/creator/shows/${job.show_id}`} className="btn-primary text-sm">
          Open the show
        </Link>
      )}
    </div>
  );
}

function GenerateInner() {
  const { data: genres } = useGenres();
  const { data: languages } = useLanguages();
  const [brief, setBrief] = useState("");
  const [episodeCount, setEpisodeCount] = useState(8);
  const [language, setLanguage] = useState("en");
  const [isPremium, setIsPremium] = useState(false);
  const [jobId, setJobId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (brief.trim().length < 10) return;
    setBusy(true);
    setError(null);
    try {
      const res = await api<{ job_id: string }>("/generate/series", {
        method: "POST",
        body: {
          brief: brief.trim(),
          episode_count: episodeCount,
          language_code: language,
          is_premium: isPremium,
        },
      });
      setJobId(res.job_id);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not start generation");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="container-page max-w-3xl">
      <p className="eyebrow mb-1">AI story pipeline</p>
      <h1 className="flex items-center gap-2 font-display text-3xl text-bone-100">
        <SparkIcon className="h-6 w-6 text-signal" /> Generate a series
      </h1>
      <p className="mt-2 text-sm text-bone-300">
        Describe the show you want. The pipeline writes a story bible, per-episode outlines, full
        scripts, then synthesizes and packages the audio. Nothing is published automatically: you
        review the scripts and submit them yourself. Generation runs whether or not an external model
        is configured.
      </p>

      <form onSubmit={submit} className="mt-6 space-y-4">
        <label className="block text-sm">
          <span className="mb-1 block text-bone-300">Brief</span>
          <textarea
            className="field h-32 resize-none"
            maxLength={2000}
            value={brief}
            onChange={(e) => setBrief(e.target.value)}
            placeholder="A night-shift paramedic in a coastal city starts getting radio calls from addresses that do not exist yet."
          />
          <span className="mt-1 block text-xs text-bone-500">{brief.length}/2000</span>
        </label>

        <div className="grid gap-4 sm:grid-cols-3">
          <label className="block text-sm">
            <span className="mb-1 block text-bone-300">Episodes</span>
            <input
              type="number"
              min={3}
              max={24}
              value={episodeCount}
              onChange={(e) => setEpisodeCount(Number(e.target.value))}
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
          </label>
          <label className="flex items-end gap-2 pb-2 text-sm text-bone-200">
            <input type="checkbox" checked={isPremium} onChange={(e) => setIsPremium(e.target.checked)} className="h-4 w-4 accent-amber" />
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
        <button disabled={busy || brief.trim().length < 10} className="btn-primary">
          {busy ? "Starting" : "Start generation"}
        </button>
      </form>

      {jobId && <JobStatus jobId={jobId} />}
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
