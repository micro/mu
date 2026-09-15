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
export function SMS() {
  const [adding, setAdding] = useState(false);
  const { data, error, refresh } = useData<any>(() => json("/sms"));
  return (
    <>
      <PageHeading
        title="SMS"
        actions={<Button onClick={() => setAdding(!adding)}>New</Button>}
      />
      {adding && (
        <div className="mb-6">
          <Form
            fields={[
              {
                name: "to",
                label: "Phone number",
                type: "tel",
                required: true,
              },
              {
                name: "channel",
                label: "Channel",
                options: ["sms", "whatsapp"],
              },
              {
                name: "text",
                label: "Message",
                type: "textarea",
                required: true,
              },
            ]}
            label="Send"
            submit={async (v) => {
              await call("sms", "send", v);
              setAdding(false);
              await refresh();
            }}
          />
        </div>
      )}
      {error && <Status error>{error}</Status>}
      <Rows
        items={data?.messages}
        render={(m) => (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <span>
                {m.direction === "out" ? "To" : "From"} {m.number}
              </span>
              {m.status && <StateBadge value={m.status} />}
            </div>
            <Read>{m.text}</Read>
            <p className="text-sm text-muted-foreground">
              <When value={m.at} /> · {m.channel || "sms"}
            </p>
          </>
        )}
      />
    </>
  );
}
