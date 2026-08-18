import { cva, type VariantProps } from "class-variance-authority";

import { cn } from "@/lib/utils";

const messageVariants = cva("rounded-2xl px-3 py-2 text-sm", {
  variants: {
    tone: {
      error: "bg-destructive/10 text-destructive font-medium",
      success: "bg-accent text-accent-foreground font-medium",
      info: "text-muted-foreground px-0 py-0",
    },
  },
  defaultVariants: { tone: "info" },
});

/**
 * Inline form and page feedback. Errors are announced immediately; loading and
 * success text is polite, so a slow API does not interrupt a screen reader
 * mid-sentence.
 */
function Message({
  className,
  tone = "info",
  ...props
}: React.ComponentProps<"p"> & VariantProps<typeof messageVariants>) {
  return (
    <p
      data-slot="message"
      data-tone={tone}
      role={tone === "error" ? "alert" : "status"}
      className={cn(messageVariants({ tone }), className)}
      {...props}
    />
  );
}

export { Message };
