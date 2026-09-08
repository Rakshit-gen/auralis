"use client";

import { use, useEffect, useState } from "react";
import Link from "next/link";
import { RequireAuth } from "@/components/layout/require-auth";
import { useEpisodeDetail, useSaveScript, useSubmitForReview } from "@/lib/creator";
import { Spinner, ErrorState } from "@/components/ui";
import { ApiError } from "@/lib/api";

function ScriptEditorInner({ episodeId }: { episodeId: string }) {
  const { data, isLoading, error, refetch } = useEpisodeDetail(episodeId);
  const save = useSaveScript();
  const submit = useSubmitForReview();
  const [script, setScript] = useState("");
  const [status, setStatus] = useState<string | null>(null);

  useEffect(() => {
    if (data?.episode) setScript(data.episode.script ?? "");
  }, [data?.episode]);

  if (isLoading) return <Spinner />;
  if (error || !data?.episode) {
    return <ErrorState message={(error as Error)?.message ?? "Episode not found"} retry={() => void refetch()} />;
  }

  const { episode, show } = data;
  const editable = ["draft", "rejected", "ready_for_review"].includes(episode.status);
  const words = script.trim() ? script.trim().split(/\s+/).length : 0;

  const doSave = async () => {
    setStatus(null);
    try {
      await save.mutateAsync({ episodeId, script });
      setStatus("Script saved. The audio will re-generate when you resubmit.");
    } catch (e) {
      setStatus(e instanceof ApiError ? e.message : "Could not save");
    }
  };

  return (
    <div className="container-page space-y-6">
      <div>
        <Link href={`/creator/shows/${show.id}`} className="text-sm text-bone-400 hover:text-bone-200">
          ← {show.title}
        </Link>
        <h1 className="mt-2 font-display text-3xl text-bone-100">
          EP {episode.number}: {episode.title}
        </h1>
        <p className="mt-1 text-sm text-bone-400">
          {episode.ai_generated ? "AI-generated draft" : "Uploaded episode"} · status{" "}
          {episode.status.replace(/_/g, " ")}
        </p>
      </div>

      {!editable && (
        <p className="surface p-4 text-sm text-bone-300">
          This script is locked because the episode is {episode.status.replace(/_/g, " ")}. Only draft,
          rejected, or in-review episodes can be edited.
        </p>
      )}

      <div className="surface p-1">
        <textarea
          value={script}
          onChange={(e) => setScript(e.target.value)}
          disabled={!editable}
          spellCheck
          className="h-[55vh] w-full resize-none rounded-lg bg-ink-950 p-4 font-mono text-sm leading-relaxed text-bone-100 focus:outline-none disabled:opacity-60"
          placeholder="Narrator: ...\nCharacter: ..."
        />
      </div>
      <div className="flex flex-wrap items-center gap-3 text-sm">
        <span className="text-bone-400">{words} words</span>
        <button disabled={!editable || save.isPending} onClick={doSave} className="btn-primary">
          {save.isPending ? "Saving" : "Save script"}
        </button>
        <button
          disabled={submit.isPending || !["draft", "rejected"].includes(episode.status)}
          onClick={() => submit.mutate({ kind: "episode", id: episodeId })}
          className="btn-ghost"
        >
          Submit episode for review
        </button>
        {status && <span className="text-bone-400">{status}</span>}
      </div>
    </div>
  );
}

export default function Page({ params }: { params: Promise<{ episodeId: string }> }) {
  const { episodeId } = use(params);
  return (
    <RequireAuth role="CREATOR">
      <ScriptEditorInner episodeId={episodeId} />
    </RequireAuth>
  );
}
