import { readFile, writeFile, readdir } from "node:fs/promises";
const lock = JSON.parse(
  await readFile(new URL("./package-lock.json", import.meta.url), "utf8"),
);
const blocks = [];
for (const [path, pkg] of Object.entries(lock.packages).sort(([a], [b]) =>
  a.localeCompare(b),
)) {
  if (!path || pkg.dev) continue;
  const directory = new URL("./" + path + "/", import.meta.url);
  const names = (await readdir(directory))
    .filter((name) => /^(licen[cs]e|copying)(\.|$)/i.test(name))
    .sort();
  for (const name of names)
    blocks.push(
      path +
        " " +
        pkg.version +
        "\n" +
        (await readFile(new URL(name, directory), "utf8")),
    );
}
blocks.push(
  "DM Sans font\n" +
    (await readFile(
      new URL("../../internal/app/html/dm-sans-LICENSE.txt", import.meta.url),
      "utf8",
    )),
);
await writeFile(
  new URL("./static/THIRD_PARTY_LICENSES.txt", import.meta.url),
  blocks.join("\n\n--------------------\n\n"),
);
