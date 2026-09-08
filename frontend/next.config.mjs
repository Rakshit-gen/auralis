/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  // Standalone output is only for the self-hosted Docker image; the Dockerfile
  // sets NEXT_OUTPUT=standalone. On Vercel it is left unset (Vercel does its
  // own tracing and the standalone step breaks its build pipeline).
  output: process.env.NEXT_OUTPUT === "standalone" ? "standalone" : undefined,
  async rewrites() {
    // In local dev the browser talks to the gateway directly via NEXT_PUBLIC_API_BASE.
    // This rewrite lets same-origin deployments proxy /api to the gateway.
    const gateway = process.env.GATEWAY_INTERNAL_URL;
    return gateway ? [{ source: "/api/:path*", destination: `${gateway}/api/:path*` }] : [];
  },
};

export default nextConfig;
