"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { Spinner } from "@/components/ui";
import { relativeTime } from "@/lib/format";
import type { GenerationJob } from "@/lib/types";

const STATUS_STYLE: Record<string, string> = {
  completed: "text-green-400",
  failed: "text-red-400",
  queued: "text-bone-400",
};

const FILTERS = ["", "queued", "generating_script", "packaging", "completed", "failed"];

export default function JobsPage() {
  const [status, setStatus] = useState("");
  const { data, isLoading } = useQuery({
    queryKey: ["admin-jobs", status],
    queryFn: () =>
      api<{ jobs: GenerationJob[] }>("/ai/jobs", { query: { status: status || undefined, limit: 100 } }).then(
        (r) => r.jobs,
      ),
    refetchInterval: 8_000,
  });

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap gap-2">
        {FILTERS.map((f) => (
          <button
            key={f || "all"}
            onClick={() => setStatus(f)}
            className={`rounded-full border px-3 py-1 text-sm ${
              status === f ? "border-amber bg-amber/15 text-amber-soft" : "border-ink-600 text-bone-300"
            }`}
          >
            {f ? f.replace(/_/g, " ") : "all"}
          </button>
        ))}
      </div>

      {isLoading ? (
        <Spinner />
      ) : !data?.length ? (
        <p className="text-sm text-bone-400">No jobs match this filter.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[720px] text-sm">
            <thead>
              <tr className="border-b border-ink-700 text-left text-xs uppercase tracking-wide text-bone-500">
                <th className="py-2 pr-3">Kind</th>
                <th className="py-2 pr-3">Status</th>
                <th className="py-2 pr-3">Progress</th>
                <th className="py-2 pr-3">Provider</th>
                <th className="py-2 pr-3">Attempts</th>
                <th className="py-2 pr-3">Created</th>
                <th className="py-2 pr-3">Detail</th>
              </tr>
            </thead>
            <tbody>
              {data.map((j) => (
                <tr key={j.id} className="border-b border-ink-800 align-top">
                  <td className="py-2 pr-3 text-bone-200">{j.kind}</td>
                  <td className={`py-2 pr-3 ${STATUS_STYLE[j.status] ?? "text-signal"}`}>
                    {j.status.replace(/_/g, " ")}
                  </td>
                  <td className="py-2 pr-3 text-bone-300">{j.progress}%</td>
                  <td className="py-2 pr-3 text-bone-300">{j.provider}</td>
                  <td className="py-2 pr-3 text-bone-300">{j.attempts}</td>
                  <td className="py-2 pr-3 text-bone-400">{relativeTime(j.created_at)}</td>
                  <td className="py-2 pr-3 text-xs text-bone-400">
                    {j.error || j.events?.at(-1)?.note || "-"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
