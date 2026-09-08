"use client";

import { RequireAuth } from "@/components/layout/require-auth";
import { LibraryView } from "@/components/library-view";

export default function Page() {
  return (
    <RequireAuth>
      <LibraryView initial="history" />
    </RequireAuth>
  );
}
