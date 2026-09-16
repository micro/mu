import { useState } from "react";
import catalogue from "../app/catalog.json";
import { Input } from "../web/src/components/ui/input";
import { PageHeading } from "../web/src/components/layout";
import type { Identity } from "../web/src/lib/api";
export function Launcher({ account }: { account: Identity }) {
  const [query, setQuery] = useState("");
  const entries = catalogue
    .filter((a) => (!a.admin || account.admin) && !["inbox", "work", "admin"].includes(a.id))
    .map((a) => ({ ...a, key: a.id }));
  const first = ["assistant"];
  entries.sort((a, b) => {
    const ai = first.indexOf(a.id),
      bi = first.indexOf(b.id);
    return (
      (ai < 0 ? 99 : ai) - (bi < 0 ? 99 : bi) || a.name.localeCompare(b.name)
    );
  });
  const shown = entries.filter((a) =>
    a.name.toLowerCase().includes(query.toLowerCase()),
  );
  return (
    <div className="w-full min-w-0">
      <PageHeading title="Home" />
      <Input
        type="search"
        aria-label="Find an app"
        placeholder="Find an app"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        className="mb-6"
      />
      <nav aria-label="Apps" className="space-y-5">
        {[
          ["Everyday", ["assistant", "apps", "mail", "chat", "sms", "contacts", "events", "notify", "users"]],
          ["Read and explore", ["news", "blog", "video", "images", "web", "social", "stream", "markets"]],
          ["Places and travel", ["places", "maps", "routes", "transit", "flights", "weather", "hazards", "food", "prayer"]],
          ["Tools", ["docs", "notes", "files", "tasks", "bookmarks", "archive", "recall", "browser", "shell", "text", "wallet"]],
        ].map(([label, ids]) => {
          const apps = shown.filter(a => (ids as string[]).includes(a.id));
          return apps.length ? <section key={label as string} className="border-t pt-4">
            <h2 className="mb-2 text-sm font-medium text-muted-foreground">{label}</h2>
            <div className="grid grid-cols-3 gap-2 sm:grid-cols-4 lg:grid-cols-6">
              {apps.map(a => <a key={a.id} href={a.path} className="flex min-w-0 flex-col items-center gap-2 rounded-lg px-2 py-3 text-center hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                <img src={"/" + a.icon} alt="" className="size-8 object-contain" />
                <span className="max-w-full break-words text-sm font-medium">{a.name}</span>
              </a>)}
            </div>
          </section> : null;
        })}
      </nav>
      {!shown.length && (
        <p className="py-5 text-muted-foreground">No apps match.</p>
      )}
    </div>
  );
}
