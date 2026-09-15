import { useState } from "react";
import catalogue from "../app/catalog.json";
import { useData, json } from "../app/shared";
import { Input } from "../web/src/components/ui/input";
import { PageHeading, Status } from "../web/src/components/layout";
import type { Identity } from "../web/src/lib/api";
type SavedApp = {
  slug: string;
  name: string;
  official: boolean;
  can_edit: boolean;
};
export function Launcher({ account }: { account: Identity }) {
  const [query, setQuery] = useState("");
  const { data, error } = useData<SavedApp[]>(() => json("/apps"), [], "apps");
  const builtins = catalogue.filter((a) => !a.admin || account.admin);
  const entries = [
    ...builtins.map((a) => ({ ...a, key: a.id })),
    ...(data || [])
      .filter(
        (a) =>
          a.can_edit && !(a.official && builtins.some((b) => b.id === a.slug)),
      )
      .map((a) => ({
        id: a.slug,
        key: "saved:" + a.slug,
        name: a.name,
        path: "/apps/" + encodeURIComponent(a.slug),
        icon: "apps.svg",
      })),
  ];
  const first = ["assistant", "inbox", "work"];
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
    <div className="mx-auto w-full max-w-4xl">
      <PageHeading title="Home" />
      <Input
        type="search"
        aria-label="Find an app"
        placeholder="Find an app"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        className="mb-6"
      />
      <nav
        aria-label="Apps"
        className="grid grid-cols-3 gap-2 sm:grid-cols-4 lg:grid-cols-6"
      >
        {shown.map((a) => (
          <a
            key={a.key}
            href={a.path}
            className="flex min-w-0 flex-col items-center gap-2 rounded-xl px-2 py-4 text-center hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <img src={"/" + a.icon} alt="" className="size-8 object-contain" />
            <span className="max-w-full break-words text-sm font-medium">
              {a.name}
            </span>
          </a>
        ))}
      </nav>
      {!shown.length && (
        <p className="py-5 text-muted-foreground">No apps match.</p>
      )}
      {error && <Status error>Could not load your saved apps: {error}</Status>}
    </div>
  );
}
