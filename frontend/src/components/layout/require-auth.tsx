"use client";

import { useEffect } from "react";
import { useRouter, usePathname } from "next/navigation";
import { useAuth } from "@/stores/auth";
import { Spinner } from "@/components/ui";

export function RequireAuth({
  children,
  role,
}: {
  children: React.ReactNode;
  role?: "CREATOR" | "ADMIN";
}) {
  const { user, ready, hasRole } = useAuth();
  const router = useRouter();
  const pathname = usePathname();

  useEffect(() => {
    if (ready && !user) {
      router.replace(`/login?next=${encodeURIComponent(pathname)}`);
    }
  }, [ready, user, router, pathname]);

  if (!ready || !user) {
    return (
      <div className="container-page py-24">
        <Spinner label="Checking your session" />
      </div>
    );
  }

  if (role && !hasRole(role)) {
    return (
      <div className="container-page py-24">
        <div className="surface px-6 py-10 text-center">
          <p className="font-display text-lg text-bone-100">You need the {role.toLowerCase()} role for this page.</p>
          <p className="mt-2 text-sm text-bone-300">
            Ask an admin to grant it, or head back to your{" "}
            <a href="/home" className="text-amber-soft">
              home feed
            </a>
            .
          </p>
        </div>
      </div>
    );
  }

  return <>{children}</>;
}
