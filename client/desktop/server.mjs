import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { resolve, sep, extname } from "node:path";
import { randomBytes, timingSafeEqual } from "node:crypto";
export function secureOrigin(value) {
  const u = new URL(value);
  if (
    u.username ||
    u.password ||
    u.hash ||
    u.search ||
    !["", "/"].includes(u.pathname) ||
    (u.protocol !== "https:" &&
      !(
        u.protocol === "http:" &&
        ["localhost", "127.0.0.1", "[::1]"].includes(u.hostname)
      ))
  )
    throw Error("Invalid server");
  return u.origin;
}
export async function serve(root, onCallback) {
  let pending = null;
  const launch = randomBytes(32).toString("hex");
  const server = createServer(async (req, res) => {
    res.setHeader("Cache-Control", "no-store");
    res.setHeader("Referrer-Policy", "no-referrer");
    res.setHeader("X-Content-Type-Options", "nosniff");
    res.setHeader(
      "Content-Security-Policy",
      "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self' https: http://localhost:* http://127.0.0.1:* http://[::1]:*; img-src 'self' data: https:; media-src 'self' https:; frame-src https://www.youtube-nocookie.com; object-src 'none'; base-uri 'none'; frame-ancestors 'none'",
    );
    if (req.headers.host !== new URL(origin).host) {
      res.writeHead(403);
      res.end();
      return;
    }
    if (req.method !== "GET" && req.method !== "HEAD") {
      res.writeHead(405);
      res.end();
      return;
    }
    let u;
    try {
      u = new URL(req.url, origin);
    } catch {
      res.writeHead(400);
      res.end();
      return;
    }
    if (u.pathname === "/oauth/callback") {
      const state = u.searchParams.get("state") || "";
      const valid =
        /^[A-Za-z0-9_-]+$/.test(state) &&
        pending &&
        Date.now() < pending.until &&
        state.length === pending.state.length &&
        timingSafeEqual(Buffer.from(state), Buffer.from(pending.state)) &&
        u.searchParams.getAll("state").length === 1 &&
        ((u.searchParams.getAll("code").length === 1 &&
          !u.searchParams.has("error")) ||
          (u.searchParams.getAll("error").length === 1 &&
            !u.searchParams.has("code")));
      if (!valid || req.method !== "GET") {
        res.writeHead(400);
        res.end(
          "Sign-in expired or could not be verified. Return to Mu and try again.",
        );
        return;
      }
      pending = null;
      const result = { state, callback: origin + "/oauth/callback" };
      if (u.searchParams.has("code")) result.code = u.searchParams.get("code");
      else result.error = u.searchParams.get("error");
      res.setHeader("Content-Type", "text/html; charset=utf-8");
      res.end(
        '<!doctype html><meta name="viewport" content="width=device-width"><title>Mu</title><h1>Return to Mu</h1><p>You can close this browser tab.</p>',
      );
      onCallback(result);
      return;
    }
    // App assets require an unguessable per-launch path, not a machine-wide origin.
    if (!u.pathname.startsWith("/" + launch + "/")) {
      res.writeHead(404);
      res.end();
      return;
    }
    try {
      const relative =
        decodeURIComponent(u.pathname.slice(launch.length + 2)) || "index.html";
      const path = resolve(root, relative);
      if (!path.startsWith(resolve(root) + sep)) throw Error("Outside app");
      const bytes = await readFile(path);
      const types = {
        ".html": "text/html; charset=utf-8",
        ".js": "text/javascript; charset=utf-8",
        ".css": "text/css; charset=utf-8",
        ".woff2": "font/woff2",
        ".png": "image/png",
      };
      res.setHeader(
        "Content-Type",
        types[extname(path)] || "application/octet-stream",
      );
      res.end(req.method === "HEAD" ? undefined : bytes);
    } catch {
      res.writeHead(404);
      res.end();
    }
  });
  await new Promise((ok, fail) => {
    server.once("error", fail);
    server.listen(0, "127.0.0.1", ok);
  });
  const origin = "http://127.0.0.1:" + server.address().port;
  return {
    url: origin + "/" + launch + "/",
    callback: origin + "/oauth/callback",
    close: () => server.close(),
    authorize(value) {
      const u = new URL(value);
      secureOrigin(u.origin);
      if (
        u.username ||
        u.password ||
        u.hash ||
        u.pathname != "/oauth/authorize" ||
        u.searchParams.get("redirect_uri") !== origin + "/oauth/callback" ||
        u.searchParams.get("response_type") !== "code" ||
        u.searchParams.get("code_challenge_method") !== "S256" ||
        !/^[A-Za-z0-9_-]{43,128}$/.test(u.searchParams.get("state") || "") ||
        !/^[A-Za-z0-9_-]{43}$/.test(u.searchParams.get("code_challenge") || "")
      )
        throw Error("Invalid sign-in request");
      pending = {
        state: u.searchParams.get("state"),
        until: Date.now() + 600000,
      };
      return u.href;
    },
  };
}
