import { useEffect, useState } from "react";
import { DropdownMenu } from "radix-ui";
import { MoreHorizontal } from "lucide-react";
import { Button } from "../../web/src/components/ui/button";
import { Input } from "../../web/src/components/ui/input";
import { Tabs, TabsList, TabsTrigger } from "../../web/src/components/ui/tabs";
import { PageHeading, Pager, Status } from "../../web/src/components/layout";
import { initialData, json, mutate } from "../../web/src/lib/api";
type User = {
  id: string;
  name: string;
  created: string;
  admin: boolean;
  agent: boolean;
  banned: boolean;
  approved: boolean;
  balance: number;
  self: boolean;
  verified: boolean;
};
type Users = { items: User[]; page: number; total: number; page_size: number };
export function AdminUsers() {
  const [data, setData] = useState<Users | undefined>(() =>
      initialData<Users>(),
    ),
    [tab, setTab] = useState(
      new URLSearchParams(location.search).get("tab") || "all",
    ),
    [page, setPage] = useState(1),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [grant, setGrant] = useState(""),
    [amount, setAmount] = useState("");
  async function load() {
    try {
      setData(
        await json<Users>(
          "/admin/users?" + new URLSearchParams({ tab, page: String(page) }),
        ),
      );
    } catch (e) {
      setError((e as Error).message);
    }
  }
  useEffect(() => {
    load();
  }, [tab, page]);
  async function action(id: string, action: string) {
    if (
      ["delete", "ban", "toggle_admin", "toggle_agent"].includes(action) &&
      !confirm(action.replaceAll("_", " ") + " @" + id + "?")
    )
      return;
    setBusy(true);
    setError("");
    try {
      await mutate("/admin/users", { action, user_id: id, tab, amount });
      setGrant("");
      setAmount("");
      await load();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <PageHeading
        title="Users"
        actions={
          <>
            <Button variant="ghost" asChild>
              <a href="/admin">Admin</a>
            </Button>
            <Button asChild variant="outline">
              <a href="/admin/invite">Invites</a>
            </Button>
          </>
        }
      />
      <Tabs
        value={tab}
        onValueChange={(v) => {
          setPage(1);
          setTab(v);
        }}
      >
        <TabsList>
          {[
            ["all", "All"],
            ["banned", "Banned"],
            ["new", "New (24h)"],
          ].map(([v, label]) => (
            <TabsTrigger key={v} value={v}>
              {label}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>
      <div className="my-3">
        <Status error={!!error}>
          {error || (!data ? "Loading users…" : data.total + " users")}
        </Status>
      </div>
      <div className="divide-y border-y">
        {data?.items.map((u) => (
          <div key={u.id} className="py-3">
            <div className="flex min-w-0 items-center gap-3">
              <div className="min-w-0 flex-1">
                <a
                  className="block truncate font-medium"
                  href={"/@" + encodeURIComponent(u.id)}
                >
                  @{u.id}
                </a>
                {u.name !== u.id && (
                  <p className="truncate text-sm text-muted-foreground">
                    {u.name}
                  </p>
                )}
                <div className="mt-1 flex flex-wrap gap-1">
                  {[
                    u.admin && "Admin",
                    u.agent && "Agent",
                    u.banned && "Banned",
                    u.verified && "Verified",
                    !u.approved && "Unapproved",
                  ]
                    .filter(Boolean)
                    .map((b) => (
                      <span
                        key={String(b)}
                        className="rounded border bg-muted px-1.5 text-xs"
                      >
                        {b}
                      </span>
                    ))}
                </div>
              </div>
              <div className="shrink-0 text-right text-sm">
                <p>{u.balance.toLocaleString()} credits</p>
                <time
                  className="hidden text-muted-foreground sm:block"
                  dateTime={u.created}
                >
                  {new Date(u.created).toLocaleDateString()}
                </time>
              </div>
              <DropdownMenu.Root>
                <DropdownMenu.Trigger asChild>
                  <Button
                    disabled={busy}
                    variant="ghost"
                    size="icon"
                    aria-label={"Actions for @" + u.id}
                  >
                    <MoreHorizontal />
                  </Button>
                </DropdownMenu.Trigger>
                <DropdownMenu.Portal>
                  <DropdownMenu.Content
                    align="end"
                    sideOffset={4}
                    className="z-50 min-w-44 rounded-lg border bg-background p-1 shadow-md"
                  >
                    {[
                      [u.banned ? "unban" : "ban", u.banned ? "Unban" : "Ban"],
                      ["toggle_admin", u.admin ? "Remove admin" : "Make admin"],
                      [
                        "toggle_agent",
                        u.agent ? "Mark as person" : "Mark as agent",
                      ],
                      ["approve", "Approve"],
                      ["delete", "Delete"],
                    ]
                      .filter(
                        ([key]) =>
                          !(
                            u.self &&
                            ["ban", "delete", "toggle_admin"].includes(key)
                          ) && !(u.approved && key === "approve"),
                      )
                      .map(([key, label]) => (
                        <DropdownMenu.Item
                          className="cursor-pointer rounded px-3 py-2 text-sm outline-none focus:bg-accent"
                          key={key}
                          onSelect={() => action(u.id, key)}
                        >
                          {label}
                        </DropdownMenu.Item>
                      ))}
                    <DropdownMenu.Item
                      className="cursor-pointer rounded px-3 py-2 text-sm outline-none focus:bg-accent"
                      onSelect={() => setGrant(u.id)}
                    >
                      Add credits
                    </DropdownMenu.Item>
                  </DropdownMenu.Content>
                </DropdownMenu.Portal>
              </DropdownMenu.Root>
            </div>
            {grant === u.id && (
              <form
                className="mt-3 flex flex-wrap gap-2"
                onSubmit={(e) => {
                  e.preventDefault();
                  action(u.id, "credit");
                }}
              >
                <Input
                  className="w-36"
                  type="number"
                  min="1"
                  aria-label="Credits to add"
                  required
                  value={amount}
                  onChange={(e) => setAmount(e.target.value)}
                />
                <Button disabled={busy}>Add credits</Button>
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => setGrant("")}
                >
                  Cancel
                </Button>
              </form>
            )}
          </div>
        ))}
      </div>
      {data && (
        <Pager
          page={data.page}
          total={data.total}
          size={data.page_size}
          onChange={setPage}
        />
      )}
    </>
  );
}
