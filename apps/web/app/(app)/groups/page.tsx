"use client";

import { ChevronRightIcon, PlusIcon } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { Tape, tapePlacement, tapeTone, tiltClass } from "@/components/tape";
import { UnreadBadge } from "@/components/unread-badge";
import { useUnreadGroups } from "@/components/unread-provider";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Message } from "@/components/ui/message";
import { apiFetch, ApiRequestError, type Group } from "@/lib/api";
import { cn } from "@/lib/utils";
import { groupHasUnread } from "@/lib/unread";

export default function GroupsPage() {
  const { groups, error: groupsError, addGroup } = useUnreadGroups();
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleCreate(event: React.SyntheticEvent) {
    event.preventDefault();
    setError(null);
    setSubmitting(true);

    try {
      const created = await apiFetch<Group>("/api/v1/groups", {
        method: "POST",
        body: { name },
      });
      addGroup(created);
      setName("");
    } catch (caught) {
      setError(
        caught instanceof ApiRequestError
          ? caught.message
          : "無法連線，請稍後再試",
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="flex flex-1 flex-col gap-5 px-5 pt-7 pb-5">
      <header className="flex flex-col gap-1">
        <h1>群組</h1>
        <p className="text-muted-foreground text-sm">
          只有被邀請的人看得到裡面的餐。
        </p>
      </header>

      {groups === null ? (
        <Message tone={groupsError ? "error" : "info"}>
          {groupsError ? "讀取失敗，請重新整理" : "載入中…"}
        </Message>
      ) : groups.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          還沒有群組。建立一個，再把邀請連結傳給朋友。
        </p>
      ) : (
        <ul className="flex flex-col gap-4">
          {groups.map((group, index) => (
            <li key={group.id}>
              {/* An index card with a strip of tape, not a table row: the
                  group is a place you go, and it should look like an object. */}
              <Link
                href={`/groups/${group.id}`}
                className={cn(
                  "polaroid relative flex items-center gap-3 p-4 transition-transform active:scale-[0.99]",
                  tiltClass(index),
                )}
              >
                <Tape tone={tapeTone(index)} className={tapePlacement(index)} />
                <span className="font-heading flex-1 text-lg font-black">
                  {group.name}
                </span>
                {groupHasUnread(group) ? (
                  <UnreadBadge
                    count={group.unread_count ?? 1}
                    showCount
                    label={`${group.name}有未讀訊息`}
                  />
                ) : null}
                {group.is_owner ? (
                  <span className="bg-accent text-accent-foreground rounded-full px-2.5 py-1 text-xs font-bold">
                    管理者
                  </span>
                ) : null}
                <ChevronRightIcon
                  className="text-muted-foreground size-4"
                  aria-hidden
                />
              </Link>
            </li>
          ))}
        </ul>
      )}

      <form onSubmit={handleCreate} className="card-surface flex flex-col gap-3">
        <Label htmlFor="group-name" className="text-muted-foreground text-xs tracking-[0.06em]">
          開一個新的
        </Label>
        <Input
          id="group-name"
          variant="ruled"
          required
          maxLength={50}
          placeholder="例如：週五宵夜"
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
        {error && groups !== null ? (
          <Message tone="error">{error}</Message>
        ) : null}
        <Button type="submit" loading={submitting}>
          <PlusIcon aria-hidden />
          {submitting ? "建立中…" : "建立群組"}
        </Button>
      </form>
    </main>
  );
}
