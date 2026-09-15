import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { dedupe: ["react", "react-dom", "react-markdown", "remark-gfm", "radix-ui", "lucide-react"] },
  base: "/client/assets/",
  build: {
    outDir: "dist",
    emptyOutDir: true,
    rollupOptions: {
      output: {
        entryFileNames: "client-[hash].js",
        chunkFileNames: "[name]-[hash].js",
        assetFileNames: "client-[hash].[ext]",
      },
    },
  },
});
