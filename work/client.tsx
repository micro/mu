import { PageHeading } from "../web/src/components/layout";
import { AppsPage } from "../app/apps";
import { Tasks } from "../app/tasks";

export function Workspace() {
  const detail = new URLSearchParams(location.search).has("id");
  return <div className="space-y-8">
    <header>
      <PageHeading title="Work" />
      <p className="text-muted-foreground">A space to build. Create apps, delegate tasks and work with the results.</p>
    </header>
    {!detail && <section><AppsPage workspace /></section>}
    <section><Tasks workspace /></section>
  </div>;
}
