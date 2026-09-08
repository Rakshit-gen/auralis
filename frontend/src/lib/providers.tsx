"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { useAuth } from "@/stores/auth";

export function Providers({ children }: { children: React.ReactNode }) {
  const clientRef = useRef<QueryClient | null>(null);
  if (!clientRef.current) {
    clientRef.current = new QueryClient({
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

  const init = useAuth((s) => s.init);
  useEffect(() => {
    void init();
  }, [init]);

  return <QueryClientProvider client={clientRef.current}>{children}</QueryClientProvider>;
}
