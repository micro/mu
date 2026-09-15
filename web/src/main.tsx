import { lazy, Suspense, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { Layout, Status } from "./components/layout";
import { json, type State } from "./lib/api";
import "./index.css";
const Conversation = lazy(() =>
  import("./components/conversation").then((m) => ({
    default: m.Conversation,
  })),
);
const InboxPage = lazy(() =>
  import("./components/inbox").then((m) => ({ default: m.InboxPage })),
);
const InboxCompose = lazy(() =>
  import("./components/inbox-compose").then((m) => ({
    default: m.InboxCompose,
  })),
);
const InboxSettings = lazy(() =>
  import("./components/inbox-settings").then((m) => ({
    default: m.InboxSettings,
  })),
);
const AdminUsers = lazy(() =>
  import("./components/admin-users").then((m) => ({ default: m.AdminUsers })),
);
const AccountPage = lazy(() =>
  import("./components/account").then((m) => ({ default: m.AccountPage })),
);
const AppsPage = lazy(() =>
  import("./components/apps").then((m) => ({ default: m.AppsPage })),
);
function App() {
  const [state, setState] = useState<State>(),
    [error, setError] = useState("");
  const path = location.pathname.replace(/\/$/, "") || "/",
    home = path === "/";
  const title = home
    ? "Home"
    : path === "/inbox/settings"
      ? "Inbox settings"
      : path.startsWith("/inbox")
        ? "Inbox"
        : path === "/admin/users"
          ? "Users"
          : path === "/apps"
            ? "Apps"
            : path.endsWith("/billing") || path.endsWith("/usage")
              ? "Billing"
              : path.endsWith("/profile")
                ? "Profile"
                : "Account";
  useEffect(() => {
    json<State>(home ? "/" + location.search : "/client/state")
      .then((s) => {
        setState(s);
        if (home && location.search && !s.conversation?.attachment)
          history.replaceState(null, "", "/");
      })
      .catch((e) => setError(e.message));
    document.title = title + " | Micro";
    if ("serviceWorker" in navigator)
      navigator.serviceWorker
        .register("/mu.js", { scope: "/", updateViaCache: "none" })
        .then((r) => r.update())
        .catch(() => {});
  }, []);
  if (!state)
    return (
      <div className="p-6">
        <Status error={!!error}>{error || "Loading…"}</Status>
        {error && (
          <a href="/" className="mt-3 inline-block underline">
            Home
          </a>
        )}
      </div>
    );
  return (
    <Layout
      account={state.account}
      title={
        state.conversation?.agent
          ? state.conversation.agent_name || title
          : title
      }
      conversation={home}
    >
      <Suspense fallback={<Status>Loading…</Status>}>
        {home ? (
          <Conversation state={state} />
        ) : path === "/inbox/settings" ? (
          <InboxSettings />
        ) : path === "/inbox/new" ? (
          <InboxCompose />
        ) : path.startsWith("/inbox") ? (
          <InboxPage />
        ) : path === "/admin/users" ? (
          <AdminUsers />
        ) : path === "/apps" ? (
          <AppsPage />
        ) : (
          <AccountPage />
        )}
      </Suspense>
    </Layout>
  );
}
createRoot(document.getElementById("root")!).render(<App />);
