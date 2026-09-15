import { useState } from "react";
import {
  useData,
  call,
  json,
  mutate,
  Button,
  Input,
  Status,
  Rows,
  When,
  Read,
  Action,
  Form,
  Search,
  Link,
} from "./shared";
import { PageHeading } from "../web/src/components/layout";
export function Contacts() {
  const [editing, setEditing] = useState<any>(),
    [q, setQ] = useState("");
  const { data, error, refresh } = useData<any>(() => call("contacts", "list"));
  return (
    <>
      <PageHeading
        title="Contacts"
        actions={<Button onClick={() => setEditing({})}>New</Button>}
      />
      <Search placeholder="Find a contact" onSearch={setQ} />
      <ContactImport refresh={refresh} />
      {editing && (
        <div className="mb-6">
          <Form
            key={editing.id || "new"}
            initial={editing}
            fields={[
              { name: "name", label: "Name", required: true },
              { name: "email", label: "Email", type: "email" },
              { name: "phone", label: "Phone", type: "tel" },
              { name: "note", label: "Notes", type: "textarea" },
            ]}
            submit={async (v) => {
              await call("contacts", "add", v);
              setEditing(undefined);
              await refresh();
            }}
          />
          <Button className="mt-2" onClick={() => setEditing(undefined)}>
            Cancel
          </Button>
        </div>
      )}
      {error && <Status error>{error}</Status>}
      <Rows
        items={data?.contacts?.filter((c: any) =>
          (c.name + " " + c.email + " " + c.phone)
            .toLowerCase()
            .includes(q.toLowerCase()),
        )}
        render={(c) => (
          <>
            <h2 className="font-medium">{c.name}</h2>
            <p className="break-words">
              {c.email} {c.phone}
            </p>
            {c.note && <Read>{c.note}</Read>}
            <div className="flex flex-wrap gap-2">
              <Button onClick={() => setEditing(c)}>Edit</Button>
              {c.email && (
                <Button asChild>
                  <a href={"/inbox/new?to=" + encodeURIComponent(c.email)}>
                    Write
                  </a>
                </Button>
              )}
              <Action
                danger
                run={async () => {
                  await call("contacts", "delete", { id: c.id });
                  await refresh();
                }}
              >
                Delete
              </Action>
            </div>
          </>
        )}
      />
    </>
  );
}
export function Events() {
  const [editing, setEditing] = useState<any>(
    new URLSearchParams(location.search).has("new") ? {} : undefined,
  );
  const { data, error, refresh } = useData<any>(() =>
    json(
      "/events" +
        (new URLSearchParams(location.search).get("id")
          ? "?id=" +
            encodeURIComponent(new URLSearchParams(location.search).get("id")!)
          : ""),
    ),
  );
  const id = new URLSearchParams(location.search).get("id");
  return (
    <>
      <PageHeading
        title="Events"
        actions={<Button onClick={() => setEditing({})}>New</Button>}
      />
      <BriefSchedule />
      {editing && (
        <div className="mb-6">
          <Form
            key={editing.id || "new"}
            initial={editing}
            fields={[
              { name: "title", label: "Title", required: true },
              {
                name: "when",
                label: "When",
                type: "datetime-local",
                required: true,
              },
              {
                name: "minutes",
                label: "Duration in minutes",
                type: "number",
                value: "30",
              },
              { name: "note", label: "Notes", type: "textarea" },
              {
                name: "repeat",
                label: "Repeat",
                options: ["once", "hourly", "daily", "weekly", "monthly"],
              },
              {
                name: "prompt",
                label: "Instruction for your agent",
                type: "textarea",
              },
            ]}
            submit={async (v) => {
              await call("events", editing.id ? "update" : "create", {
                ...v,
                id: editing.id,
                when: new Date(v.when).toISOString(),
                minutes: Number(v.minutes),
                repeat: v.repeat === "once" ? "" : v.repeat,
              });
              setEditing(undefined);
              await refresh();
            }}
          />
          <Button className="mt-2" onClick={() => setEditing(undefined)}>
            Cancel
          </Button>
        </div>
      )}
      {error && <Status error>{error}</Status>}
      <Rows
        items={data?.filter((e: any) => !id || e.id === id)}
        render={(e) => (
          <>
            <h2 className="font-medium">{e.title}</h2>
            <p className="text-muted-foreground">
              <When value={e.when} />
              {e.repeat && <> · {e.repeat}</>}
            </p>
            {e.note && <Read>{e.note}</Read>}
            {e.prompt && <Read>{e.prompt}</Read>}
            <div className="flex gap-2">
              <Button
                onClick={() => {
                  const d = new Date(e.when);
                  setEditing({
                    ...e,
                    when: new Date(d.getTime() - d.getTimezoneOffset() * 60000)
                      .toISOString()
                      .slice(0, 16),
                  });
                }}
              >
                Edit
              </Button>
              <Action
                danger
                run={async () => {
                  await call("events", "delete", { id: e.id });
                  await refresh();
                }}
              >
                Cancel event
              </Action>
            </div>
          </>
        )}
      />
    </>
  );
}
export function Files() {
  if (location.pathname.endsWith("/edit")) return <FileEditor />;
  return <FileList />;
}
function FileEditor() {
  const { data, error } = useData<any>(() => json(location.pathname));
  return (
    <>
      <PageHeading
        title="Edit file"
        actions={
          <Button asChild>
            <a href="/files">Files</a>
          </Button>
        }
      />
      {error && <Status error>{error}</Status>}
      {data && (
        <Form
          initial={{ content: data.content }}
          fields={[{ name: "content", label: "Contents", type: "textarea" }]}
          label="Save file"
          submit={async (v) => {
            await mutate(location.pathname, {
              ...v,
              checksum: data.file.checksum,
            });
            location.assign("/files");
          }}
        >
          <p>{data.file.name}</p>
        </Form>
      )}
    </>
  );
}
function FileList() {
  const [newFile, setNewFile] = useState(
    new URLSearchParams(location.search).has("new"),
  );
  const { data, error, refresh } = useData<any>(() => call("files", "list"));
  return (
    <>
      <PageHeading
        title="Files"
        actions={<Button onClick={() => setNewFile(!newFile)}>New</Button>}
      />
      {newFile && (
        <div className="mb-6">
          <Form
            fields={[
              {
                name: "name",
                label: "Filename",
                required: true,
                value: "untitled.txt",
              },
              {
                name: "content",
                label: "Contents",
                type: "textarea",
                required: false,
              },
            ]}
            submit={async (v) => {
              await mutate("/files?new=1", v);
              setNewFile(false);
              await refresh();
            }}
          />
        </div>
      )}
      <Upload refresh={refresh} />
      <SSHAccess />
      {error && <Status error>{error}</Status>}
      <Rows
        items={data?.files}
        render={(f) => (
          <>
            <h2 className="break-words font-medium">
              <Link url={f.url}>{f.name}</Link>
            </h2>
            <p className="text-sm text-muted-foreground">
              {new Intl.NumberFormat().format(f.size)} bytes ·{" "}
              {f.public ? "Public" : "Private"} · <When value={f.created} />
            </p>
            <div className="flex flex-wrap gap-2">
              <Button asChild>
                <a href={f.url} download>
                  Download
                </a>
              </Button>
              <Button asChild>
                <a href={`/files/${encodeURIComponent(f.id)}/edit`}>
                  Edit text
                </a>
              </Button>
              <Action
                run={async () => {
                  await call("files", "share", { id: f.id, public: !f.public });
                  await refresh();
                }}
              >
                {f.public ? "Make private" : "Share"}
              </Action>
              <Action
                danger
                run={async () => {
                  await call("files", "delete", { id: f.id });
                  await refresh();
                }}
              >
                Delete
              </Action>
            </div>
          </>
        )}
      />
    </>
  );
}
function Upload({ refresh }: { refresh: () => Promise<unknown> }) {
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  return (
    <form
      className="mb-6 space-y-3"
      onSubmit={async (e) => {
        e.preventDefault();
        const form = e.currentTarget;
        setBusy(true);
        setError("");
        try {
          const res = await fetch("/files", {
            method: "POST",
            headers: {
              Accept: "application/json",
              "X-CSRF-Token": decodeURIComponent(
                document.cookie.match(/(?:^|; )csrf_token=([^;]+)/)?.[1] || "",
              ),
            },
            body: new FormData(form),
          });
          const payload = await res.json();
          if (!res.ok) throw Error(payload.error || "Upload failed");
          form.reset();
          await refresh();
        } catch (e) {
          setError((e as Error).message);
        } finally {
          setBusy(false);
        }
      }}
    >
      <div className="flex flex-wrap gap-2">
        <Input
          className="min-w-0 flex-[1_1_16rem]"
          type="file"
          name="file"
          required
          aria-label="File to upload"
        />
        <Button disabled={busy}>{busy ? "Uploading…" : "Upload"}</Button>
      </div>
      {error && <Status error>{error}</Status>}
    </form>
  );
}

