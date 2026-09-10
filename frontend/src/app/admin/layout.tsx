"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { RequireAuth } from "@/components/layout/require-auth";

const TABS = [
  { href: "/admin", label: "Overview" },
  { href: "/admin/review", label: "Content review" },
  { href: "/admin/jobs", label: "Processing jobs" },
  { href: "/admin/users", label: "Users" },
  { href: "/admin/health", label: "System health" },
];

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  return (
    <RequireAuth role="ADMIN">
      <div className="container-page">
        <p className="eyebrow mb-1">Operations</p>
        <h1 className="mb-4 font-display text-3xl text-bone-100">Admin</h1>
        <nav className="mb-6 flex gap-1 overflow-x-auto border-b border-ink-800">
          {TABS.map((t) => {
            const active =
              t.href === "/admin"
                ? pathname === t.href
                : pathname === t.href || pathname.startsWith(t.href + "/");
            return (
              <Link
                key={t.href}
                href={t.href}
                className={`shrink-0 border-b-2 px-4 py-2 text-sm transition ${
                  active
                    ? "border-amber text-amber-soft"
                    : "border-transparent text-bone-300 hover:text-bone-100"
                }`}
              >
                {t.label}
              </Link>
            );
          })}
        </nav>
        {children}
      </div>
    </RequireAuth>
  );
}
