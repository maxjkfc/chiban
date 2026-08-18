"use client";

import Link from "next/link";
import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Message } from "@/components/ui/message";
import { apiFetch, ApiRequestError, type Group } from "@/lib/api";

export default function GroupsPage() {
  const [groups, setGroups] = useState<Group[] | null>(null);
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    const controller = new AbortController();

    apiFetch<Group[]>("/api/v1/groups", { signal: controller.signal })
      .then(setGroups)
      .catch(() => {
        if (!controller.signal.aborted) setError("讀取失敗，請重新整理");
      });

    return () => controller.abort();
  }, []);

  async function handleCreate(event: React.SyntheticEvent) {
    event.preventDefault();
    setError(null);
    setSubmitting(true);

    try {
      const created = await apiFetch<Group>("/api/v1/groups", {
        method: "POST",
        body: { name },
      });
      setGroups((current) => [...(current ?? []), created]);
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
    <main className="flex flex-1 flex-col gap-6 p-6">
      <h1>群組</h1>

      {groups === null ? (
        <Message tone={error ? "error" : "info"}>{error ?? "載入中…"}</Message>
      ) : groups.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          還沒有群組。建立一個，再把邀請連結傳給朋友。
        </p>
      ) : (
        <ul className="flex flex-col gap-2">
          {groups.map((group) => (
            <li key={group.id}>
              <Link
                href={`/groups/${group.id}`}
                className="card-surface flex items-center justify-between font-semibold"
              >
                <span>{group.name}</span>
                {group.is_owner ? (
                  <span className="bg-accent text-accent-foreground rounded-full px-2.5 py-1 text-xs">
                    管理者
                  </span>
                ) : null}
              </Link>
            </li>
          ))}
        </ul>
      )}

      <form
        onSubmit={handleCreate}
        className="flex flex-col gap-3 border-t pt-6"
      >
        <Label htmlFor="group-name">建立群組</Label>
        <Input
          id="group-name"
          required
          maxLength={50}
          placeholder="例如：午餐團"
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
        {error && groups !== null ? (
          <Message tone="error">{error}</Message>
        ) : null}
        <Button type="submit" loading={submitting}>
          {submitting ? "建立中…" : "建立"}
        </Button>
      </form>
    </main>
  );
}
