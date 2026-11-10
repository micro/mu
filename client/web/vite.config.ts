import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
// Relative assets work both at /app/ in Mu and at / on an independent host.
export default defineConfig({
  base: "./",
  plugins: [react()],
  build: { outDir: "static", emptyOutDir: true },
});
