import { UserIcon } from "lucide-react";

import { avatarUrl } from "@/lib/api";
import { cn } from "@/lib/utils";

type AvatarProps = {
  mediaId?: string;
  displayName: string;
  className?: string;
};

/**
 * Someone's picture, or a neutral placeholder.
 *
 * Setting an avatar is optional and onboarding skips it, so "no picture" is a
 * normal state rather than an error — every list has to render it calmly.
 */
export function Avatar({ mediaId, displayName, className }: AvatarProps) {
  const shape = cn(
    "bg-muted size-10 shrink-0 overflow-hidden rounded-full",
    className,
  );

  if (!mediaId) {
    return (
      <div className={cn(shape, "text-muted-foreground grid place-items-center")}>
        <UserIcon className="size-1/2" aria-hidden />
        <span className="sr-only">{displayName}尚未設定頭像</span>
      </div>
    );
  }

  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={avatarUrl(mediaId)}
      alt={`${displayName}的頭像`}
      className={cn(shape, "object-cover")}
    />
  );
}
