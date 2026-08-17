"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

const items = [
  { href: "/today", label: "今日" },
  { href: "/record", label: "記錄" },
  { href: "/groups", label: "群組" },
  { href: "/profile", label: "我的" },
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
      className="bg-background sticky bottom-0 border-t pb-[env(safe-area-inset-bottom)]"
    >
      <ul className="grid grid-cols-4">
        {items.map((item) => {
          const active = pathname === item.href;
          return (
            <li key={item.href}>
              <Link
                href={item.href}
                aria-current={active ? "page" : undefined}
                className={`flex h-14 items-center justify-center text-sm ${
                  active ? "text-foreground font-medium" : "text-muted-foreground"
                }`}
              >
                {item.label}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
