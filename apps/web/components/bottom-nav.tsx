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
 */
export function BottomNav() {
  const pathname = usePathname();

  return (
    <nav
      aria-label="主要導航"
      className="bg-card/90 sticky bottom-0 border-t backdrop-blur pb-[env(safe-area-inset-bottom)]"
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
                className={`my-1.5 flex h-14 w-full max-w-20 flex-col items-center justify-center gap-1 rounded-2xl text-xs transition-colors ${
                  active
                    ? "bg-primary/10 text-primary font-semibold"
                    : "text-muted-foreground"
                }`}
              >
                <Icon className={active ? "size-6" : "size-5"} aria-hidden />
                {item.label}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
