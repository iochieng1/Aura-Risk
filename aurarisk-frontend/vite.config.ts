import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => {
  // API_PROXY_TARGET has no VITE_ prefix, so it stays out of the client bundle.
  const env = loadEnv(mode, process.cwd(), "");

  return {
    plugins: [react()],
    server: {
      port: 5173,
      // @aurarisk/shared is linked from ../packages/shared, outside this project root.
      fs: { allow: [".", "../packages/shared"] },
      proxy: {
        "/api": {
          target: env.API_PROXY_TARGET || "http://localhost:8080",
          changeOrigin: true,
        },
      },
    },
  };
});
