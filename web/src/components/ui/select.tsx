import type { ComponentProps } from "react";
import { cn } from "../../lib/utils";
export function NativeSelect({
  className,
  ...props
}: ComponentProps<"select">) {
  return (
    <select
      data-slot="select"
      className={cn(
        "h-9 min-w-0 rounded-md border border-input bg-background px-3 text-base outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:opacity-50",
        className,
      )}
      {...props}
    />
  );
}
