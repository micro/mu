import { Launcher } from "../../home/client";
import { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { Layout, Status } from "./components/layout";
import { json, type State } from "./lib/api";
import "./index.css";
import { Conversation } from "../../app/assistant";
import { InboxPage } from "./components/inbox";
import { InboxCompose } from "./components/inbox-compose";
import { InboxSettings } from "./components/inbox-settings";
import { AdminUsers } from "../../app/admin/users";
import { AccountPage } from "./components/account";
import { AppsPage } from "../../app/apps";
import { Admin } from "../../app/admin";
import { AppEditor } from "../../app/apps/editor";
import { Agents } from "./components/agents";
import { Application } from "../../app";
import { Workspace } from "../../work/client";
import { Services } from "./components/services";
import { PublicPage, publicTitles } from "./components/public-page";
import catalogue from "../../app/catalog.json";
function App() {
  const [state, setState] = useState<State>(() => {
      const value = document.getElementById("client-state")?.textContent;
      return value ? JSON.parse(value) : undefined;
    }),
    [error, setError] = useState("");
  const path = location.pathname.replace(/\/$/, "") || "/",
    home =
      path === "/assistant" ||
      path === "/" ||
      (path.startsWith("/agent/") && path !== "/agent/new");
  const application = path.startsWith("/blog/post")
    ? { id: "blog", name: "Blog" }
    : path === "/social/thread"
      ? { id: "social", name: "Social" }
      : path.startsWith("/files/") && path.endsWith("/edit")
        ? { id: "files", name: "Files" }
        : path === "/work"
          ? { id: "tasks", name: "Work" }
          : catalogue.find(
              (a) =>
                a.path === path &&
                !["inbox", "work", "assistant"].includes(a.id),
            );
  const publicPage = !!publicTitles[path];
  const [legacyConversation] = useState(
    () =>
      path === "/" &&
      (history.state?.microConversation?.scope ===
        `${state?.account?.id || "guest"}:/` ||
        ["session", "continue", "new", "item", "bookmark", "saved"].some(
          (key) => new URLSearchParams(location.search).has(key),
        ) ||
        (sessionStorage.getItem("micro-guest-conversation") &&
          sessionStorage.getItem("micro-guest-conversation") !== "[]")),
  );
  const launcher = path === "/" && !!state?.account && !legacyConversation;
  const title =
    publicTitles[path] ||
    (path === "/assistant" ? "Assistant" : undefined) ||
    application?.name ||
    (path === "/agents" || path === "/agent/new"
      ? "Agents"
      : path === "/apps/new" || path.startsWith("/apps/")
        ? "Apps"
        : path.startsWith("/services") || path.startsWith("/service/")
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
      account={publicPage ? null : state.account}
      publicPage={publicPage}
      title={
        !launcher && state.conversation?.agent
          ? state.conversation.agent_name || title
          : title
      }
      conversation={
        (home && !launcher) ||
        (path === "/chat" && new URLSearchParams(location.search).has("id"))
      }
    >
      <>
        {publicPage ? (
          <PublicPage path={path} />
        ) : path.startsWith("/admin") && path !== "/admin/users" ? (
          <Admin />
        ) : path === "/agents" || path === "/agent/new" ? (
          <Agents />
        ) : path === "/apps/new" || path.startsWith("/apps/") ? (
          <AppEditor />
        ) : path === "/work" ? (
          <Workspace />
        ) : launcher ? (
          <Launcher account={state.account!} />
        ) : home ? (
          <Conversation
            state={state}
            onConversation={(conversation) =>
              setState((s) => ({ ...s, conversation }))
            }
          />
        ) : application ? (
          <Application name={application.id} />
        ) : path.startsWith("/services") || path.startsWith("/service/") ? (
          <Services />
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
      </>
    </Layout>
  );
}
createRoot(document.getElementById("root")!).render(<App />);
