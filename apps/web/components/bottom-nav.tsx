"use client";

import { CameraIcon, SunIcon, UserIcon, UsersIcon } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";

const items = [
  { href: "/today", label: "今日", icon: SunIcon },
  { href: "/record", label: "記錄", icon: CameraIcon },
  { href: "/groups", label: "群組", icon: UsersIcon },
  { href: "/profile", label: "我的", icon: UserIcon },
] as const;

/**
 * Primary navigation. Recording is its own destination rather than an action
 * hidden inside a chat composer, because it is the product's main job.
 *
 * The current tab is marked by a short coral rule under its label rather than
 * by a filled pill: the pill read as a button, which made four buttons sit
 * where there is only ever one thing to press.
 */
export function BottomNav() {
  const pathname = usePathname();

  return (
    <nav
      aria-label="主要導航"
      className="bg-background/92 border-border sticky bottom-0 border-t pb-[env(safe-area-inset-bottom)] backdrop-blur"
    >
      <ul className="grid grid-cols-4">
        {items.map((item) => {
          const active = pathname === item.href;
          const Icon = item.icon;
          return (
            <li key={item.href} className="flex justify-center">
              <Link
                href={item.href}
                aria-current={active ? "page" : undefined}
                className={`flex h-[3.75rem] w-full max-w-20 flex-col items-center justify-center gap-1 text-[0.72rem] transition-colors ${
                  active ? "text-primary-ink font-bold" : "text-muted-foreground"
                }`}
              >
                <Icon className={active ? "size-6" : "size-5"} aria-hidden />
                {item.label}
                {/* Colour is never the only cue: the icon grows, the label
                    bolds, and this rule appears. */}
                <span
                  aria-hidden
                  className={`h-0.5 w-4 rounded-full ${active ? "bg-primary-ink" : "bg-transparent"}`}
                />
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
