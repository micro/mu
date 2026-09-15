import { useEffect, useRef, useState } from "react";
import {
  useData,
  json,
  mutate,
  call,
  Button,
  Status,
  Form,
  Search,
  Rows,
  When,
  Read,
  Action,
  StateBadge,
  Link,
  Textarea,
} from "../shared";
import { PageHeading, Pager } from "../../web/src/components/layout";
import { Email } from "../../web/src/components/email";
export function Notifications() {
  const [adding, setAdding] = useState(false);
  const { data, error, refresh } = useData<any>(() => json("/notify"));
  return (
    <>
      <PageHeading
        title="Notifications"
        actions={<Button onClick={() => setAdding(!adding)}>New</Button>}
      />
      <PushSettings publicKey={data?.public_key} refresh={refresh} />
      <p className="mb-4 text-muted-foreground">
        {data?.reachable
          ? "Notifications are enabled on your devices."
          : "Enable notifications on this device to receive updates."}{" "}
        <Link url="/account">Settings</Link>
      </p>
      {adding && (
        <div className="mb-6">
          <Form
            fields={[
              { name: "title", label: "Title", required: true },
              { name: "body", label: "Message", type: "textarea" },
            ]}
            label="Send to my devices"
            submit={async (v) => {
              await call("notify", "send", v);
              setAdding(false);
              await refresh();
            }}
          />
        </div>
      )}
      {error && <Status error>{error}</Status>}
      <Rows
        items={data?.sent}
        render={(n) => (
          <>
            <h2 className="font-medium">
              <Link url={n.url}>{n.title}</Link>
            </h2>
            <Read>{n.body || ""}</Read>
            <p className="text-sm text-muted-foreground">
              <When value={n.at || n.created} />
            </p>
          </>
        )}
      />
      <details className="mt-5">
        <summary>Devices</summary>
        <Rows
          items={data?.devices || []}
          render={(d) => (
            <p>
              {d.label} · {d.last || d.added}
            </p>
          )}
        />
      </details>
    </>
  );
}

function PushSettings({
  publicKey,
  refresh,
}: {
  publicKey?: string;
  refresh: () => Promise<unknown>;
}) {
  const [sub, setSub] = useState<PushSubscription | null>(null),
    [status, setStatus] = useState("");
  const supported = "serviceWorker" in navigator && "PushManager" in window;
  useEffect(() => {
    if (supported)
      navigator.serviceWorker.ready
        .then((r) => r.pushManager.getSubscription())
        .then(setSub)
        .catch((e) => setStatus(e.message));
  }, []);
  async function post(action: string, body: any) {
    const r = await json<any>("/notify/" + action, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (r.error || r.ok === false)
      throw Error(r.error || "Notification request failed");
    return r;
  }
  return (
    <div className="mb-5 space-y-3">
      {supported ? (
        <div className="flex flex-wrap gap-2">
          <Action
            run={async () => {
              if (sub) {
                await post("unsubscribe", { endpoint: sub.endpoint });
                await sub.unsubscribe();
                setSub(null);
              } else {
                if (!publicKey)
                  throw Error(
                    "Push notifications are not configured on this instance",
                  );
                if ((await Notification.requestPermission()) !== "granted")
                  throw Error(
                    "Allow notifications in your browser settings, then try again.",
                  );
                const r = await navigator.serviceWorker.ready;
                const key = Uint8Array.from(
                  atob(publicKey.replace(/-/g, "+").replace(/_/g, "/")),
                  (c) => c.charCodeAt(0),
                );
                const subscription = await r.pushManager.subscribe({
                  userVisibleOnly: true,
                  applicationServerKey: key,
                });
                await post("subscribe", {
                  ...subscription.toJSON(),
                  label: navigator.userAgent,
                });
                setSub(subscription);
              }
              await refresh();
            }}
          >
            {sub ? "Turn off on this device" : "Enable on this device"}
          </Action>
          {sub && (
            <Action
              run={async () => {
                await post("test", { endpoint: sub.endpoint });
                setStatus("Test sent to this device.");
              }}
            >
              Test notification
            </Action>
          )}
        </div>
      ) : (
        <p className="text-muted-foreground">
          This browser does not support push notifications. On iPhone, install
          the app on your Home Screen first.
        </p>
      )}
      {status && <Status>{status}</Status>}
    </div>
  );
}
