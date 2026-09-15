import type { ComponentProps, ReactNode } from "react";
import { Input } from "./input";
import { Label } from "./label";
export function Field({
  label,
  help,
  ...props
}: ComponentProps<typeof Input> & { label: string; help?: ReactNode }) {
  const id = props.id || props.name;
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      <Input
        {...props}
        id={id}
        aria-describedby={help ? id + "-help" : undefined}
      />
      {help && (
        <p id={id + "-help"} className="text-sm text-muted-foreground">
          {help}
        </p>
      )}
    </div>
  );
}
