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
} from "../shared";
import { PageHeading } from "../../web/src/components/layout";
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


export function SSHAccess() {
  const { data, error, refresh } = useData<any>(() => json("/client/ssh"), [], "ssh");
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
