import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { Button } from "./ui/button";
import { Field } from "./ui/field";
import { Switch } from "./ui/switch";
import { PageHeading, Section, Status } from "./layout";
import { initialData, json, mutate } from "../lib/api";
import { passkey } from "../lib/passkey";
type Account = {
  id: string;
  name: string;
  status: string;
  email: string;
  email_verified: boolean;
  addresses: string[];
  numbers: string[];
  pending_number: string;
  phone_enabled: boolean;
  balance: number;
  admin: boolean;
  daily_credits: number;
  included_today: number;
  payments: boolean;
  place: string;
  lat: number;
  lon: number;
  timezone: string;
  forwarding: boolean;
  google_enabled: boolean;
  google_linked: boolean;
  passkeys: { id: string; name: string; created: string; last_used: string }[];
  transactions: {
    id: string;
    label: string;
    amount_label: string;
    balance: number;
    created_at: string;
  }[];
};
export function AccountPage() {
  const [account, setAccount] = useState<Account | undefined>(() => initialData<Account>()),
    [error, setError] = useState(""),
    [notice, setNotice] = useState(""),
    [busy, setBusy] = useState(false);
  const path = location.pathname,
    profile = path === "/account/profile",
    billing = path === "/account/billing" || path === "/account/usage";
  async function load() {
    setAccount(await json<Account>(path));
  }
  useEffect(() => {
    load().catch((e) => setError(e.message));
  }, [path]);
  async function run(action: () => Promise<unknown>, message = "Saved.") {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await action();
      await load();
      setNotice(message);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = e.currentTarget;
    await run(
      () =>
        mutate(
          f.getAttribute("action") || path,
          Object.fromEntries(new FormData(f)) as Record<string, string>,
        ),
      f.dataset.notice || "Saved.",
    );
  }
  const form = (
    children: ReactNode,
    label = "Save",
    action = path,
    notice = "Saved.",
  ) => (
    <form
      action={action}
      onSubmit={submit}
      data-notice={notice}
      className="max-w-xl space-y-3"
    >
      {children}
      <Button disabled={busy}>{label}</Button>
    </form>
  );
  function locate() {
    if (!navigator.geolocation) {
      setError("Location is unavailable in this browser.");
      return;
    }
    setBusy(true);
    navigator.geolocation.getCurrentPosition(
      (p) =>
        run(() =>
          mutate("/account/place", {
            place: account?.place || "",
            lat: (Math.round(p.coords.latitude * 200) / 200).toString(),
            lon: (Math.round(p.coords.longitude * 200) / 200).toString(),
            zone: Intl.DateTimeFormat().resolvedOptions().timeZone,
          }),
        ),
      () => {
        setBusy(false);
        setError("Location unavailable. You can enter your city instead.");
      },
      { timeout: 15000, maximumAge: 30000, enableHighAccuracy: true },
    );
  }
  const links = (items: [string, string][]) => (
    <div className="flex flex-wrap gap-2">
      {items.map(([href, label]) => (
        <Button asChild variant="outline" key={href}>
          <a href={href}>{label}</a>
        </Button>
      ))}
    </div>
  );
  return (
    <div className="max-w-3xl">
      <PageHeading
        title={billing ? "Billing" : profile ? "Profile" : "Account"}
        actions={
          path !== "/account" && (
            <Button asChild variant="outline">
              <a href="/account">Account</a>
            </Button>
          )
        }
      />
      {error && <Status error>{error}</Status>}
      <Status>{busy ? "Saving…" : notice}</Status>
      {account ? (
        <div className="mt-4">
          {billing ? (
            <>
              <Section title="Balance">
                <p>{account.balance.toLocaleString()} credits</p>
                <p className="text-sm text-muted-foreground">1 credit = 1¢</p>
                {account.daily_credits > 0 && (
                  <p className="text-sm text-muted-foreground">
                    {account.included_today.toLocaleString()} of{" "}
                    {account.daily_credits.toLocaleString()} included credits
                    left today. Resets at midnight UTC; unused credits do not
                    roll over. Messaging limits still apply.
                  </p>
                )}
                {account.admin && (
                  <p className="text-sm text-muted-foreground">
                    Your own calls are not charged because you are an admin.
                  </p>
                )}
                {links([
                  ...(account.payments
                    ? [["/account/topup", "Top up"] as [string, string]]
                    : []),
                  ["/account/transfer", "Transfer"],
                  ["/usage", "View usage"],
                ])}
              </Section>
              <Section title="Recent transactions">
                <p className="text-sm text-muted-foreground">
                  Latest 20 transactions, newest first.
                </p>
                <div className="divide-y">
                  {account.transactions?.map((t) => (
                    <div
                      key={t.id}
                      className="flex items-start justify-between gap-3 py-3"
                    >
                      <div className="min-w-0">
                        <p className="break-words">{t.label}</p>
                        <time
                          className="text-sm text-muted-foreground"
                          dateTime={t.created_at}
                        >
                          {new Date(t.created_at).toLocaleString()}
                        </time>
                      </div>
                      <p className="shrink-0 text-right tabular-nums">
                        {t.amount_label}
                        {t.amount_label !== "included" && " credits"}
                        <span className="block text-sm text-muted-foreground">
                          {t.balance.toLocaleString()} balance
                        </span>
                      </p>
                    </div>
                  ))}
                  {!account.transactions?.length && <p>No transactions yet.</p>}
                </div>
              </Section>
            </>
          ) : profile ? (
            <>
              <Section title="Identity">
                <p>@{account.id}</p>
                {form(
                  <>
                    <input type="hidden" name="save_name" value="1" />
                    <Field
                      name="display_name"
                      label="Display name"
                      defaultValue={account.name}
                      maxLength={60}
                    />
                  </>,
                )}
                {links([
                  ["/@" + encodeURIComponent(account.id), "View profile"],
                ])}
              </Section>
              <Section title="Status">
                <p>{account.status || "No status set."}</p>
                {links([
                  [
                    "/@" + encodeURIComponent(account.id) + "#profile-status",
                    "Set status",
                  ],
                ])}
              </Section>
              <Section title="Location">
                <p className="text-sm text-muted-foreground">
                  Used for local weather, nearby places and scheduled briefs.
                  Coordinates are rounded to a grid of roughly 500 metres.
                </p>
                {form(
                  <>
                    <Field
                      name="place"
                      label="Town or city"
                      defaultValue={account.place}
                      maxLength={120}
                    />
                    <Field
                      name="zone"
                      label="Timezone"
                      defaultValue={
                        account.timezone ||
                        Intl.DateTimeFormat().resolvedOptions().timeZone
                      }
                      placeholder="Europe/London"
                    />
                    <input type="hidden" name="lat" value={account.lat} />
                    <input type="hidden" name="lon" value={account.lon} />
                  </>,
                  "Save",
                  "/account/place",
                )}
                <div className="flex flex-wrap gap-2">
                  <Button disabled={busy} onClick={locate}>
                    Use my location
                  </Button>
                  <Button
                    disabled={busy}
                    variant="ghost"
                    onClick={() =>
                      run(() =>
                        mutate("/account/place", {
                          place: "",
                          lat: "0",
                          lon: "0",
                          zone: "",
                        }),
                      )
                    }
                  >
                    Forget location
                  </Button>
                </div>
                {(account.lat !== 0 || account.lon !== 0) && (
                  <p className="text-sm text-muted-foreground">
                    Saved: {account.lat}, {account.lon}
                  </p>
                )}
              </Section>
            </>
          ) : (
            <>
              <section className="space-y-4 pb-5">
                <p>@{account.id}</p>
                {links([
                  ["/account/profile", "Profile"],
                  ["/account/billing", "Billing"],
                ])}
              </section>
              <Section title="Email">
                <p className="break-words">
                  {account.email || "No email address linked"}
                  {account.email &&
                    (account.email_verified
                      ? " · Verified"
                      : " · Verification needed")}
                </p>
                {form(
                  <Field
                    name="email"
                    label="Email address"
                    type="email"
                    defaultValue={account.email}
                    placeholder="you@example.com"
                    required
                  />,
                  "Send verification link",
                  path,
                  "Verification email sent.",
                )}
                {account.addresses
                  ?.filter((a) => a !== account.email)
                  .map((a) => (
                    <div
                      key={a}
                      className="flex items-center justify-between gap-3"
                    >
                      <span className="min-w-0 break-all">{a}</span>
                      <Button
                        disabled={busy}
                        variant="outline"
                        onClick={() =>
                          run(() => mutate(path, { forget_address: a }))
                        }
                      >
                        Remove
                      </Button>
                    </div>
                  ))}
                {account.email_verified && (
                  <div className="flex items-center justify-between gap-4">
                    <label htmlFor="forwarding">Forward mail to my email</label>
                    <Switch
                      id="forwarding"
                      checked={account.forwarding}
                      disabled={busy}
                      onCheckedChange={(value) =>
                        run(() =>
                          mutate(path, { forwarding: value ? "on" : "off" }),
                        )
                      }
                    />
                  </div>
                )}
              </Section>
              {account.phone_enabled && (
                <Section title="Phone numbers">
                  {account.numbers?.map((n) => (
                    <div key={n} className="flex flex-wrap items-center gap-3">
                      <span>{n}</span>
                      <Button
                        disabled={busy}
                        variant="outline"
                        onClick={() =>
                          run(() => mutate(path, { forget_number: n }))
                        }
                      >
                        Remove
                      </Button>
                    </div>
                  ))}
                  {account.pending_number
                    ? form(
                        <>
                          <input
                            type="hidden"
                            name="confirm_number"
                            value={account.pending_number}
                          />
                          <p>Code sent to {account.pending_number}</p>
                          <Field
                            name="code"
                            label="Verification code"
                            inputMode="numeric"
                            required
                            autoComplete="one-time-code"
                          />
                        </>,
                        "Verify",
                      )
                    : form(
                        <Field
                          name="verify_number"
                          label="Link a phone number"
                          type="tel"
                          placeholder="+44…"
                          required
                        />,
                        "Send code",
                        path,
                        "Verification code sent.",
                      )}
                </Section>
              )}
              <Section title="Password">
                {form(
                  <>
                    <input type="hidden" name="save_secret" value="1" />
                    <Field
                      name="new_secret"
                      label="New password"
                      type="password"
                      autoComplete="new-password"
                      minLength={6}
                      required
                    />
                    <Field
                      name="confirm_secret"
                      label="Confirm password"
                      type="password"
                      autoComplete="new-password"
                      minLength={6}
                      required
                    />
                  </>,
                  "Save password",
                )}
              </Section>
              <Section title="Passkeys">
                <p className="text-sm text-muted-foreground">
                  Sign in with your device or security key.
                </p>
                {account.passkeys.map((k) => (
                  <div
                    key={k.id}
                    className="flex items-center justify-between gap-3"
                  >
                    <div className="min-w-0">
                      <p className="break-words">{k.name}</p>
                      <p className="text-sm text-muted-foreground">
                        Added {new Date(k.created).toLocaleDateString()}
                      </p>
                    </div>
                    <Button
                      disabled={busy}
                      variant="outline"
                      onClick={() => {
                        if (confirm("Remove this passkey?"))
                          run(() => mutate("/passkey/delete", { id: k.id }));
                      }}
                    >
                      Remove
                    </Button>
                  </div>
                ))}
                <Button
                  disabled={busy}
                  onClick={() => run(() => passkey(true), "Passkey added.")}
                >
                  Add a passkey
                </Button>
              </Section>
              {account.google_enabled && (
                <Section title="Google sign-in">
                  {account.google_linked ? (
                    <p>Connected</p>
                  ) : (
                    links([["/oauth2/google/connect", "Connect Google"]])
                  )}
                </Section>
              )}
              <Section title="Connections">
                {links([
                  ["/token", "API credentials"],
                  ["/inbox/imap", "Mail settings"],
                ])}
              </Section>
              <Section title="Notifications">
                {links([
                  ["/inbox/settings", "Scheduled messages"],
                  ["/notify", "Notification settings"],
                ])}
              </Section>
            </>
          )}
        </div>
      ) : (
        !error && <Status>Loading account…</Status>
      )}
    </div>
  );
}
