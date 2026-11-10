#!/usr/bin/env node
// Serve the built client, not the runtime. No proxy, credentials or model keys.
import { createServer } from "node:http";
import { readFile, stat } from "node:fs/promises";
import { resolve, sep, extname } from "node:path";
import { fileURLToPath } from "node:url";
const root = fileURLToPath(new URL("./static/", import.meta.url));
const port = Number(process.env.PORT || 5173),
  host = process.env.HOST || "127.0.0.1";
const types = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".png": "image/png",
  ".svg": "image/svg+xml",
};
createServer(async (req, res) => {
  res.setHeader(
    "Content-Security-Policy",
    "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self' https: http://localhost:* http://127.0.0.1:* http://[::1]:*; img-src 'self' data: https:; media-src 'self' https:; frame-src https://www.youtube-nocookie.com; object-src 'none'; base-uri 'self'; frame-ancestors 'none'",
  );
  res.setHeader("Cache-Control", "no-cache");
  res.setHeader("X-Content-Type-Options", "nosniff");
  res.setHeader("Referrer-Policy", "no-referrer");
  if (!["GET", "HEAD"].includes(req.method)) {
    res.writeHead(405);
    res.end();
    return;
  }
  try {
    const url = new URL(req.url, "http://localhost"),
      path = resolve(
        root,
        "." +
          decodeURIComponent(
            url.pathname === "/" ? "/index.html" : url.pathname,
          ),
      );
    if (
      !path.startsWith(root.endsWith(sep) ? root : root + sep) ||
      (await stat(path)).isDirectory()
    ) {
      res.writeHead(404);
      res.end();
      return;
    }
    const body = await readFile(path);
    res.setHeader(
      "Content-Type",
      types[extname(path)] || "application/octet-stream",
    );
    res.end(req.method === "HEAD" ? undefined : body);
  } catch {
    res.writeHead(404);
    res.end("Not found");
  }
}).listen(port, host, () => console.log(`Mu client: http://${host}:${port}`));
