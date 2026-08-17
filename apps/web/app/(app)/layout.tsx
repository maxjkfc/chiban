import { AuthGate } from "@/components/auth-gate";
import { BottomNav } from "@/components/bottom-nav";

/** Shell for signed-in pages: gate first, then content, then navigation. */
export default function AppLayout({ children }: LayoutProps<"/">) {
  return (
    <AuthGate>
      <div className="flex min-h-dvh flex-1 flex-col">
        <div className="flex flex-1 flex-col">{children}</div>
        <BottomNav />
      </div>
    </AuthGate>
  );
}
