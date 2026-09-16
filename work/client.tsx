import { useState } from "react";
import { PageHeading } from "../web/src/components/layout";
import { AppsPage } from "../app/apps";
import { AppBuilder } from "../app/apps/builder";
import { Tasks } from "../app/tasks";
import { Tabs, TabsList, TabsTrigger } from "../web/src/components/ui/tabs";

export function Workspace() {
  const detail = new URLSearchParams(location.search).has("id");
  const [tab, setTab] = useState(detail ? "tasks" : "build");
  return (
    <div className="w-full min-w-0 space-y-5">
      <PageHeading title="Work" />
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList aria-label="Work">
          <TabsTrigger value="build">Build</TabsTrigger>
          <TabsTrigger value="apps">Your apps</TabsTrigger>
          <TabsTrigger value="tasks">Tasks</TabsTrigger>
        </TabsList>
      </Tabs>
      <div hidden={tab !== "build"}>
        <AppBuilder />
      </div>
      {tab === "apps" && <AppsPage workspace />}
      {tab === "tasks" && <Tasks workspace />}
    </div>
  );
}
