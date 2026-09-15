import { useState, type ReactNode } from "react";
import { Dialog } from "radix-ui";
import {
  Home,
  Inbox,
  Bot,
  Grid2X2,
  Menu,
  X,
  Settings,
  LogOut,
  Shield,
} from "lucide-react";
import { Button } from "./ui/button";
import type { Identity } from "../lib/api";

const links = [
  { href: "/", label: "Home", icon: Home },
  { href: "/inbox", label: "Inbox", icon: Inbox },
  { href: "/agents", label: "Agents", icon: Bot },
  { href: "/services", label: "Services", icon: Grid2X2 },
];
function Navigation({
  account,
  onNavigate,
}: {
  account: Identity;
  onNavigate?: () => void;
}) {
  return (
    <div className="flex h-full min-h-0 flex-col p-3">
      <a href="/" className="px-3 py-4 text-lg font-semibold">
        Micro
      </a>
      <nav aria-label="Main" className="space-y-1">
        {links.map(({ href, label, icon: Icon }) => (
          <a
            key={href}
            onClick={onNavigate}
            aria-current={
              (
                href === "/"
                  ? location.pathname === "/"
                  : location.pathname.startsWith(href)
              )
                ? "page"
                : undefined
            }
            className="flex items-center gap-3 rounded-lg px-3 py-2.5 text-base hover:bg-accent aria-[current=page]:bg-accent"
            href={href}
          >
            <Icon className="size-4" />
            {label}
          </a>
        ))}
      </nav>
      <nav aria-label="Account" className="mt-auto space-y-1 border-t pt-3">
        <a
          href="/account"
          className="flex items-center gap-3 rounded-lg px-3 py-2.5 hover:bg-accent"
        >
          <Settings className="size-4" />
          <span className="truncate font-normal">@{account.id}</span>
        </a>
        {account.admin && (
          <a
            className="flex items-center gap-3 rounded-lg px-3 py-2 hover:bg-accent"
            href="/admin"
          >
            <Shield className="size-4" />
            Admin
          </a>
        )}
        <a
          className="flex items-center gap-3 rounded-lg px-3 py-2 hover:bg-accent"
          href="/logout"
        >
          <LogOut className="size-4" />
          Log out
        </a>
      </nav>
    </div>
  );
}
export function Layout({
  account,
  title,
  children,
  conversation = false,
}: {
  account: Identity | null;
  title: string;
  children: ReactNode;
  conversation?: boolean;
}) {
  const [open, setOpen] = useState(false);
  return (
    <div className="flex min-h-dvh bg-background text-foreground">
      {account && (
        <aside className="fixed inset-y-0 left-0 hidden w-56 border-r bg-muted/30 md:block">
          <Navigation account={account} />
        </aside>
      )}
      <div
        className={
          "flex min-w-0 flex-1 flex-col " +
          (account ? "md:ml-56 " : "") +
          (conversation ? "h-dvh overflow-hidden" : "")
        }
      >
        <header className="flex h-14 shrink-0 items-center justify-between gap-3 border-b px-4 sm:px-6">
          {account ? (
            <>
              <Dialog.Root open={open} onOpenChange={setOpen}>
                <Dialog.Trigger asChild>
                  <Button
                    size="icon"
                    variant="ghost"
                    className="md:hidden"
                    aria-label="Open navigation"
                  >
                    <Menu />
                  </Button>
                </Dialog.Trigger>
                <Dialog.Portal>
                  <Dialog.Overlay className="fixed inset-0 z-40 bg-black/30" />
                  <Dialog.Content className="fixed inset-y-0 left-0 z-50 w-64 max-w-[85vw] bg-background shadow-lg">
                    <Dialog.Title className="sr-only">Navigation</Dialog.Title>
                    <Dialog.Description className="sr-only">
                      Your conversations and account
                    </Dialog.Description>
                    <Navigation
                      account={account}
                      onNavigate={() => setOpen(false)}
                    />
                    <Dialog.Close asChild>
                      <Button
                        size="icon"
                        variant="ghost"
                        className="absolute right-2 top-2"
                        aria-label="Close navigation"
                      >
                        <X />
                      </Button>
                    </Dialog.Close>
                  </Dialog.Content>
                </Dialog.Portal>
              </Dialog.Root>
              <span className="min-w-0 flex-1 truncate font-medium">
                {title}
              </span>
            </>
          ) : (
            <>
              <a href="/" className="font-semibold">
                Micro
              </a>
              <Button variant="ghost" asChild>
                <a href="/login?redirect=%2F">Log in</a>
              </Button>
            </>
          )}
        </header>
        <main
          className={
            "mx-auto w-full min-w-0 flex-1 px-4 sm:px-6 " +
            (conversation
              ? "flex min-h-0 max-w-3xl flex-col py-3"
              : "max-w-6xl py-6")
          }
        >
          {children}
        </main>
        {!account && location.pathname === "/" && (
          <footer className="flex shrink-0 flex-wrap justify-center gap-x-4 gap-y-2 px-4 py-3 text-sm text-muted-foreground">
            {["About", "Contact", "Pricing", "Privacy", "Status"].map(
              (label) => (
                <a key={label} href={"/" + label.toLowerCase()}>
                  {label}
                </a>
              ),
            )}
          </footer>
        )}
      </div>
    </div>
  );
}

export function PageHeading({
  title,
  actions,
}: {
  title: string;
  actions?: ReactNode;
}) {
  return (
    <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
      <h1 className="min-w-0 max-w-full break-words text-2xl font-semibold tracking-tight">
        {title}
      </h1>
      <div className="flex flex-wrap items-center gap-2">{actions}</div>
    </div>
  );
}
export function Section({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <section className="space-y-4 border-t py-5 first:border-t-0 first:pt-0">
      <h2 className="text-lg font-medium">{title}</h2>
      {children}
    </section>
  );
}
export function Status({
  error,
  children,
}: {
  error?: boolean;
  children: ReactNode;
}) {
  return (
    <p
      role={error ? "alert" : "status"}
      className={
        "text-sm " + (error ? "text-destructive" : "text-muted-foreground")
      }
    >
      {children}
    </p>
  );
}
export function Pager({
  page,
  total,
  size,
  onChange,
}: {
  page: number;
  total: number;
  size: number;
  onChange: (page: number) => void;
}) {
  return (
    <nav
      aria-label="Pagination"
      className="mt-5 flex items-center justify-between gap-3 text-sm"
    >
      <Button
        variant="outline"
        disabled={page <= 1}
        onClick={() => onChange(page - 1)}
      >
        Previous
      </Button>
      <span>
        {page} of {Math.max(1, Math.ceil(total / size))}
      </span>
      <Button
        variant="outline"
        disabled={page * size >= total}
        onClick={() => onChange(page + 1)}
      >
        Next
      </Button>
    </nav>
  );
}
