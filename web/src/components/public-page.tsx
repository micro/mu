import { Pricing } from "./pricing";
import { useEffect, useState } from "react";
import content from "../public-content.json";
import { initialData, json } from "../lib/api";
import { Button } from "./ui/button";
import { Status } from "./layout";

export const publicTitles: Record<string, string> = {
  "/about": "About Micro",
  "/contact": "Contact",
  "/pricing": "Pricing",
  "/privacy": "Privacy",
  "/status": "Status",
};
export function PublicPage({ path }: { path: string }) {
  const copy = content[path.slice(1) as keyof typeof content];
  const [data, setData] = useState<any>(() => initialData());
  const [error, setError] = useState("");
  useEffect(() => {
    if (!copy)
      json(path)
        .then(setData)
        .catch((e) => setError(e.message));
  }, [path]);
  return (
    <article className="space-y-8">
      <h1 className="text-3xl font-semibold tracking-tight">
        {publicTitles[path]}
      </h1>
      {copy ? (
        <>
          {copy.sections.map((section) => (
            <section key={section.title} className="space-y-3">
              <h2 className="text-lg font-medium">{section.title}</h2>
              {section.paragraphs.map((p) => (
                <p key={p} className="leading-7">
                  {p}
                </p>
              ))}
            </section>
          ))}
          <div className="flex flex-wrap gap-3">
            {(path === "/about"
              ? [
                  ["/", "Talk to Micro"],
                  ["/contact", "Contact"],
                  ["/pricing", "Pricing"],
                  ["https://github.com/micro/mu", "Source code"],
                ]
              : [
                  ["mailto:admin@micro.mu", "Contact the operator"],
                  ["/account/profile", "Profile"],
                  ["/token", "API credentials"],
                ]
            ).map(([href, label]) => (
              <Button key={href} asChild>
                <a href={href}>{label}</a>
              </Button>
            ))}
          </div>
        </>
      ) : error ? (
        <Status error>{error}</Status>
      ) : !data ? (
        <Status>Loading…</Status>
      ) : path === "/contact" ? (
        <>
          <section className="space-y-4">
            <h2 className="text-lg font-medium">Talk to Micro</h2>
            {data.channels.map((c: any) => (
              <div key={c.Label} className="space-y-1 border-b pb-4">
                <h3 className="font-medium">{c.Label}</h3>
                <p className="break-words">
                  {c.Href ? (
                    <a className="underline underline-offset-4" href={c.Href}>
                      {c.Address}
                    </a>
                  ) : (
                    c.Address
                  )}
                </p>
                <p className="text-sm text-muted-foreground">{c.Note}</p>
              </div>
            ))}
            {data.savable && (
              <Button asChild>
                <a href="/contact.vcf">Add to contacts</a>
              </Button>
            )}
            {data.verify_number && (
              <p>
                <a className="underline" href="/sms">
                  Verify your number
                </a>{" "}
                so Micro can recognise you by phone.
              </p>
            )}
          </section>
          <section id="support" className="space-y-3">
            <h2 className="text-lg font-medium">Contact the operator</h2>
            <p>
              For help with Micro, email{" "}
              <a className="underline" href="mailto:admin@micro.mu">
                admin@micro.mu
              </a>
              . Include the diagnostic report when reporting a failed task.
            </p>
          </section>
        </>
      ) : path === "/pricing" ? (
        <Pricing data={data} />
      ) : (
        <>
          <p className="capitalize">
            {data.state === "unknown"
              ? "Micro is reachable; no recent model data."
              : data.state}
          </p>
          <p className="text-sm text-muted-foreground">
            Updated {new Date(data.checked_at).toLocaleString()}
          </p>
          {data.capabilities.map((c: any) => (
            <section key={c.name} className="space-y-2 border-b pb-4">
              <h2 className="text-lg font-medium">{c.name}</h2>
              <p className="capitalize">
                {c.state === "unknown" ? "No recent data" : c.state}
              </p>
              <p>{c.details}</p>
            </section>
          ))}
          <section className="space-y-3">
            <h2 className="text-lg font-medium">About these checks</h2>
            <p>
              Model results cover recorded calls in the last 15 minutes, within
              the latest 500 external calls. They include retries and background
              work. Call time is not the total time to receive an answer.
            </p>
            <p>
              These checks do not verify end-to-end mail delivery or scheduled
              work. This page runs on the same server as Micro and may be
              unreachable during an outage.
            </p>
            <a className="underline" href="/contact#support">
              Report a problem
            </a>
          </section>
        </>
      )}
    </article>
  );
}
