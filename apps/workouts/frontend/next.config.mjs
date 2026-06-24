/**
 * Pocketknife serves this app's *pre-built static export* at /ui/workouts/...
 * from the same Go origin as the API, so:
 *   - output: 'export'     -> emit a static bundle into ./out (manifest dist)
 *   - basePath/assetPrefix -> all _next asset URLs are rooted at /ui/workouts
 *   - images.unoptimized   -> there is no Node image-optimizer server in prod
 *   - trailingSlash        -> '/ui/workouts/' resolves to index.html cleanly,
 *                             matching the assets server's SPA entry fallback
 * API calls use origin-absolute paths (/apps/workouts/...) and are NOT subject
 * to basePath, so they hit the pocketknife API on the same origin (no CORS).
 *
 * @type {import('next').NextConfig}
 */
const nextConfig = {
  output: "export",
  basePath: "/ui/workouts",
  assetPrefix: "/ui/workouts",
  images: { unoptimized: true },
  trailingSlash: true,
  reactStrictMode: true,
};

export default nextConfig;
