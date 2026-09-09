"use client";

import { CameraIcon, UserIcon, UsersIcon } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";

import { UnreadBadge } from "@/components/unread-badge";
import { useUnreadGroups } from "@/components/unread-provider";
import { isActiveNavItem, primaryNavItems } from "@/lib/navigation.mts";
import { groupHasUnread } from "@/lib/unread";

const icons = {
  groups: UsersIcon,
  record: CameraIcon,
  profile: UserIcon,
} as const;

/**
 * Primary navigation. Recording is the single central action rather than an
 * action hidden inside a chat composer, because it is the product's main job.
 *
 * The rail has three destinations so the action can sit on the visual center.
 * Groups and profile stay quiet on either side; Today remains a page reached
 * from the app root and meal links, not a competing bottom-bar action.
 */
export function BottomNav() {
  const pathname = usePathname();
  const { groups } = useUnreadGroups();
  const hasUnreadGroup = groups?.some(groupHasUnread) ?? false;

  // A conversation takes the whole screen. Leaving the nav here stacked three
  // bars at the bottom — quick rail, composer, nav — and pushed the newest
  // message off a short phone. Getting out is the header's back button.
  if (/^\/groups\/[^/]+$/.test(pathname)) return null;

  return (
    <nav
      aria-label="主要導航"
      className="bg-background/92 border-border sticky bottom-0 border-t pb-[env(safe-area-inset-bottom)] backdrop-blur"
    >
      <ul className="grid grid-cols-3 items-end">
        {primaryNavItems.map((item) => {
          const active = pathname ? isActiveNavItem(pathname, item.href) : false;
          const Icon = icons[item.icon];
          return (
            <li key={item.href} className="flex justify-center">
              <Link
                href={item.href}
                aria-current={active ? "page" : undefined}
                aria-label={item.primary ? "記錄一餐" : undefined}
                className={
                  item.primary
                    ? "text-primary-foreground relative -mt-5 flex h-[4.75rem] w-20 flex-col items-center justify-center gap-0.5 rounded-full bg-primary shadow-pop transition-transform active:scale-95"
                    : `flex h-[3.75rem] w-full max-w-20 flex-col items-center justify-center gap-1 text-[0.72rem] transition-colors ${
                        active
                          ? "text-primary-ink font-bold"
                          : "text-muted-foreground"
                      }`
                }
              >
                <span className="relative">
                  <Icon
                    className={item.primary ? "size-7" : active ? "size-6" : "size-5"}
                    aria-hidden
                  />
                  {item.href === "/groups" && hasUnreadGroup ? (
                    <span className="absolute -top-1 -right-2">
                      <UnreadBadge count={1} />
                    </span>
                  ) : null}
                </span>
                {item.label}
                {item.primary ? null : (
                  /* Colour is never the only cue: the icon grows, the label
                     bolds, and this rule appears. */
                  <span
                    aria-hidden
                    className={`h-0.5 w-4 rounded-full ${active ? "bg-primary-ink" : "bg-transparent"}`}
                  />
                )}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
