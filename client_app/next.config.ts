import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  /* config options here */
  output: process.env.DOCKER_BUILD === "true" ? "standalone" : undefined,
  reactCompiler: true,
};

export default nextConfig;
