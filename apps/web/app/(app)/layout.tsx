import { HomeIcon } from "lucide-react";
import Link from "next/link";

import { AuthGate } from "@/components/auth-gate";
import { BottomNav } from "@/components/bottom-nav";
import { NotificationBanner } from "@/components/notification-banner";
import { buttonVariants } from "@/components/ui/button";
import { UnreadProvider } from "@/components/unread-provider";
import { appShellHomeLink } from "@/lib/navigation.mts";
import { cn } from "@/lib/utils";

/** Shell for signed-in pages: gate first, then content, then navigation. */
export default function AppLayout({ children }: LayoutProps<"/">) {
  return (
    <AuthGate>
      <NotificationBanner />
      {/* The content scrolls inside the shell rather than growing the page:
          a document that grew would leave the chat's message list no height to
          scroll within, pushing the composer off the screen. */}
      <UnreadProvider>
        <div className="flex min-h-0 flex-1 flex-col">
          {/* Keep this outside the scrolling content so every authenticated
              destination has a persistent route back to Today. */}
          <header className="bg-background/92 border-border flex min-h-12 shrink-0 items-center border-b px-4 pt-[env(safe-area-inset-top)] backdrop-blur">
            <Link
              href={appShellHomeLink.href}
              aria-label={appShellHomeLink.ariaLabel}
              className={cn(
                buttonVariants({ variant: "ghost" }),
                "text-primary-ink",
              )}
            >
              <HomeIcon aria-hidden />
              <span>{appShellHomeLink.label}</span>
            </Link>
          </header>
          <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
            {children}
          </div>
          <BottomNav />
        </div>
      </UnreadProvider>
    </AuthGate>
  );
}
