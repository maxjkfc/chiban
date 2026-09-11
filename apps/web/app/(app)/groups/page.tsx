"use client";

import {
  ChevronRightIcon,
  PinIcon,
  PlusIcon,
  XIcon,
} from "lucide-react";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";

import { Tape, tapePlacement, tapeTone, tiltClass } from "@/components/tape";
import { UnreadBadge } from "@/components/unread-badge";
import { useUnreadGroups } from "@/components/unread-provider";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Message } from "@/components/ui/message";
import {
  apiFetch,
  ApiRequestError,
  pinGroup,
  unpinGroup,
  type Group,
} from "@/lib/api";
import { pinnedGroups } from "@/lib/groups-page-state.mts";
import { cn } from "@/lib/utils";
import { groupHasUnread } from "@/lib/unread";

export default function GroupsPage() {
  const {
    groups,
    error: groupsError,
    addGroup,
    replaceGroup,
  } = useUnreadGroups();
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [pinningGroupId, setPinningGroupId] = useState<string | null>(null);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const createTriggerRef = useRef<HTMLButtonElement>(null);
  const nameInputRef = useRef<HTMLInputElement>(null);

  function closeCreateModal() {
    setShowCreateModal(false);
    setError(null);
    createTriggerRef.current?.focus();
  }

  useEffect(() => {
    if (!showCreateModal) return;

    function handleEscape(event: KeyboardEvent) {
      if (event.key === "Escape") closeCreateModal();
    }

    document.addEventListener("keydown", handleEscape);
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    nameInputRef.current?.focus();
    return () => {
      document.removeEventListener("keydown", handleEscape);
      document.body.style.overflow = previousOverflow;
    };
  }, [showCreateModal]);

  async function handleCreate(event: React.FormEvent<HTMLFormElement>) {
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
      setShowCreateModal(false);
      createTriggerRef.current?.focus();
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

  async function handlePinToggle(group: Group) {
    const nextPinned = group.pinned !== true;
    const updated = { ...group, pinned: nextPinned };
    setError(null);
    replaceGroup(updated);
    setPinningGroupId(group.id);

    try {
      if (nextPinned) {
        await pinGroup(group.id);
      } else {
        await unpinGroup(group.id);
      }
    } catch (caught) {
      replaceGroup(group);
      setError(
        caught instanceof ApiRequestError
          ? caught.message
          : "無法連線，請稍後再試",
      );
    } finally {
      setPinningGroupId(null);
    }
  }

  const quickGroups = groups ? pinnedGroups(groups) : [];

  return (
    <main className="flex flex-1 flex-col gap-5 px-5 pt-7 pb-5">
      <header className="flex items-start gap-4">
        <div className="flex flex-1 flex-col gap-1">
          <h1>群組</h1>
          <p className="text-muted-foreground text-sm">
            只有被邀請的人看得到裡面的餐。
          </p>
        </div>
        <Button
          ref={createTriggerRef}
          variant="outline"
          size="icon"
          aria-label="建立群組"
          title="建立群組"
          onClick={() => {
            setError(null);
            setShowCreateModal(true);
          }}
        >
          <PlusIcon aria-hidden />
        </Button>
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
        <>
          {quickGroups.length > 0 ? (
            <section className="flex flex-col gap-3" aria-labelledby="pinned-groups-title">
              <div className="flex items-center gap-2">
                <PinIcon className="text-primary-ink size-4" aria-hidden />
                <h2 id="pinned-groups-title" className="text-sm">
                  已釘選頻道
                </h2>
              </div>
              <ul className="-mx-1 flex snap-x gap-3 overflow-x-auto px-1 pb-2">
                {quickGroups.map((group) => (
                  <li key={group.id} className="min-w-40 snap-start">
                    <Link
                      href={`/groups/${group.id}`}
                      className="card-surface flex min-h-20 items-center gap-2 p-3 transition-transform active:scale-[0.98]"
                    >
                      <span className="bg-secondary flex size-9 shrink-0 items-center justify-center rounded-full">
                        <PinIcon className="text-secondary-foreground size-4" aria-hidden />
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-sm font-bold">
                          {group.name}
                        </span>
                        <span className="text-muted-foreground text-xs">進入聊天</span>
                      </span>
                      <ChevronRightIcon className="text-muted-foreground size-4" aria-hidden />
                    </Link>
                  </li>
                ))}
              </ul>
            </section>
          ) : null}

          <ul className="flex flex-col gap-4">
            {groups.map((group, index) => (
              <li key={group.id}>
                <div
                  className={cn(
                    "polaroid relative flex items-center gap-2 p-2 transition-transform",
                    tiltClass(index),
                  )}
                >
                  <Tape tone={tapeTone(index)} className={tapePlacement(index)} />
                  <Link
                    href={`/groups/${group.id}`}
                    className="flex min-w-0 flex-1 items-center gap-3 p-2 active:scale-[0.99]"
                  >
                    <span className="font-heading min-w-0 flex-1 truncate text-lg font-black">
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
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={group.pinned === true ? `取消釘選${group.name}` : `釘選${group.name}`}
                    aria-pressed={group.pinned === true}
                    loading={pinningGroupId === group.id}
                    disabled={pinningGroupId !== null && pinningGroupId !== group.id}
                    onClick={() => void handlePinToggle(group)}
                  >
                    <PinIcon aria-hidden />
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        </>
      )}

      {error && groups !== null ? <Message tone="error">{error}</Message> : null}

      {showCreateModal ? (
        <div className="fixed inset-0 z-50 flex items-end justify-center p-4 sm:items-center">
          <button
            type="button"
            aria-label="關閉建立群組視窗"
            className="absolute inset-0 cursor-default bg-black/30"
            onClick={closeCreateModal}
          />
          <div
            role="dialog"
            aria-modal="true"
            aria-labelledby="create-group-title"
            className="bg-card relative w-full max-w-md rounded-2xl border p-5 shadow-2xl"
          >
            <div className="mb-5 flex items-start gap-3">
              <div className="flex-1">
                <h2 id="create-group-title" className="text-xl">
                  建立群組
                </h2>
                <p className="text-muted-foreground mt-1 text-sm">
                  建立後就可以邀請朋友一起記錄。
                </p>
              </div>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="關閉建立群組視窗"
                onClick={closeCreateModal}
              >
                <XIcon aria-hidden />
              </Button>
            </div>
            <form onSubmit={handleCreate} className="flex flex-col gap-4">
              <div className="flex flex-col gap-2">
                <Label htmlFor="group-name">群組名稱</Label>
                <Input
                  ref={nameInputRef}
                  id="group-name"
                  variant="ruled"
                  required
                  maxLength={50}
                  placeholder="例如：週五宵夜"
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                />
              </div>
              {error ? <Message tone="error">{error}</Message> : null}
              <Button type="submit" loading={submitting}>
                <PlusIcon aria-hidden />
                {submitting ? "建立中…" : "建立群組"}
              </Button>
            </form>
          </div>
        </div>
      ) : null}
    </main>
  );
}