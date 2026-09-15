import type { ComponentProps } from "react";
import { cn } from "../../lib/utils";

export function Badge({ className, ...props }: ComponentProps<"span">) {
  return (
    <span
      data-slot="badge"
      className={cn(
        "inline-flex shrink-0 items-center rounded-md border border-control-border bg-control px-2 py-0.5 text-xs font-semibold leading-5 text-foreground",
        className,
      )}
      {...props}
    />
  );
}
