import * as React from "react";
import { Tabs as Primitive } from "radix-ui";
import { cn } from "../../lib/utils";
export const Tabs = Primitive.Root;
export function TabsList({
  className,
  ...props
}: React.ComponentProps<typeof Primitive.List>) {
  return (
    <Primitive.List
      className={cn(
        "inline-flex max-w-full items-center gap-1 rounded-lg bg-muted p-1",
        className,
      )}
      {...props}
    />
  );
}
export function TabsTrigger({
  className,
  ...props
}: React.ComponentProps<typeof Primitive.Trigger>) {
  return (
    <Primitive.Trigger
      className={cn(
        "rounded-md px-3 py-1.5 text-sm font-medium data-[state=active]:bg-background data-[state=active]:shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring",
        className,
      )}
      {...props}
    />
  );
}
