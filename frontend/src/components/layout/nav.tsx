"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useState } from "react";
import { useAuth } from "@/stores/auth";
import { SearchIcon, SparkIcon, WaveIcon } from "@/components/icons";

const NAV = [
  { href: "/home", label: "Home", auth: true },
  { href: "/discover", label: "Discover" },
  { href: "/trending", label: "Trending" },
  { href: "/genres", label: "Genres" },
  { href: "/library", label: "Library", auth: true },
];

export function Nav() {
  const pathname = usePathname();
  const router = useRouter();
  const { user, hasRole, logout } = useAuth();
  const [q, setQ] = useState("");
  const [menuOpen, setMenuOpen] = useState(false);

  const submitSearch = (e: React.FormEvent) => {
    e.preventDefault();
    if (q.trim()) router.push(`/search?q=${encodeURIComponent(q.trim())}`);
  };

  return (
    <header className="sticky top-0 z-20 border-b border-ink-800 bg-ink-950/85 backdrop-blur">
      <div className="container-page flex h-16 items-center gap-4">
        <Link href={user ? "/home" : "/"} className="flex items-center gap-2">
          <WaveIcon className="h-6 w-6 text-amber" />
          <span className="font-display text-xl tracking-tight text-bone-100">Auralis</span>
        </Link>

        <nav className="hidden items-center gap-1 md:flex">
          {NAV.filter((n) => !n.auth || user).map((n) => (
            <Link
              key={n.href}
              href={n.href}
              className={`rounded-lg px-3 py-1.5 text-sm transition ${
                pathname === n.href || pathname.startsWith(n.href + "/")
                  ? "text-amber-soft"
                  : "text-bone-300 hover:text-bone-100"
              }`}
            >
              {n.label}
            </Link>
          ))}
        </nav>

        <form onSubmit={submitSearch} className="ml-auto hidden items-center sm:flex">
          <div className="relative">
            <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-500" />
            <input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="Search stories"
              aria-label="Search"
              className="field w-48 pl-8 lg:w-64"
            />
          </div>
        </form>

        {user ? (
          <div className="relative">
            <button
              onClick={() => setMenuOpen((o) => !o)}
              className="grid h-9 w-9 place-items-center rounded-full border border-ink-600 text-sm font-semibold text-amber-soft"
              aria-haspopup="menu"
              aria-expanded={menuOpen}
            >
              {user.display_name.slice(0, 1).toUpperCase()}
            </button>
            {menuOpen && (
              <div
                className="surface absolute right-0 mt-2 w-52 overflow-hidden py-1 text-sm"
                onMouseLeave={() => setMenuOpen(false)}
              >
                <div className="border-b border-ink-700 px-3 py-2">
                  <p className="truncate text-bone-100">{user.display_name}</p>
                  <p className="truncate text-xs text-bone-400">{user.email}</p>
                </div>
                <MenuLink href="/generate" onClick={() => setMenuOpen(false)}>
                  <SparkIcon className="h-4 w-4 text-signal" /> Generate a story
                </MenuLink>
                <MenuLink href="/profile" onClick={() => setMenuOpen(false)}>
                  Profile
                </MenuLink>
                <MenuLink href="/preferences" onClick={() => setMenuOpen(false)}>
                  Preferences
                </MenuLink>
                <MenuLink href="/premium" onClick={() => setMenuOpen(false)}>
                  Premium
                </MenuLink>
                {hasRole("CREATOR") && (
                  <MenuLink href="/creator" onClick={() => setMenuOpen(false)}>
                    Creator dashboard
                  </MenuLink>
                )}
                {hasRole("ADMIN") && (
                  <MenuLink href="/admin" onClick={() => setMenuOpen(false)}>
                    Admin
                  </MenuLink>
                )}
                <button
                  onClick={() => {
                    setMenuOpen(false);
                    void logout();
                    router.push("/");
                  }}
                  className="block w-full px-3 py-2 text-left text-bone-300 hover:bg-ink-800 hover:text-bone-100"
                >
                  Sign out
                </button>
              </div>
            )}
          </div>
        ) : (
          <div className="ml-auto flex items-center gap-2 sm:ml-0">
            <Link href="/login" className="btn-quiet text-sm">
              Sign in
            </Link>
            <Link href="/register" className="btn-primary text-sm">
              Join
            </Link>
          </div>
        )}
      </div>

      <nav className="flex gap-1 overflow-x-auto border-t border-ink-800 px-4 py-2 md:hidden">
        {NAV.filter((n) => !n.auth || user).map((n) => (
          <Link
            key={n.href}
            href={n.href}
            className={`shrink-0 rounded-lg px-3 py-1 text-sm ${
              pathname === n.href ? "bg-ink-800 text-amber-soft" : "text-bone-300"
            }`}
          >
            {n.label}
          </Link>
        ))}
      </nav>
    </header>
  );
}

function MenuLink({ href, onClick, children }: { href: string; onClick: () => void; children: React.ReactNode }) {
  return (
    <Link
      href={href}
      onClick={onClick}
      className="flex items-center gap-2 px-3 py-2 text-bone-300 hover:bg-ink-800 hover:text-bone-100"
    >
      {children}
    </Link>
  );
}
