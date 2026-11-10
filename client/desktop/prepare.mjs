import { cp, readFile, writeFile, mkdir, rm } from "node:fs/promises";
const base = new URL("./", import.meta.url);
await rm(new URL("web", base), { recursive: true, force: true });
await cp(new URL("../web/static", base), new URL("web", base), {
  recursive: true,
});
await cp(new URL("../../LICENSE", base), new URL("LICENSE", base));
await mkdir(new URL("resources", base), { recursive: true });
await cp(
  new URL("../../internal/app/html/icon-512.png", base),
  new URL("resources/icon.png", base),
);
const path = new URL("package.json", base),
  pkg = JSON.parse(await readFile(path));
pkg.version = (await readFile(new URL("../../VERSION", base), "utf8"))
  .trim()
  .replace(/^v/, "");
await writeFile(path, JSON.stringify(pkg, null, 2) + "\n");
