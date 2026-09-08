"use client";

import { usePlayer } from "@/stores/player";
import { PlayIcon } from "@/components/icons";

export function QueuePanel() {
  const queue = usePlayer((s) => s.queue);
  const index = usePlayer((s) => s.index);
  const setQueue = usePlayer((s) => s.setQueue);

  if (queue.length <= 1) return null;

  return (
    <div className="w-full max-w-xl">
      <p className="eyebrow mb-2">Up next</p>
      <ol className="max-h-48 space-y-1 overflow-y-auto">
        {queue.map((q, i) => (
          <li key={q.episodeId}>
            <button
              onClick={() => void setQueue(queue, i)}
              className={`flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-sm transition ${
                i === index ? "bg-amber/15 text-amber-soft" : "text-bone-300 hover:bg-ink-800"
              }`}
            >
              <span className="w-5 text-xs text-ink-500">{i + 1}</span>
              {i === index ? <PlayIcon className="h-3.5 w-3.5" /> : <span className="w-3.5" />}
              <span className="truncate">{q.episodeTitle}</span>
            </button>
          </li>
        ))}
      </ol>
    </div>
  );
}
