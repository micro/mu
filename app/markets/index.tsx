import { useState } from "react";
import {
  useData,
  json,
  call,
  Button,
  Status,
  Form,
  Read,
  Link,
} from "../shared";
import { PageHeading } from "../../web/src/components/layout";

type Quote = {
  symbol: string;
  name?: string;
  chart?: string;
  price: number;
  change_24h: number;
  source: string;
  updated_at?: string;
};
type Prices = {
  data: Quote[];
  freshness: string;
  stale: boolean;
  partial: boolean;
};
export function App() {
  const category =
    new URLSearchParams(location.search).get("category") || "crypto";
  const { data, error } = useData<Prices>(() =>
    json(`/markets?category=${encodeURIComponent(category)}`),
  );
  const [conversion, setConversion] = useState("");
  return (
    <>
      <PageHeading title="Markets" />
      <nav aria-label="Market categories" className="mb-5 flex flex-wrap gap-2">
        {["crypto", "stocks", "futures", "commodities", "currencies"].map(
          (c) => (
            <Button
              key={c}
              asChild
              aria-current={c === category ? "page" : undefined}
            >
              <a
                href={`/markets?category=${c}`}
                className="capitalize aria-[current=page]:bg-foreground aria-[current=page]:text-background"
              >
                {c}
              </a>
            </Button>
          ),
        )}
      </nav>
      {error && <Status error>{error}</Status>}
      {data && (
        <>
          <div className="overflow-x-auto">
            <table className="w-full text-sm tabular-nums">
              <thead>
                <tr className="border-b text-left">
                  <th className="py-3">Symbol</th>
                  <th className="px-3 py-3 text-right">Price (USD)</th>
                  <th className="py-3 text-right">24h change</th>
                  <th className="py-3 pl-3 text-right">Chart</th>
                </tr>
              </thead>
              <tbody>
                {[...(data.data || [])]
                  .sort((a, b) => a.symbol.localeCompare(b.symbol))
                  .map((q) => (
                    <tr key={q.symbol} className="border-b">
                      <th scope="row" className="py-4 text-left font-semibold">
                        {q.symbol}
                        {q.name && (
                          <span className="mt-1 block text-xs font-normal text-muted-foreground">
                            {q.name}
                          </span>
                        )}
                      </th>
                      <td className="px-3 py-4 text-right">
                        {q.price
                          ? new Intl.NumberFormat(undefined, {
                              style: "currency",
                              currency: "USD",
                              maximumFractionDigits: q.price < 1 ? 6 : 2,
                            }).format(q.price)
                          : "Unavailable"}
                      </td>
                      <td
                        className={`py-4 text-right font-medium ${q.change_24h < 0 ? "text-red-700" : "text-green-800"}`}
                      >
                        {q.price
                          ? `${q.change_24h > 0 ? "+" : ""}${q.change_24h.toFixed(2)}%`
                          : "—"}
                      </td>
                      <td className="py-4 pl-3 text-right">
                        {q.chart && <Link url={q.chart}>Chart</Link>}
                      </td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
          <p className="mt-3 text-sm text-muted-foreground">{data.freshness}</p>
        </>
      )}
      <details className="mt-6 border-t pt-4">
        <summary className="cursor-pointer font-semibold">
          Currency and asset converter
        </summary>
        <div className="mt-4">
          <Form
            fields={[
              {
                name: "amount",
                label: "Amount",
                type: "number",
                value: "1",
                required: true,
              },
              { name: "from", label: "From", value: "GBP", required: true },
              { name: "to", label: "To", value: "USD", required: true },
              {
                name: "date",
                label: "Historical date (optional)",
                type: "date",
              },
            ]}
            label="Convert"
            submit={async (v) => {
              const r = await call("markets", "convert", {
                ...v,
                amount: Number(v.amount),
              });
              setConversion(r.text);
            }}
          />
          {conversion && <Read>{conversion}</Read>}
        </div>
      </details>
    </>
  );
}
