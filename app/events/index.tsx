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

function BriefSchedule() {
  const { data, error, refresh } = useData<any>(() =>
    json("/events?view=brief"), [], "brief"
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
