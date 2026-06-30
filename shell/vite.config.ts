import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3001,
    proxy: {
      "/platform": "http://localhost:8080",
      "/apps": "http://localhost:8080",
      "/builds": "http://localhost:8080",
      "/ui": "http://localhost:8080",
      "/validate": "http://localhost:8080",
      "/deploy": "http://localhost:8080",
      "/export": "http://localhost:8080",
    },
  },
  build: {
    outDir: "dist",
  },
});
