import * as React from "react";

import { cn } from "@/lib/utils";

const variants = {
  // A control that reads as a field: for credentials, search and anything on
  // a page that is genuinely a form.
  box: "border-input bg-card h-11 rounded-lg border px-4 py-1 focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:bg-input/50 dark:bg-input/30 dark:disabled:bg-input/80 aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40",
  // A line to write on, for the places the scrapbook wants handwriting rather
  // than a form: a meal note, a display name, a new group's name. The rule
  // sits at 27px so one line of 1rem text rests on it.
  ruled:
    "ruled h-7 rounded-none border-0 bg-transparent px-0 leading-[1.6875rem] focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ring aria-invalid:text-destructive",
} as const;

function Input({
  className,
  type,
  variant = "box",
  ...props
}: React.ComponentProps<"input"> & {
  /** `ruled` swaps the box for a notebook rule. See DESIGN_SYSTEM.md. */
  variant?: keyof typeof variants;
}) {
  return (
    <input
      type={type}
      data-slot="input"
      data-variant={variant}
      className={cn(
        "w-full min-w-0 text-base transition-colors outline-none file:mr-3 file:inline-flex file:h-7 file:items-center file:rounded-full file:border-0 file:bg-secondary file:px-3 file:text-sm file:font-semibold file:text-secondary-foreground placeholder:text-muted-foreground disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 md:text-sm",
        variants[variant],
        className,
      )}
      {...props}
    />
  );
}

export { Input };
