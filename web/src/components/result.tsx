import { useState } from "react";
import { Button } from "./ui/button";
import { mutate, safeURL, type Result } from "../lib/api";
function RouteMap({ shape }: { shape: NonNullable<Result["shape"]> }) {
  const valid =
    shape.length > 1 &&
    shape.length <= 20000 &&
    shape.every(
      (p) =>
        Number.isFinite(p.Lat) &&
        Number.isFinite(p.Lon) &&
        Math.abs(p.Lat) <= 90 &&
        Math.abs(p.Lon) <= 180,
    );
  if (!valid) return null;
  const points = shape.map((p) => {
    const lat = (Math.max(-85, Math.min(85, p.Lat)) * Math.PI) / 180;
    return [
      (p.Lon + 180) / 360,
      (1 - Math.log(Math.tan(lat) + 1 / Math.cos(lat)) / Math.PI) / 2,
    ];
  });
  const xs = points.map((p) => p[0]),
    ys = points.map((p) => p[1]),
    minX = Math.min(...xs),
    maxX = Math.max(...xs),
    minY = Math.min(...ys),
    maxY = Math.max(...ys);
  let z = 1;
  while (
    z < 18 &&
    (maxX - minX) * 2 ** (z + 1) * 256 < 592 &&
    (maxY - minY) * 2 ** (z + 1) * 256 < 272
  )
    z++;
  const size = 2 ** z * 256,
    left = ((minX + maxX) * size) / 2 - 320,
    top = ((minY + maxY) * size) / 2 - 160,
    tiles = [];
  for (let x = Math.floor(left / 256); x * 256 < left + 640; x++)
    for (let y = Math.floor(top / 256); y * 256 < top + 320; y++)
      if (x >= 0 && y >= 0 && x < 2 ** z && y < 2 ** z)
        tiles.push(
          <image
            key={`${x}-${y}`}
            href={`/maps/world/${z}/${x}/${y}.png`}
            x={x * 256 - left}
            y={y * 256 - top}
            width="256"
            height="256"
          />,
        );
  return (
    <figure>
      <svg
        viewBox="0 0 640 320"
        role="img"
        aria-label="Route map"
        className="w-full rounded-md"
      >
        {tiles}
        <polyline
          points={points
            .map((p) => `${p[0] * size - left},${p[1] * size - top}`)
            .join(" ")}
          fill="none"
          stroke="#1655a0"
          strokeWidth="4"
        />
      </svg>
      <figcaption className="text-xs text-muted-foreground">
        © <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>
      </figcaption>
    </figure>
  );
}
export function ResultView({
  item,
  signedIn,
}: {
  item: Result;
  signedIn: boolean;
}) {
  const [status, setStatus] = useState(""),
    url = safeURL(item.url);
  async function save() {
    setStatus("Saving…");
    try {
      await mutate("/bookmarks", {
        action: "add",
        url: url!,
        title: item.title || url!,
      });
      setStatus("Saved");
    } catch {
      setStatus("Could not save. Try again.");
    }
  }
  return (
    <section className="my-4 space-y-3 rounded-lg border p-4">
      {item.kind === "video" && /^[\w-]{1,64}$/.test(item.id || "") && (
        <iframe
          className="aspect-video w-full rounded-md"
          src={`https://www.youtube.com/embed/${item.id}?playsinline=1`}
          title={item.title || "Video"}
          loading="lazy"
          allow="autoplay; encrypted-media; picture-in-picture"
          allowFullScreen
        />
      )}
      {item.kind === "route" && item.shape && <RouteMap shape={item.shape} />}
      <h3 className="font-medium">{item.title}</h3>
      {item.summary && <p>{item.summary}</p>}
      {!!item.steps?.length && (
        <details>
          <summary className="cursor-pointer">Directions</summary>
          <ol className="list-decimal space-y-2 pl-5 pt-3">
            {item.steps.map((step, i) => (
              <li key={i}>{step}</li>
            ))}
          </ol>
        </details>
      )}
      {url && (
        <div className="flex items-center gap-3">
          <Button asChild variant="outline">
            <a href={url} target="_blank" rel="noopener noreferrer">
              Open
            </a>
          </Button>
          {signedIn ? (
            <Button
              variant="outline"
              onClick={save}
              disabled={status === "Saved" || status === "Saving…"}
            >
              Save
            </Button>
          ) : (
            <Button asChild variant="outline">
              <a href="/login?redirect=%2F">Log in to save</a>
            </Button>
          )}
          <span role="status" className="text-sm">
            {status}
          </span>
        </div>
      )}
    </section>
  );
}
