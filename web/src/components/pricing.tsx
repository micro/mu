import { Button } from "./ui/button";
export type PricingData = {
  payments: boolean;
  topup: boolean;
  question_cost: number;
  welcome: number;
  daily: number;
  prices: {
    operation: string;
    description: string;
    cost: number;
    unit: string;
  }[];
  limits: { label: string; limit: number }[];
};
export function Pricing({ data }: { data: PricingData }) {
  if (!data.payments)
    return (
      <section className="space-y-3">
        <h2 className="text-lg font-semibold">Free on this instance</h2>
        <p>
          This instance does not take payments. The operator covers the cost of
          models and other providers.
        </p>
      </section>
    );
  return (
    <div className="space-y-8">
      <section className="space-y-3">
        <h2 className="text-lg font-semibold">Pay for what you use</h2>
        <p>
          A question to Micro costs {data.question_cost} credits. Paid tools
          used to answer it are charged separately.
        </p>
        <p>
          One credit is one US cent. Reading saved information is free; cached
          results that do not call a paid provider are not charged.
        </p>
      </section>
      <section className="space-y-3">
        <h2 className="text-lg font-semibold">Start without a card</h2>
        <p>
          A new account receives {data.welcome} welcome credits ($
          {(data.welcome / 100).toFixed(2)}).
        </p>
        {data.daily > 0 && (
          <p>
            {data.daily} included credits each day are used before your paid
            balance. They reset at midnight UTC and do not roll over.
          </p>
        )}
        {data.topup ? (
          <>
            <p>
              Top up with $5, $10, $25 or $50, or choose an amount. No
              subscription or charge for an idle account.
            </p>
            <div className="flex flex-wrap gap-2">
              <Button asChild>
                <a href="/signup">Get started</a>
              </Button>
              <Button asChild>
                <a href="/account/topup">Top up</a>
              </Button>
            </div>
          </>
        ) : (
          <p>This instance accepts payment over x402 rather than by card.</p>
        )}
      </section>
      <section className="space-y-3">
        <h2 className="text-lg font-semibold">Operation costs</h2>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b">
                <th className="py-3 pr-4">Operation</th>
                <th className="py-3 text-right">Credits</th>
              </tr>
            </thead>
            <tbody>
              {data.prices.map((p) => (
                <tr key={p.operation} className="border-b">
                  <th scope="row" className="py-3 pr-4 font-normal">
                    {p.description}
                  </th>
                  <td className="py-3 text-right tabular-nums">
                    {p.cost === 0 ? "Free" : p.cost}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
      {data.limits.length > 0 && (
        <section className="space-y-3">
          <h2 className="text-lg font-semibold">Daily limits</h2>
          <p>
            Sending and call limits are separate from credits and reset at
            midnight UTC. Account-specific limits may differ.
          </p>
          <dl className="divide-y">
            {data.limits.map((l) => (
              <div
                key={l.label}
                className="flex justify-between gap-4 py-3 text-sm"
              >
                <dt>{l.label}</dt>
                <dd className="shrink-0 tabular-nums">{l.limit} per day</dd>
              </div>
            ))}
          </dl>
        </section>
      )}
    </div>
  );
}
