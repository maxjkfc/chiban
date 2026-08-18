"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { Message } from "@/components/ui/message";
import { apiFetch, ApiRequestError, type Profile } from "@/lib/api";

type State = "checking" | "allowed" | "redirecting";

/**
 * Client-side gate for signed-in pages.
 *
 * This cannot be Next middleware: the session cookie belongs to the API's
 * origin, so the Next server never sees it. The gate is a convenience for the
 * user, never a security boundary — every protected operation is authorised
 * again by the Go API.
 *
 * A missing profile means onboarding was never finished, so the user is sent
 * there rather than into an app that cannot render their name.
 */
export function AuthGate({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const [state, setState] = useState<State>("checking");

  useEffect(() => {
    const controller = new AbortController();

    apiFetch<Profile>("/api/v1/me/profile", { signal: controller.signal })
      .then(() => setState("allowed"))
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        if (error instanceof ApiRequestError && error.status === 401) {
          setState("redirecting");
          router.replace("/login");
          return;
        }
        if (error instanceof ApiRequestError && error.status === 404) {
          setState("redirecting");
          router.replace("/onboarding");
          return;
        }
        // Anything else (API down, network drop) should not look like a
        // logout; say so and let the user retry.
        setState("allowed");
      });

    return () => controller.abort();
  }, [router]);

  if (state !== "allowed") {
    return (
      <div className="flex flex-1 items-center justify-center p-6">
        <Message>載入中…</Message>
      </div>
    );
  }
  return <>{children}</>;
}
