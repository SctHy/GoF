import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath } from "node:url";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  server: {
    host: "127.0.0.1",
    port: Number(process.env.GCFEED_WEB_PORT || 15173),
    strictPort: true,
    proxy: {
      "/api": {
        target: process.env.GCFEED_API_PROXY || "http://127.0.0.1:18080",
        changeOrigin: true
      },
      "/uploads": {
        target: process.env.GCFEED_API_PROXY || "http://127.0.0.1:18080",
        changeOrigin: true
      }
    }
  }
});
