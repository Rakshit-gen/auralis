"use client";

/**
 * Thin client for the Auralis API gateway. Attaches the access token, refreshes
 * it once on a 401, and surfaces the platform error envelope as ApiError.
 */

export const API_BASE =
  process.env.NEXT_PUBLIC_API_BASE?.replace(/\/$/, "") || "/api";

const ACCESS_KEY = "auralis.access";
const REFRESH_KEY = "auralis.refresh";

export type Tokens = {
  access_token: string;
  refresh_token: string;
  expires_at?: string;
};

export function loadTokens(): Tokens | null {
  if (typeof window === "undefined") return null;
  try {
    const access = localStorage.getItem(ACCESS_KEY);
    const refresh = localStorage.getItem(REFRESH_KEY);
    return access && refresh
      ? { access_token: access, refresh_token: refresh }
      : null;
  } catch {
    return null;
  }
}

export function saveTokens(t: Tokens) {
  try {
    localStorage.setItem(ACCESS_KEY, t.access_token);
    localStorage.setItem(REFRESH_KEY, t.refresh_token);
  } catch {
    /* private mode: session stays in memory only */
  }
}

export function clearTokens() {
  try {
    localStorage.removeItem(ACCESS_KEY);
    localStorage.removeItem(REFRESH_KEY);
  } catch {
    /* ignore */
  }
}

export class ApiError extends Error {
  code: string;
  status: number;
  fields?: Record<string, string>;
  constructor(
    status: number,
    code: string,
    message: string,
    fields?: Record<string, string>,
  ) {
    super(message);
    this.status = status;
    this.code = code;
    this.fields = fields;
  }
}

let refreshInFlight: Promise<Tokens | null> | null = null;

// A refresh token works once: the server treats a second use as theft and
// signs the user out everywhere. Tabs share tokens through localStorage, so
// a lock makes them refresh one at a time, and a tab that gets the lock after
// another tab refreshed just picks up the new tokens.
export async function refreshTokens(): Promise<Tokens | null> {
  const tokens = loadTokens();
  if (!tokens) return null;
  if (!refreshInFlight) {
    const locks =
      typeof navigator !== "undefined" ? navigator.locks : undefined;
    const run = () => refreshOnce(tokens.refresh_token);
    // The DOM types give request() a Promise<Promise<T>> here; .then unwraps
    // the type to match what the browser actually returns.
    const pending: Promise<Tokens | null> = locks
      ? locks.request("auralis.refresh", run).then((t) => t)
      : run();
    refreshInFlight = pending.finally(() => {
      refreshInFlight = null;
    });
  }
  return refreshInFlight;
}

async function refreshOnce(staleRefresh: string): Promise<Tokens | null> {
  const current = loadTokens();
  if (!current) return null;
  if (current.refresh_token !== staleRefresh) return current; // another tab got there first
  return fetch(`${API_BASE}/auth/refresh`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ refresh_token: current.refresh_token }),
  })
    .then(async (res) => {
      if (!res.ok) {
        clearTokens();
        return null;
      }
      const body = await res.json();
      const next: Tokens = body.tokens ?? body;
      saveTokens(next);
      return next;
    })
    .catch(() => null);
}

type RequestOptions = {
  method?: string;
  body?: unknown;
  auth?: boolean;
  signal?: AbortSignal;
  query?: Record<string, string | number | boolean | undefined>;
};

export async function api<T = unknown>(
  path: string,
  opts: RequestOptions = {},
): Promise<T> {
  const { method = "GET", body, auth = true, signal, query } = opts;

  let url = `${API_BASE}${path}`;
  if (query) {
    const qs = new URLSearchParams();
    for (const [k, v] of Object.entries(query)) {
      if (v !== undefined && v !== "") qs.set(k, String(v));
    }
    const s = qs.toString();
    if (s) url += `?${s}`;
  }

  const doFetch = async (token?: string): Promise<Response> => {
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
    };
    if (token) headers.Authorization = `Bearer ${token}`;
    return fetch(url, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
      signal,
    });
  };

  const tokens = auth ? loadTokens() : null;
  let res = await doFetch(tokens?.access_token);

  if (res.status === 401 && auth && tokens) {
    const refreshed = await refreshTokens();
    if (refreshed) {
      res = await doFetch(refreshed.access_token);
    }
  }

  if (res.status === 204) return undefined as T;

  let payload: unknown = null;
  const text = await res.text();
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = {
        error: { code: "client.bad_response", message: text.slice(0, 200) },
      };
    }
  }

  if (!res.ok) {
    const err = (
      payload as {
        error?: {
          code?: string;
          message?: string;
          fields?: Record<string, string>;
        };
      }
    )?.error;
    throw new ApiError(
      res.status,
      err?.code ?? "client.error",
      err?.message ?? `request failed (${res.status})`,
      err?.fields,
    );
  }

  return payload as T;
}
