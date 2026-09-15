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
