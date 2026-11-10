import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

// The dev server proxies API calls to the Go backend. Set API_TARGET (for
// example in web/.env.local) when the backend does not listen on :8080.
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, ".", "API_");
  return {
    plugins: [react()],
    server: {
      host: "127.0.0.1",
      port: 5173,
      proxy: { "/api": env.API_TARGET || "http://127.0.0.1:8080" },
    },
  };
});
