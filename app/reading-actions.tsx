import { useState } from "react";
import { Button, mutate, Status } from "./shared";
export function ReadingActions({
  reference,
  href,
}: {
  reference: string;
  href: string;
}) {
  const [saved, setSaved] = useState(false),
    [busy, setBusy] = useState(false),
    [notice, setNotice] = useState("");
  return (
    <>
      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          disabled={busy || saved}
          onClick={async () => {
            setBusy(true);
            setNotice("");
            try {
              await mutate("/bookmarks", { action: "add", ref: reference });
              setSaved(true);
            } catch (e) {
              setNotice((e as Error).message);
            } finally {
              setBusy(false);
            }
          }}
        >
          {saved ? "Saved" : "Save"}
        </Button>
        <Button
          size="sm"
          onClick={async () => {
            try {
              const url = new URL(href, location.origin).href;
              if (navigator.share) await navigator.share({ url });
              else {
                await navigator.clipboard.writeText(url);
                setNotice("Link copied");
              }
            } catch (e) {
              if ((e as Error).name !== "AbortError")
                setNotice("Could not share this link.");
            }
          }}
        >
          Share
        </Button>
        <Button size="sm" asChild>
          <a href={`/?item=${encodeURIComponent(reference)}`}>Discuss</a>
        </Button>
      </div>
      {notice && <Status>{notice}</Status>}
    </>
  );
}
