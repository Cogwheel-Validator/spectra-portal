import type { NextConfig } from "next";
import withRspack from 'next-rspack';

const nextConfig: NextConfig = {
  /* config options here */
  reactCompiler: true,
};

export default withRspack(nextConfig);
