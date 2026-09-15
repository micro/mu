import type { ComponentProps } from "react";
import { cn } from "../../lib/utils";
export function Card({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      data-slot="card"
      className={cn(
        "flex min-w-0 flex-col gap-3 rounded-xl border bg-background p-4",
        className,
      )}
      {...props}
    />
  );
}
