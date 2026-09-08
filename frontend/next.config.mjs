/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  output: "standalone",
  async rewrites() {
    // In local dev the browser talks to the gateway directly via NEXT_PUBLIC_API_BASE.
    // This rewrite lets same-origin deployments proxy /api to the gateway.
    const gateway = process.env.GATEWAY_INTERNAL_URL;
    return gateway ? [{ source: "/api/:path*", destination: `${gateway}/api/:path*` }] : [];
  },
};

export default nextConfig;
