import { useState } from "react";
// MIME content keeps its existing decoder and sanitization. The document sandbox
// blocks scripts and forms and keeps sender styles out of the application.
export function Email({ html }: { html: string }) {
  const [height, setHeight] = useState(300);
  const css =
    document.querySelector<HTMLLinkElement>("link[rel=stylesheet]")?.href || "";
  return (
    <iframe
      title="Email message"
      sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox"
      referrerPolicy="no-referrer"
      height={height}
      className="max-h-[65dvh] w-full rounded-md border"
      onLoad={(e) => {
        const doc = e.currentTarget.contentDocument;
        if (doc) {
          setHeight(Math.min(700, doc.documentElement.scrollHeight + 4));
          doc.querySelectorAll("a").forEach((a) => {
            a.target = "_blank";
            a.rel = "noopener noreferrer";
          });
        }
      }}
      srcDoc={
        '<!doctype html><html><head><meta name="viewport" content="width=device-width"><meta http-equiv="Content-Security-Policy" content="default-src \'none\'; img-src data: https:; style-src ' +
        location.origin +
        " 'unsafe-inline'; base-uri 'none'; form-action 'none'\"><link rel=\"stylesheet\" href=\"" +
        css +
        '"></head><body class="email-document">' +
        html +
        "</body></html>"
      }
    />
  );
}
