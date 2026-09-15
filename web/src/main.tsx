import { lazy, Suspense, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { Layout, Status } from "./components/layout";
import { json, type State } from "./lib/api";
import "./index.css";
import { Conversation } from "./components/conversation";
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
const Admin = lazy(() =>
  import("./components/admin").then((m) => ({ default: m.Admin })),
);
const AppEditor = lazy(() =>
  import("./components/app-editor").then((m) => ({ default: m.AppEditor })),
);
const Agents = lazy(() =>
  import("./components/agents").then((m) => ({ default: m.Agents })),
);
const Application = lazy(() =>
  import("../../apps").then((m) => ({ default: m.Application })),
);
const Services = lazy(() =>
  import("./components/services").then((m) => ({ default: m.Services })),
);
import catalogue from "../../apps/catalog.json";
function App() {
  const [state, setState] = useState<State>(() => {
      const value = document.getElementById("client-state")?.textContent;
      return value ? JSON.parse(value) : undefined;
    }),
    [error, setError] = useState("");
  const path = location.pathname.replace(/\/$/, "") || "/",
    home =
      path === "/" || (path.startsWith("/agent/") && path !== "/agent/new");
  const application = path.startsWith("/blog/post")
    ? { id: "blog", name: "Blog" }
    : path === "/social/thread"
      ? { id: "social", name: "Social" }
      : path.startsWith("/files/") && path.endsWith("/edit")
        ? { id: "files", name: "Files" }
        : path === "/work"
          ? { id: "tasks", name: "Work" }
          : catalogue.find((a) => a.path === path);
  const title =
    application?.name ||
    (path === "/agents" || path === "/agent/new"
      ? "Agents"
      : path === "/apps/new" || path.startsWith("/apps/")
        ? "Apps"
        : path.startsWith("/services")
          ? "Services"
          : home
            ? "Home"
            : path === "/inbox/settings"
              ? "Inbox settings"
              : path.startsWith("/inbox")
                ? "Inbox"
                : path.startsWith("/admin")
                  ? (path.split("/")[2] || "Admin").replace(/^./, (c) =>
                      c.toUpperCase(),
                    )
                  : path === "/apps"
                    ? "Apps"
                    : path.endsWith("/billing") || path.endsWith("/usage")
                      ? "Billing"
                      : path.endsWith("/profile")
                        ? "Profile"
                        : "Account");
  useEffect(() => {
    if (!state)
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
        {path.startsWith("/admin") && path !== "/admin/users" ? (
          <Admin />
        ) : path === "/agents" || path === "/agent/new" ? (
          <Agents />
        ) : path === "/apps/new" || path.startsWith("/apps/") ? (
          <AppEditor />
        ) : application ? (
          <Application name={application.id} />
        ) : path.startsWith("/services") ? (
          <Services />
        ) : home ? (
          <Conversation
            state={state}
            onConversation={(conversation) =>
              setState((s) => ({ ...s, conversation }))
            }
          />
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
