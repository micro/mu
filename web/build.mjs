import { build } from "vite";
import { readFile, writeFile, rm, readdir, unlink } from "node:fs/promises";
for (const name of await readdir("dist").catch(() => []))
  await unlink("dist/" + name);
await build();
await build({
  build: {
    ssr: "src/prerender.tsx",
    outDir: ".prerender",
    rollupOptions: { output: { entryFileNames: "render.mjs" } },
  },
});
const { render } = await import("./.prerender/render.mjs");
const html = await readFile("dist/index.html", "utf8");
await writeFile(
  "dist/index.html",
  html.replace(
    '<div id="root"></div>',
    '<div id="root"><!--landing-->' + render() + "<!--/landing--></div>",
  ),
);
await rm(".prerender", { recursive: true, force: true });
