"use client";

import { ChevronLeftIcon, MessageCircleIcon, UsersIcon } from "lucide-react";
import Link from "next/link";
import { use, useCallback, useEffect, useState } from "react";

import { Avatar } from "@/components/avatar";
import { ChatRoom } from "@/components/chat-room";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Message } from "@/components/ui/message";
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
  const [showDetails, setShowDetails] = useState(false);

  const loadInvites = useCallback(
    (signal?: AbortSignal) =>
      apiFetch<Invite[]>(`/api/v1/groups/${groupId}/invites`, { signal }).then(
        setInvites,
      ),
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
      const created = await apiFetch<Invite>(
        `/api/v1/groups/${groupId}/invites`,
        {
          method: "POST",
        },
      );
      setInvites((current) => [created, ...current]);
    } catch (caught) {
      setError(
        caught instanceof ApiRequestError
          ? caught.message
          : "無法連線，請稍後再試",
      );
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
      setInvites((current) =>
        current.filter((invite) => invite.id !== inviteId),
      );
    } catch (caught) {
      setError(
        caught instanceof ApiRequestError
          ? caught.message
          : "無法連線，請稍後再試",
      );
    } finally {
      setBusy(false);
    }
  }

  if (error && !group) {
    return (
      <main className="flex flex-1 flex-col gap-4 p-6">
        <Message tone="error">{error}</Message>
      </main>
    );
  }

  return (
    <main className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <header className="bg-background/92 border-border flex items-center gap-3 border-b px-4 py-3 backdrop-blur">
        <Link
          href="/groups"
          aria-label="回到群組"
          className={buttonVariants({ variant: "outline", size: "icon" })}
        >
          <ChevronLeftIcon aria-hidden />
        </Link>
        <div className="flex flex-1 flex-col gap-0.5">
          <h1 className="page-title-bar">{group?.name ?? "載入中…"}</h1>
          <p className="text-muted-foreground text-xs">{members.length} 人</p>
        </div>
        <Button
          variant={showDetails ? "secondary" : "outline"}
          size="icon"
          aria-expanded={showDetails}
          aria-label={showDetails ? "回到聊天" : "成員與邀請"}
          onClick={() => setShowDetails((shown) => !shown)}
        >
          {showDetails ? <MessageCircleIcon aria-hidden /> : <UsersIcon aria-hidden />}
        </Button>
      </header>

      {/* The conversation is the page; membership and invites are settings
          you visit occasionally, so they take the screen only when asked for. */}
      {!showDetails ? (
        // key: switching groups must start a fresh conversation, not reuse
        // this one's refs and in-flight requests under a new id.
        <ChatRoom key={groupId} groupId={groupId} members={members} />
      ) : (
        <div className="flex flex-1 flex-col gap-4 overflow-y-auto px-5 py-6">
          <section className="card-surface flex flex-col gap-3">
            <h2 className="text-muted-foreground text-xs tracking-[0.06em]">成員</h2>
            <ul className="flex flex-col gap-1">
              {members.map((member) => (
                <li
                  key={member.user_id}
                  className="flex items-center gap-3 text-sm"
                >
                  <Avatar
                    mediaId={member.avatar_media_id}
                    displayName={member.display_name || "這位成員"}
                    className="size-8"
                  />
                  {/* A member who joined before finishing onboarding has no name yet. */}
                  <span className="flex-1">
                    {member.display_name || "（尚未設定暱稱）"}
                  </span>
                  {member.role === "owner" ? (
                    <span className="text-muted-foreground text-xs">
                      管理者
                    </span>
                  ) : null}
                </li>
              ))}
            </ul>
          </section>

          <section className="card-surface flex flex-col gap-3">
            <h2 className="text-muted-foreground text-xs tracking-[0.06em]">邀請朋友</h2>
            <p className="text-muted-foreground text-xs">
              連結 7 天內有效，可以給多個人使用。
            </p>

            {/* Disabled until the initial load lands: a create that resolves
            before it would otherwise be overwritten by the slower list fetch. */}
            <Button onClick={handleInvite} loading={busy} disabled={!group}>
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

            {error ? <Message tone="error">{error}</Message> : null}
          </section>
        </div>
      )}
    </main>
  );
}
