"use client";

import { useRouter } from "next/navigation";
import { use, useEffect, useState } from "react";

import { apiFetch, ApiRequestError, type Group, type Profile } from "@/lib/api";

type State = "joining" | "failed";

/**
 * The invite landing page, and the only signed-out page that leads somewhere.
 *
 * Someone arriving from a friend's link may have no account at all, so this
 * sends them through registration and onboarding carrying the invite in
 * `next`, then finishes the join for them. Nobody has to find the group by
 * hand afterwards.
 */
export default function JoinPage({ params }: PageProps<"/join/[code]">) {
  const { code } = use(params);
  const router = useRouter();

  const [state, setState] = useState<State>("joining");
  const [message, setMessage] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    const back = `/join/${encodeURIComponent(code)}`;
    const options = { signal: controller.signal };

    async function join() {
      try {
        // Profile first: the join needs a session, and a user without a
        // profile has not finished onboarding yet.
        await apiFetch<Profile>("/api/v1/me/profile", options);
      } catch (caught) {
        if (controller.signal.aborted) return;
        if (caught instanceof ApiRequestError && caught.status === 401) {
          router.replace(`/register?next=${encodeURIComponent(back)}`);
          return;
        }
        if (caught instanceof ApiRequestError && caught.status === 404) {
          router.replace(`/onboarding?next=${encodeURIComponent(back)}`);
          return;
        }
        setState("failed");
        setMessage("無法連線，請稍後再試");
        return;
      }

      try {
        const group = await apiFetch<Group>("/api/v1/groups/join", {
          ...options,
          method: "POST",
          body: { code },
        });
        router.replace(`/groups/${group.id}`);
      } catch (caught) {
        if (controller.signal.aborted) return;
        setState("failed");
        setMessage(
          caught instanceof ApiRequestError && caught.status === 404
            ? "這個邀請已經失效，請向朋友要一份新的連結"
            : "無法連線，請稍後再試",
        );
      }
    }

    join();
    return () => controller.abort();
  }, [code, router]);

  return (
    <main className="flex flex-1 flex-col justify-center gap-3 p-6">
      <h1 className="text-2xl font-semibold">吃伴</h1>
      {state === "joining" ? (
        <p className="text-muted-foreground text-sm" role="status">
          加入群組中…
        </p>
      ) : (
        <p role="alert" className="text-destructive text-sm">
          {message}
        </p>
      )}
    </main>
  );
}
