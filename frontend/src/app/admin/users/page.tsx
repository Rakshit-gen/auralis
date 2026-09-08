"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import { Spinner } from "@/components/ui";
import { useState } from "react";
import { relativeTime } from "@/lib/format";

type AdminUser = {
  id: string;
  email: string;
  roles: string[];
  status: string;
  display_name: string;
  created_at: string;
};

const ALL_ROLES = ["USER", "CREATOR", "ADMIN"];

export default function AdminUsersPage() {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const { data, isLoading } = useQuery({
    queryKey: ["admin-users"],
    queryFn: () => api<{ users: AdminUser[] }>("/admin/users", { query: { limit: 100 } }).then((r) => r.users),
  });

  const setRoles = useMutation({
    mutationFn: (input: { id: string; roles: string[] }) =>
      api(`/admin/users/${input.id}/roles`, { method: "POST", body: { roles: input.roles } }),
    onError: (e) => setError(e instanceof ApiError ? e.message : "Could not update roles"),
    onSuccess: () => {
      setError(null);
      qc.invalidateQueries({ queryKey: ["admin-users"] });
    },
  });

  const setStatus = useMutation({
    mutationFn: (input: { id: string; status: string }) =>
      api(`/admin/users/${input.id}/status`, { method: "POST", body: { status: input.status } }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["admin-users"] }),
  });

  const grantPremium = useMutation({
    mutationFn: (id: string) =>
      api(`/admin/entitlements/users/${id}/entitlement`, {
        method: "POST",
        body: { plan: "premium", duration_days: 365 },
      }),
  });

  if (isLoading) return <Spinner />;

  return (
    <div className="space-y-4">
      {error && <p className="text-sm text-red-400">{error}</p>}
      <div className="overflow-x-auto">
        <table className="w-full min-w-[760px] text-sm">
          <thead>
            <tr className="border-b border-ink-700 text-left text-xs uppercase tracking-wide text-bone-500">
              <th className="py-2 pr-3">User</th>
              <th className="py-2 pr-3">Roles</th>
              <th className="py-2 pr-3">Status</th>
              <th className="py-2 pr-3">Joined</th>
              <th className="py-2 pr-3">Actions</th>
            </tr>
          </thead>
          <tbody>
            {data?.map((u) => (
              <tr key={u.id} className="border-b border-ink-800">
                <td className="py-3 pr-3">
                  <p className="text-bone-100">{u.display_name}</p>
                  <p className="text-xs text-bone-400">{u.email}</p>
                </td>
                <td className="py-3 pr-3">
                  <div className="flex flex-wrap gap-1">
                    {ALL_ROLES.map((role) => {
                      const has = u.roles.includes(role);
                      return (
                        <button
                          key={role}
                          onClick={() =>
                            setRoles.mutate({
                              id: u.id,
                              roles: has ? u.roles.filter((r) => r !== role) : [...u.roles, role],
                            })
                          }
                          className={`rounded border px-1.5 py-0.5 text-xs ${
                            has ? "border-amber bg-amber/15 text-amber-soft" : "border-ink-600 text-bone-400"
                          }`}
                        >
                          {role}
                        </button>
                      );
                    })}
                  </div>
                </td>
                <td className="py-3 pr-3">
                  <span className={u.status === "active" ? "text-green-400" : "text-red-400"}>{u.status}</span>
                </td>
                <td className="py-3 pr-3 text-bone-400">{relativeTime(u.created_at)}</td>
                <td className="py-3 pr-3">
                  <div className="flex flex-wrap gap-1.5">
                    <button
                      className="btn-quiet px-2 py-1 text-xs"
                      onClick={() =>
                        setStatus.mutate({ id: u.id, status: u.status === "active" ? "suspended" : "active" })
                      }
                    >
                      {u.status === "active" ? "Suspend" : "Reactivate"}
                    </button>
                    <button className="btn-quiet px-2 py-1 text-xs" onClick={() => grantPremium.mutate(u.id)}>
                      Grant premium
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
