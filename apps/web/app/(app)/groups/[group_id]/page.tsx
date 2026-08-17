"use client";

import { use, useCallback, useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  apiFetch,
  ApiRequestError,
  type Group,
  type GroupMember,
  type Invite,
} from "@/lib/api";

/** Codes are only meaningful as links into this app. */
function inviteUrl(code: string): string {
  return `${window.location.origin}/join/${code}`;
}

export default function GroupPage({ params }: PageProps<"/groups/[group_id]">) {
  const { group_id: groupId } = use(params);

  const [group, setGroup] = useState<Group | null>(null);
  const [members, setMembers] = useState<GroupMember[]>([]);
  const [invites, setInvites] = useState<Invite[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const loadInvites = useCallback(
    (signal?: AbortSignal) =>
      apiFetch<Invite[]>(`/api/v1/groups/${groupId}/invites`, { signal }).then(setInvites),
    [groupId],
  );

  useEffect(() => {
    const controller = new AbortController();
    const options = { signal: controller.signal };

    Promise.all([
      apiFetch<Group>(`/api/v1/groups/${groupId}`, options),
      apiFetch<GroupMember[]>(`/api/v1/groups/${groupId}/members`, options),
      loadInvites(controller.signal),
    ])
      .then(([loadedGroup, loadedMembers]) => {
        setGroup(loadedGroup);
        setMembers(loadedMembers);
      })
      .catch((caught: unknown) => {
        if (controller.signal.aborted) return;
        setError(
          caught instanceof ApiRequestError && caught.status === 403
            ? "你不是這個群組的成員"
            : "讀取失敗，請重新整理",
        );
      });

    return () => controller.abort();
  }, [groupId, loadInvites]);

  async function handleInvite() {
    setError(null);
    setBusy(true);
    try {
      const created = await apiFetch<Invite>(`/api/v1/groups/${groupId}/invites`, {
        method: "POST",
      });
      setInvites((current) => [created, ...current]);
    } catch (caught) {
      setError(caught instanceof ApiRequestError ? caught.message : "無法連線，請稍後再試");
    } finally {
      setBusy(false);
    }
  }

  async function handleRevoke(inviteId: string) {
    setError(null);
    setBusy(true);
    try {
      await apiFetch<void>(`/api/v1/groups/${groupId}/invites/${inviteId}`, {
        method: "DELETE",
      });
      setInvites((current) => current.filter((invite) => invite.id !== inviteId));
    } catch (caught) {
      setError(caught instanceof ApiRequestError ? caught.message : "無法連線，請稍後再試");
    } finally {
      setBusy(false);
    }
  }

  if (error && !group) {
    return (
      <main className="flex flex-1 flex-col gap-4 p-6">
        <p role="alert" className="text-destructive text-sm">
          {error}
        </p>
      </main>
    );
  }

  return (
    <main className="flex flex-1 flex-col gap-6 p-6">
      <h1 className="text-2xl font-semibold">{group?.name ?? "載入中…"}</h1>

      <section className="flex flex-col gap-2">
        <h2 className="text-sm font-medium">成員</h2>
        <ul className="flex flex-col gap-1">
          {members.map((member) => (
            <li key={member.user_id} className="flex justify-between text-sm">
              {/* A member who joined before finishing onboarding has no name yet. */}
              <span>{member.display_name || "（尚未設定暱稱）"}</span>
              {member.role === "owner" ? (
                <span className="text-muted-foreground text-xs">管理者</span>
              ) : null}
            </li>
          ))}
        </ul>
      </section>

      <section className="flex flex-col gap-3 border-t pt-6">
        <h2 className="text-sm font-medium">邀請朋友</h2>
        <p className="text-muted-foreground text-xs">連結 7 天內有效，可以給多個人使用。</p>

        <Button onClick={handleInvite} disabled={busy}>
          {busy ? "處理中…" : "產生邀請連結"}
        </Button>

        {invites.length > 0 ? (
          <ul className="flex flex-col gap-3">
            {invites.map((invite) => (
              <li key={invite.id} className="flex flex-col gap-2">
                <Input
                  readOnly
                  value={inviteUrl(invite.code)}
                  aria-label="邀請連結"
                  onFocus={(event) => event.target.select()}
                />
                {/* Revoking is how a leaked link gets killed, so the owner
                    needs it on every invite, not just the one just created. */}
                {group?.is_owner ? (
                  <Button
                    variant="outline"
                    disabled={busy}
                    onClick={() => handleRevoke(invite.id)}
                  >
                    撤銷這個連結
                  </Button>
                ) : null}
              </li>
            ))}
          </ul>
        ) : null}

        {error ? (
          <p role="alert" className="text-destructive text-sm">
            {error}
          </p>
        ) : null}
      </section>
    </main>
  );
}
