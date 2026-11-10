import { runtimeOrigin, type Session } from "./protocol";
const sessionKey = "mu-web-session",
  pendingKey = "mu-web-signin";
interface Pending {
  server: string;
  client: string;
  redirect: string;
  verifier: string;
  state: string;
  created: number;
  query: string;
}
export const encode = (bytes: Uint8Array) =>
  btoa(String.fromCharCode(...bytes))
    .replaceAll("+", "-")
    .replaceAll("/", "_")
    .replaceAll("=", "");
export async function challenge(verifier: string) {
  return encode(
    new Uint8Array(
      await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier)),
    ),
  );
}
const random = () => encode(crypto.getRandomValues(new Uint8Array(32)));
async function request(url: string, body: string, type: string) {
  const response = await fetch(url, {
    method: "POST",
    credentials: "omit",
    cache: "no-store",
    signal: AbortSignal.timeout(20000),
    headers: { "Content-Type": type },
    body,
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok)
    throw Error(
      data.error_description ||
        data.error ||
        "Could not sign in to this runtime.",
    );
  return data;
}
export function loadSession(): Session | null {
  try {
    const value = JSON.parse(sessionStorage.getItem(sessionKey) || "null");
    if (
      value &&
      runtimeOrigin(value.server) === value.server &&
      typeof value.token === "string" &&
      value.token.length > 0 &&
      value.expires > Date.now()
    )
      return value;
  } catch {
    /* Invalid or expired sessions require sign-in. */
  }
  sessionStorage.removeItem(sessionKey);
  return null;
}
export function disconnect() {
  sessionStorage.removeItem(sessionKey);
  sessionStorage.removeItem(pendingKey);
}
export function defaultRuntime(): string {
  try {
    const saved = localStorage.getItem("mu-web-runtime");
    if (saved) return runtimeOrigin(saved);
  } catch {}
  return location.pathname.startsWith("/app/")
    ? location.origin
    : "https://micro.mu";
}
export async function signIn(value: string) {
  const server = runtimeOrigin(value),
    redirect = location.origin + location.pathname;
  const registered = await request(
    server + "/oauth/register",
    JSON.stringify({ client_name: "Mu Web", redirect_uris: [redirect] }),
    "application/json",
  );
  if (typeof registered.client_id !== "string")
    throw Error("The runtime did not register the client.");
  const pending: Pending = {
    server,
    client: registered.client_id,
    redirect,
    verifier: random(),
    state: random(),
    created: Date.now(),
    query: location.search,
  };
  sessionStorage.setItem(pendingKey, JSON.stringify(pending));
  const query = new URLSearchParams({
    response_type: "code",
    client_id: pending.client,
    redirect_uri: redirect,
    scope: "read write api:agent",
    state: pending.state,
    code_challenge_method: "S256",
    code_challenge: await challenge(pending.verifier),
  });
  location.assign(server + "/oauth/authorize?" + query);
}
export function validCallback(
  p: Pending | null,
  state: string | null,
  callback: string,
  now: number,
): p is Pending {
  return (
    !!p &&
    !!state &&
    p.state === state &&
    p.redirect === callback &&
    now >= p.created &&
    now - p.created < 600000 &&
    runtimeOrigin(p.server) === p.server
  );
}
export async function finishSignIn(): Promise<Session | null> {
  const query = new URLSearchParams(location.search);
  if (!query.has("code") && !query.has("error")) return loadSession();
  let pending: Pending | null = null;
  try {
    pending = JSON.parse(sessionStorage.getItem(pendingKey) || "null");
  } catch {}
  sessionStorage.removeItem(pendingKey);
  // Remove authorization codes before any other navigation or resource rendering.
  history.replaceState(null, "", location.pathname);
  if (
    !validCallback(
      pending,
      query.get("state"),
      location.origin + location.pathname,
      Date.now(),
    )
  )
    throw Error("Sign-in expired or could not be verified. Please try again.");
  if (query.has("error")) throw Error("Sign-in was not approved.");
  const result = await request(
    pending.server + "/oauth/token",
    new URLSearchParams({
      grant_type: "authorization_code",
      client_id: pending.client,
      redirect_uri: pending.redirect,
      code: query.get("code") || "",
      code_verifier: pending.verifier,
    }).toString(),
    "application/x-www-form-urlencoded",
  );
  if (
    typeof result.access_token !== "string" ||
    result.token_type !== "Bearer" ||
    !Number.isFinite(result.expires_in) ||
    result.expires_in <= 0
  )
    throw Error("Invalid sign-in response.");
  const session = {
    server: pending.server,
    token: result.access_token,
    expires: Date.now() + Math.min(result.expires_in, 86400) * 1000,
  };
  sessionStorage.setItem(sessionKey, JSON.stringify(session));
  localStorage.setItem("mu-web-runtime", session.server);
  history.replaceState(null, "", location.pathname + pending.query);
  return session;
}
