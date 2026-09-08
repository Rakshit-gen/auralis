"use client";

import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { Spinner } from "@/components/ui";

type ServiceStatus = {
  name: string;
  healthy: boolean;
  ready: boolean;
  latency_ms: number;
  detail?: string;
};

export default function HealthPage() {
  const { data, isLoading, dataUpdatedAt } = useQuery({
    queryKey: ["system-status"],
    queryFn: () => api<{ services: ServiceStatus[]; all_ready: boolean }>("/status", { auth: false }),
    refetchInterval: 10_000,
  });

  if (isLoading) return <Spinner />;

  return (
    <div className="space-y-4">
      <div
        className={`surface p-4 ${
          data?.all_ready ? "border-green-800/50" : "border-red-900/50"
        }`}
      >
        <p className="font-display text-lg text-bone-100">
          {data?.all_ready ? "All services ready" : "Some services are not ready"}
        </p>
        <p className="text-xs text-bone-500">Checked {new Date(dataUpdatedAt).toLocaleTimeString()}</p>
      </div>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {data?.services
          .slice()
          .sort((a, b) => a.name.localeCompare(b.name))
          .map((s) => (
            <div key={s.name} className="surface p-4">
              <div className="flex items-center justify-between">
                <p className="font-display text-bone-100">{s.name}</p>
                <span
                  className={`h-2.5 w-2.5 rounded-full ${
                    s.ready ? "bg-green-400" : s.healthy ? "bg-amber" : "bg-red-500"
                  }`}
                />
              </div>
              <dl className="mt-2 space-y-1 text-xs text-bone-400">
                <div className="flex justify-between">
                  <dt>Liveness</dt>
                  <dd className={s.healthy ? "text-green-400" : "text-red-400"}>{s.healthy ? "up" : "down"}</dd>
                </div>
                <div className="flex justify-between">
                  <dt>Readiness</dt>
                  <dd className={s.ready ? "text-green-400" : "text-amber-soft"}>{s.ready ? "ready" : "not ready"}</dd>
                </div>
                <div className="flex justify-between">
                  <dt>Latency</dt>
                  <dd>{s.latency_ms} ms</dd>
                </div>
                {s.detail && (
                  <div className="flex justify-between">
                    <dt>Detail</dt>
                    <dd className="text-red-400">{s.detail}</dd>
                  </div>
                )}
              </dl>
            </div>
          ))}
      </div>
    </div>
  );
}
