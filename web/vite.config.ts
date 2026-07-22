import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "dist",
    emptyOutDir: false,
    sourcemap: false,
    chunkSizeWarningLimit: 600
  },
  server: {
    host: "127.0.0.1",
    port: 5173,
    proxy: {
      "/api": { target: "http://127.0.0.1:3000", ws: true },
      "/healthz": "http://127.0.0.1:3000",
      "/readyz": "http://127.0.0.1:3000"
    }
  },
  test: {
    environment: "jsdom",
    environmentOptions: { jsdom: { url: "http://localhost:5173/" } }
  }
});
