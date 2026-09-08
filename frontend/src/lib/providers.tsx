"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { useAuth } from "@/stores/auth";

function makeClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        retry: (count, err) => {
          const status = (err as { status?: number })?.status ?? 0;
          if (status >= 400 && status < 500) return false;
          return count < 2;
        },
        refetchOnWindowFocus: false,
      },
    },
  });
}

export function Providers({ children }: { children: React.ReactNode }) {
  const [client] = useState(makeClient);

  const init = useAuth((s) => s.init);
  useEffect(() => {
    void init();
  }, [init]);

  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
