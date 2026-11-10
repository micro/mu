import Markdown from "react-markdown";
import type { Content, Message } from "./protocol";
export function safeURL(value: string, base: string): string {
  try {
    const u = new URL(value, base);
    return ["https:", "http:"].includes(u.protocol) &&
      !u.username &&
      !u.password
      ? u.href
      : "";
  } catch {
    return "";
  }
}
function Result({ item, server }: { item: Content; server: string }) {
  const resource = item.source || item.resource;
  if (resource)
    return (
      <div className="client-result">
        <a
          href={safeURL(resource.url, server)}
          target="_blank"
          rel="noopener noreferrer"
        >
          {resource.title || item.fallback}
        </a>
        {resource.summary && <p>{resource.summary}</p>}
      </div>
    );
  if (
    item.video?.provider === "youtube" &&
    /^[A-Za-z0-9_-]{11}$/.test(item.video.id)
  )
    return (
      <div className="client-result">
        <iframe
          src={"https://www.youtube-nocookie.com/embed/" + item.video.id}
          title={item.video.title || "Video"}
          allow="encrypted-media; picture-in-picture; fullscreen"
          referrerPolicy="strict-origin-when-cross-origin"
          loading="lazy"
        />
      </div>
    );
  if (item.audio && safeURL(item.audio.url, server))
    return (
      <div className="client-result">
        <p>{item.audio.title}</p>
        <audio controls preload="none" src={safeURL(item.audio.url, server)} />
      </div>
    );
  if (item.image && safeURL(item.image.url, server))
    return (
      <div className="client-result">
        <img
          src={safeURL(item.image.url, server)}
          alt={item.image.description || item.fallback}
          loading="lazy"
          referrerPolicy="no-referrer"
        />
      </div>
    );
  if (item.route?.steps?.length)
    return (
      <div className="client-result">
        <p>{item.fallback}</p>
        <ol>
          {item.route.steps.map((step, i) => (
            <li key={i}>{step}</li>
          ))}
        </ol>
      </div>
    );
  return (
    <p className="client-result">
      {item.fallback || "This result is not supported by this client."}
    </p>
  );
}
export function Turn({
  message,
  name,
  server,
}: {
  message: Message;
  name: string;
  server: string;
}) {
  const person = message.role === "ROLE_PERSON";
  return (
    <section className="turn">
      <div className={person ? "request" : "answer"}>
        <strong className="metadata-row">{person ? "You" : name}</strong>
        <div className="message-body">
          <Markdown
            skipHtml
            urlTransform={(url) => safeURL(url, server)}
            components={{
              a: (props) => (
                <a {...props} target="_blank" rel="noopener noreferrer" />
              ),
            }}
          >
            {message.text}
          </Markdown>
          {message.content?.map((item, i) => (
            <Result item={item} server={server} key={i} />
          ))}
        </div>
      </div>
    </section>
  );
}
