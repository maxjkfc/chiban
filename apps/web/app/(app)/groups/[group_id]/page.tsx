"use client";

import { use, useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  apiFetch,
  ApiRequestError,
  type Group,
  type GroupMember,
  type Invite,
} from "@/lib/api";

export default function GroupPage({ params }: PageProps<"/groups/[group_id]">) {
  const { group_id: groupId } = use(params);

  const [group, setGroup] = useState<Group | null>(null);
  const [members, setMembers] = useState<GroupMember[]>([]);
  const [inviteUrl, setInviteUrl] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [inviting, setInviting] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    const options = { signal: controller.signal };

    Promise.all([
      apiFetch<Group>(`/api/v1/groups/${groupId}`, options),
      apiFetch<GroupMember[]>(`/api/v1/groups/${groupId}/members`, options),
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
  }, [groupId]);

  async function handleInvite() {
    setError(null);
    setInviting(true);

    try {
      const invite = await apiFetch<Invite>(`/api/v1/groups/${groupId}/invites`, {
        method: "POST",
      });
      // The code is only ever meaningful as a link into this app; the API
      // never hands out anything the browser has to assemble itself.
      setInviteUrl(`${window.location.origin}/join/${invite.code}`);
    } catch (caught) {
      setError(
        caught instanceof ApiRequestError ? caught.message : "無法連線，請稍後再試",
      );
    } finally {
      setInviting(false);
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
              <span>{member.display_name}</span>
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
        <Button onClick={handleInvite} disabled={inviting}>
          {inviting ? "產生中…" : "產生邀請連結"}
        </Button>
        {inviteUrl ? (
          <Input readOnly value={inviteUrl} aria-label="邀請連結" onFocus={(e) => e.target.select()} />
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
