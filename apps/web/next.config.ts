import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Standalone output keeps the production image small enough to rebuild
  // comfortably on the Mac mini.
  output: "standalone",
};

export default nextConfig;
