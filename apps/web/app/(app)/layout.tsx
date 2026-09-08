import { AuthGate } from "@/components/auth-gate";
import { BottomNav } from "@/components/bottom-nav";
import { NotificationBanner } from "@/components/notification-banner";
import { UnreadProvider } from "@/components/unread-provider";

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
          <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
            {children}
          </div>
          <BottomNav />
        </div>
      </UnreadProvider>
    </AuthGate>
  );
}
