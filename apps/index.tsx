import catalogue from "./catalog.json";
import { Documents } from "./documents";
import { Tasks } from "./tasks";
import { Contacts, Events, Files } from "./organise";
import { Reading } from "./reading";
import { Utilities } from "./utilities";
import { Mail, SMS, Notifications, People, Chat } from "./communication";
import { MapApp } from "./map";
import { Images } from "./images";
import { WebSearch } from "./search";
export { catalogue };
export function Application({ name }: { name: string }) {
  if (name === "notes" || name === "docs")
    return <Documents notes={name === "notes"} />;
  if (name === "tasks") return <Tasks />;
  if (name === "contacts") return <Contacts />;
  if (name === "events") return <Events />;
  if (name === "images") return <Images />;
  if (name === "web") return <WebSearch />;
  if (name === "maps") return <MapApp />;
  if (name === "mail") return <Mail />;
  if (name === "sms") return <SMS />;
  if (name === "notify") return <Notifications />;
  if (name === "users") return <People />;
  if (name === "chat") return <Chat />;
  if (name === "files") return <Files />;
  if (
    [
      "news",
      "video",
      "blog",
      "social",
      "stream",
      "archive",
      "recall",
      "bookmarks",
    ].includes(name)
  )
    return <Reading name={name} />;
  return <Utilities name={name} />;
}
