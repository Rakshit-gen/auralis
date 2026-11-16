// Tokens live in localStorage, so the main defence is keeping other people's
// scripts off the page. Scripts may only come from this origin (plus Vercel
// Analytics). 'unsafe-inline' stays because Next inlines its bootstrap
// scripts and nonces would make every page dynamic. connect-src and
// media-src allow any https origin since hls.js fetches segments straight
// from object storage, whose host differs per deployment.
const dev = process.env.NODE_ENV !== "production";
const csp = [
  "default-src 'self'",
  `script-src 'self' 'unsafe-inline' https://va.vercel-scripts.com${dev ? " 'unsafe-eval'" : ""}`,
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: blob: https:",
  "font-src 'self' data:",
  `connect-src 'self' https:${dev ? " http://localhost:* ws://localhost:*" : ""}`,
  `media-src 'self' blob: https:${dev ? " http://localhost:*" : ""}`,
  "worker-src 'self' blob:",
  "object-src 'none'",
  "base-uri 'self'",
  "form-action 'self'",
  "frame-ancestors 'none'",
].join("; ");

/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  async headers() {
    return [
      {
        source: "/:path*",
        headers: [
          { key: "Content-Security-Policy", value: csp },
          { key: "X-Content-Type-Options", value: "nosniff" },
          { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
        ],
      },
    ];
  },
  // Standalone output is only for the self-hosted Docker image; the Dockerfile
  // sets NEXT_OUTPUT=standalone. On Vercel it is left unset (Vercel does its
  // own tracing and the standalone step breaks its build pipeline).
  output: process.env.NEXT_OUTPUT === "standalone" ? "standalone" : undefined,
  async rewrites() {
    // In local dev the browser talks to the gateway directly via NEXT_PUBLIC_API_BASE.
    // This rewrite lets same-origin deployments proxy /api to the gateway.
    const gateway = process.env.GATEWAY_INTERNAL_URL;
    return gateway
      ? [{ source: "/api/:path*", destination: `${gateway}/api/:path*` }]
      : [];
  },
};

export default nextConfig;
