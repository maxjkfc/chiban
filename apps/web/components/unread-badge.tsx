type UnreadBadgeProps = {
  count?: number;
  showCount?: boolean;
  label?: string;
};

/** A compact, accessible unread indicator used by navigation and group cards. */
export function UnreadBadge({
  count = 0,
  showCount = false,
  label = "有未讀訊息",
}: UnreadBadgeProps) {
  if (count <= 0) return null;

  const displayCount = count > 99 ? "99+" : count;
  return (
    <span
      aria-label={label}
      className="bg-destructive text-destructive-foreground inline-flex min-h-2 min-w-2 shrink-0 items-center justify-center rounded-full px-1.5 text-[0.625rem] leading-4 font-bold"
      role="status"
    >
      {showCount ? displayCount : null}
    </span>
  );
}