function ContactImport({ refresh }: { refresh: () => Promise<unknown> }) {
  const [message, setMessage] = useState("");
  return (
    <details className="mb-5">
      <summary>Import contacts</summary>
      <form
        className="mt-3 space-y-3"
        onSubmit={async (e) => {
          e.preventDefault();
          try {
            const r = await json<any>("/contacts/import", {
              method: "POST",
              body: new FormData(e.currentTarget),
            });
            setMessage(
              `Imported ${r.added}. Skipped ${r.skipped}. Failed ${r.failed}.`,
            );
            await refresh();
          } catch (e) {
            setMessage((e as Error).message);
          }
        }}
      >
        <Input
          name="file"
          type="file"
          accept=".csv,text/csv"
          required
          aria-label="Contacts CSV"
        />
        <p className="text-sm text-muted-foreground">
          Google, Outlook, or CSV with Name, Email, Phone and Note columns. Up
          to 500 contacts.
        </p>
        <Button>Import CSV</Button>
        {message && <Status>{message}</Status>}
      </form>
    </details>
  );
}
export function SSHAccess() {
  const { data, error, refresh } = useData<any>(() => json("/client/ssh"));
  return (
    <details className="mb-5">
      <summary>Connect with SSH or SFTP</summary>
      {error && <Status error>{error}</Status>}
      {data &&
        (data.enabled ? (
          <div className="mt-3 space-y-4">
            <p>Your public key identifies your account.</p>
            <pre className="overflow-auto text-sm">
              {data.ssh + "\n" + data.sftp}
            </pre>
            <Rows
              items={data.keys}
              render={(k) => (
                <>
                  <p className="break-all">
                    {k.name} · {k.fingerprint}
                  </p>
                  <Action
                    danger
                    run={async () => {
                      await mutate("/client/ssh", { removekey: k.fingerprint });
                      await refresh();
                    }}
                  >
                    Remove key
                  </Action>
                </>
              )}
            />
            <Form
              fields={[
                {
                  name: "sshkey",
                  label: "Public key",
                  type: "textarea",
                  required: true,
                },
                { name: "keyname", label: "Name" },
              ]}
              label="Add key"
              submit={async (v) => {
                await mutate("/client/ssh", v);
                await refresh();
              }}
            />
          </div>
        ) : (
          <p className="mt-3 text-muted-foreground">
            SSH is not enabled on this instance.
          </p>
        ))}
    </details>
  );
}
function BriefSchedule() {
  const { data, error, refresh } = useData<any>(() =>
    json("/events?view=brief"),
  );
  const b = data?.brief;
  const zone = b?.zone || Intl.DateTimeFormat().resolvedOptions().timeZone;
  let clock = "06:00";
  if (b?.when)
    try {
      clock = new Date(b.when).toLocaleTimeString("en-GB", {
        timeZone: zone,
        hour: "2-digit",
        minute: "2-digit",
      });
    } catch {}
  return (
    <details className="mb-6">
      <summary>Daily brief</summary>
      {error && <Status error>{error}</Status>}
      {data && (
        <div className="mt-4">
          <Form
            key={JSON.stringify(b)}
            initial={{
              clock,
              zone,
              repeat: b?.repeat || "daily",
              period: b?.prompt?.includes("today") ? "morning" : "evening",
              state: b?.paused ? "paused" : "active",
            }}
            fields={[
              {
                name: "period",
                label: "Brief",
                options: ["morning", "evening"],
              },
              { name: "clock", label: "Time", type: "time", required: true },
              { name: "zone", label: "Timezone", required: true },
              {
                name: "repeat",
                label: "Frequency",
                options: ["daily", "weekdays"],
              },
              { name: "state", label: "Status", options: ["active", "paused"] },
            ]}
            submit={async (v) => {
              await mutate("/events", { action: "brief-schedule", ...v });
              await refresh();
            }}
          />
        </div>
      )}
    </details>
  );
}
