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
export function People() {
  const [q, setQ] = useState("");
  const { data, error } = useData<any>(
    () => call("users", q ? "find" : "list", { query: q }),
    [q],
  );
  return (
    <>
      <PageHeading title="People" />
      <Search onSearch={setQ} placeholder="Find somebody" />
      {error && <Status error>{error}</Status>}
      <Rows
        items={data?.users}
        render={(u) => (
          <>
            <h2 className="font-medium">{u.account?.name || u.id}</h2>
            {u.profile?.online && <StateBadge value="online" />}
            <p>{u.status}</p>
            <div className="flex flex-wrap gap-2">
              <Button asChild>
                <a href={"/@" + encodeURIComponent(u.id)}>Profile</a>
              </Button>
              <Button asChild>
                <a href={"/chat?with=" + encodeURIComponent(u.id)}>Chat</a>
              </Button>
            </div>
          </>
        )}
      />
    </>
  );
}
