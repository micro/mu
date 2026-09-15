import { useEffect, useRef, useState } from "react";
import { call, json, Button, Status, Search, NativeSelect } from "./shared";
import { PageHeading } from "../web/src/components/layout";
export function MapApp() {
  const [center, setCenter] = useState({ lat: 20, lon: 0 }),
    [zoom, setZoom] = useState(2),
    [style, setStyle] = useState(
      new URLSearchParams(location.search).get("style") || "world",
    ),
    [error, setError] = useState("");
  const box = useRef<HTMLDivElement>(null),
    [size, setSize] = useState({ width: 600, height: 480 });
  const drag = useRef<
    { x: number; y: number; px: number; py: number } | undefined
  >(undefined);
  useEffect(() => {
    if (!box.current) return;
    const observer = new ResizeObserver(([entry]) =>
      setSize({
        width: entry.contentRect.width,
        height: entry.contentRect.height,
      }),
    );
    observer.observe(box.current);
    return () => observer.disconnect();
  }, []);
  const scale = 2 ** zoom * 256,
    lat = Math.max(-85, Math.min(85, center.lat)),
    cx = ((center.lon + 180) / 360) * scale,
    cy =
      ((1 - Math.asinh(Math.tan((lat * Math.PI) / 180)) / Math.PI) / 2) * scale;
  function move(x: number, y: number) {
    setCenter({
      lon: (((((x / scale) * 360) % 360) + 360) % 360) - 180,
      lat: Math.max(
        -85,
        Math.min(
          85,
          (Math.atan(Math.sinh(Math.PI * (1 - (2 * y) / scale))) * 180) /
            Math.PI,
        ),
      ),
    });
  }
  const tiles = [];
  for (
    let y = Math.floor((cy - size.height / 2) / 256);
    y <= Math.floor((cy + size.height / 2) / 256);
    y++
  )
    for (
      let x = Math.floor((cx - size.width / 2) / 256);
      x <= Math.floor((cx + size.width / 2) / 256);
      x++
    ) {
      if (y < 0 || y >= 2 ** zoom) continue;
      const xx = ((x % 2 ** zoom) + 2 ** zoom) % 2 ** zoom;
      tiles.push({ x, y, url: `/maps/tiles/${style}/${zoom}/${xx}/${y}.png` });
    }
  async function search(q: string) {
    try {
      const d = await json<any>("/places?q=" + encodeURIComponent(q));
      const p = d.results?.[0] || d.places?.[0] || d;
      if (p.lat === undefined) throw Error("Place not found");
      setCenter({ lat: p.lat, lon: p.lon });
      setZoom(13);
      setError("");
    } catch (e) {
      setError((e as Error).message);
    }
  }
  return (
    <>
      <PageHeading title="Maps" />
      <Search placeholder="Find a place" onSearch={(q) => void search(q)} />
      <div className="mb-3 flex flex-wrap gap-2">
        <Button
          aria-label="Zoom in"
          disabled={zoom >= 18}
          onClick={() => setZoom((z) => z + 1)}
        >
          +
        </Button>
        <Button
          aria-label="Zoom out"
          disabled={zoom <= 1}
          onClick={() => setZoom((z) => z - 1)}
        >
          −
        </Button>
        <Button
          onClick={() =>
            navigator.geolocation.getCurrentPosition(
              (p) => {
                setCenter({ lat: p.coords.latitude, lon: p.coords.longitude });
                setZoom(14);
              },
              (e) => setError(e.message),
            )
          }
        >
          My location
        </Button>
        <NativeSelect
          aria-label="Map style"
          value={style}
          onChange={(e) => setStyle(e.target.value)}
        >
          {["world", "road", "outdoor", "light"].map((s) => (
            <option key={s}>{s}</option>
          ))}
        </NativeSelect>
      </div>
      {error && <Status error>{error}</Status>}
      <div
        ref={box}
        role="region"
        aria-label="Map. Use arrow keys to pan."
        tabIndex={0}
        className="relative h-[60dvh] min-h-80 w-full touch-none overflow-hidden rounded border bg-muted"
        onKeyDown={(e) => {
          const delta: Record<string, number[]> = {
            ArrowLeft: [-100, 0],
            ArrowRight: [100, 0],
            ArrowUp: [0, -100],
            ArrowDown: [0, 100],
          };
          if (delta[e.key]) {
            e.preventDefault();
            move(cx + delta[e.key][0], cy + delta[e.key][1]);
          }
        }}
        onPointerDown={(e) => {
          e.currentTarget.setPointerCapture(e.pointerId);
          drag.current = { x: e.clientX, y: e.clientY, px: cx, py: cy };
        }}
        onPointerMove={(e) => {
          if (drag.current)
            move(
              drag.current.px - e.clientX + drag.current.x,
              drag.current.py - e.clientY + drag.current.y,
            );
        }}
        onPointerUp={() => {
          drag.current = undefined;
        }}
        onPointerCancel={() => {
          drag.current = undefined;
        }}
      >
        {tiles.map((t) => (
          <img
            key={`${zoom}/${style}/${t.x}/${t.y}`}
            alt=""
            draggable={false}
            className="pointer-events-none absolute max-w-none"
            style={{
              width: 256,
              height: 256,
              left: t.x * 256 - cx + size.width / 2,
              top: t.y * 256 - cy + size.height / 2,
            }}
            src={t.url}
          />
        ))}
        <a
          className="absolute bottom-0 right-0 bg-white/90 px-2 py-1 text-xs underline"
          href="https://www.openstreetmap.org/copyright"
        >
          © OpenStreetMap contributors
        </a>
      </div>
    </>
  );
}
