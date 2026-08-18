"use client";

import { useRouter } from "next/navigation";
import { use, useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Message } from "@/components/ui/message";
import {
  apiFetch,
  apiUrl,
  ApiRequestError,
  mealTypeLabel,
  type Group,
  type Meal,
} from "@/lib/api";

const mealTypes = [
  { value: "", label: "不指定" },
  { value: "breakfast", label: "早餐" },
  { value: "lunch", label: "午餐" },
  { value: "dinner", label: "晚餐" },
  { value: "snack", label: "點心" },
  { value: "other", label: "其他" },
] as const;

export default function MealPage({ params }: PageProps<"/meals/[meal_id]">) {
  const { meal_id: mealId } = use(params);
  const router = useRouter();

  const [meal, setMeal] = useState<Meal | null>(null);
  const [mealType, setMealType] = useState("");
  const [description, setDescription] = useState("");
  const [eatenAt, setEatenAt] = useState("");
  const [status, setStatus] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  // Which groups can currently see this meal. Sharing is meant to be
  // revocable, so the owner has to be able to see what they gave away.
  const [sharedWith, setSharedWith] = useState<string[] | null>(null);
  const [sharesFailed, setSharesFailed] = useState(false);
  const [groups, setGroups] = useState<Group[]>([]);

  useEffect(() => {
    const controller = new AbortController();

    apiFetch<Meal>(`/api/v1/meals/${mealId}`, { signal: controller.signal })
      .then((loaded) => {
        setMeal(loaded);
        setMealType(loaded.meal_type ?? "");
        setDescription(loaded.description ?? "");
        // Already wall-clock in the profile zone, which is the shape the
        // datetime-local input wants — no conversion here.
        setEatenAt(loaded.eaten_at_local);
      })
      .catch((caught: unknown) => {
        if (controller.signal.aborted) return;
        setError(
          caught instanceof ApiRequestError && caught.status === 404
            ? "找不到這筆紀錄"
            : "讀取失敗，請重新整理",
        );
      });

    // Two requests with independent failure modes, so they are kept apart: if
    // the group list blips, an unknown share list must not be drawn as an
    // empty one — that would hide real shares and the only way to revoke them.
    apiFetch<{ group_ids: string[] }>(`/api/v1/meals/${mealId}/shares`, {
      signal: controller.signal,
    })
      .then((shares) => setSharedWith(shares.group_ids))
      .catch((caught: unknown) => {
        if (controller.signal.aborted) return;
        // Owner-only, so 404 here just means this meal is not the reader's
        // and there is no sharing section to draw.
        if (caught instanceof ApiRequestError && caught.status === 404) return;
        setSharesFailed(true);
      });

    apiFetch<Group[]>("/api/v1/groups", { signal: controller.signal })
      .then(setGroups)
      .catch(() => {});

    return () => controller.abort();
  }, [mealId]);

  async function handleSave(event: React.SyntheticEvent) {
    event.preventDefault();
    setError(null);
    setStatus(null);
    setBusy(true);

    try {
      const saved = await apiFetch<Meal>(`/api/v1/meals/${mealId}`, {
        method: "PATCH",
        body: {
          meal_type: mealType,
          description,
          // Wall-clock time goes back as-is; the server resolves it against
          // the profile timezone.
          eaten_at_local: eatenAt,
        },
      });
      setMeal(saved);
      setEatenAt(saved.eaten_at_local);
      setStatus("已儲存");
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

  // One control per group, both directions. A meal that failed to share at
  // publish time is shared from here, which is what makes the recovery path
  // real rather than a one-shot chance on the record page.
  async function handleToggleShare(groupID: string, shared: boolean) {
    setError(null);
    setStatus(null);
    setBusy(true);
    try {
      if (shared) {
        await apiFetch<void>(`/api/v1/meals/${mealId}/shares/${groupID}`, {
          method: "DELETE",
        });
        setSharedWith((current) =>
          (current ?? []).filter((id) => id !== groupID),
        );
        setStatus("已取消分享");
      } else {
        await apiFetch<void>(`/api/v1/meals/${mealId}/shares`, {
          method: "POST",
          body: { group_ids: [groupID] },
        });
        setSharedWith((current) => [...(current ?? []), groupID]);
        setStatus("已分享");
      }
    } catch {
      setError(shared ? "取消分享失敗，請稍後再試" : "分享失敗，請稍後再試");
    } finally {
      setBusy(false);
    }
  }

  async function handleDelete() {
    setError(null);
    setBusy(true);

    try {
      await apiFetch<void>(`/api/v1/meals/${mealId}`, { method: "DELETE" });
      router.replace("/today");
    } catch (caught) {
      setError(
        caught instanceof ApiRequestError
          ? caught.message
          : "無法連線，請稍後再試",
      );
      setBusy(false);
    }
  }

  if (!meal) {
    return (
      <main className="flex flex-1 flex-col gap-4 p-6">
        <Message tone={error ? "error" : "info"}>{error ?? "載入中…"}</Message>
      </main>
    );
  }

  return (
    <main className="flex flex-1 flex-col gap-5 p-6">
      <h1>這一餐</h1>

      <ul className="grid grid-cols-2 gap-2">
        {meal.photo_ids.map((id) => (
          <li key={id}>
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src={apiUrl(`/api/v1/meal-images/${id}`)}
              alt="這一餐的照片"
              className="aspect-square w-full rounded-2xl object-cover"
            />
          </li>
        ))}
      </ul>

      {/* A shared meal is readable by the group but editable only by whoever
          recorded it. Showing everyone the form would offer a save that the
          API is always going to refuse. */}
      {!meal.is_owner ? (
        <dl className="flex flex-col gap-3">
          <div className="flex gap-3 text-sm">
            <dt className="text-muted-foreground w-20 shrink-0">用餐時間</dt>
            <dd>{meal.eaten_at_local.replace("T", " ")}</dd>
          </div>
          {mealTypeLabel(meal.meal_type) ? (
            <div className="flex gap-3 text-sm">
              <dt className="text-muted-foreground w-20 shrink-0">餐別</dt>
              <dd>{mealTypeLabel(meal.meal_type)}</dd>
            </div>
          ) : null}
          {meal.description ? (
            <div className="flex gap-3 text-sm">
              <dt className="text-muted-foreground w-20 shrink-0">備註</dt>
              <dd className="whitespace-pre-wrap">{meal.description}</dd>
            </div>
          ) : null}
        </dl>
      ) : null}

      {/* Photos are fixed once published: replacing them would need the same
          all-or-nothing upload handling as creating a meal, and nothing in
          V0.1 asks for it. */}
      {meal.is_owner ? (
        <form onSubmit={handleSave} className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="eaten-at">用餐時間</Label>
            <Input
              id="eaten-at"
              type="datetime-local"
              required
              value={eatenAt}
              onChange={(event) => setEatenAt(event.target.value)}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="meal-type">餐別</Label>
            <select
              id="meal-type"
              className="border-input bg-card h-11 rounded-2xl border px-4 text-sm"
              value={mealType}
              onChange={(event) => setMealType(event.target.value)}
            >
              {mealTypes.map((type) => (
                <option key={type.value} value={type.value}>
                  {type.label}
                </option>
              ))}
            </select>
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="description">備註</Label>
            <Input
              id="description"
              maxLength={500}
              value={description}
              onChange={(event) => setDescription(event.target.value)}
            />
          </div>

          {error ? <Message tone="error">{error}</Message> : null}
          {status ? <Message tone="success">{status}</Message> : null}

          <Button type="submit" loading={busy}>
            儲存
          </Button>
        </form>
      ) : null}

      {/* Sharing has to be revocable from the product, not only from the API:
          a meal shared by mistake is exactly the case this exists for. */}
      {meal.is_owner ? (
        <section className="flex flex-col gap-2 border-t pt-5">
          <h2 className="text-sm font-medium">分享</h2>
          {sharesFailed ? (
            <Message tone="error">
              無法載入分享狀態，重新整理後再試——在確定目前分享給誰之前，這裡不會顯示任何按鈕。
            </Message>
          ) : sharedWith === null ? (
            <p className="text-muted-foreground text-xs">載入中…</p>
          ) : groups.length === 0 ? (
            <p className="text-muted-foreground text-xs">
              還沒有加入任何群組，所以只有你看得到這一餐。
            </p>
          ) : (
            <ul className="flex flex-col gap-2">
              {groups.map((group) => {
                const shared = sharedWith.includes(group.id);
                return (
                  <li
                    key={group.id}
                    className="flex items-center gap-3 text-sm"
                  >
                    <span className="flex-1">{group.name}</span>
                    <Button
                      variant={shared ? "outline" : "default"}
                      size="sm"
                      disabled={busy}
                      onClick={() => handleToggleShare(group.id, shared)}
                    >
                      {shared ? "取消分享" : "分享"}
                    </Button>
                  </li>
                );
              })}
            </ul>
          )}
          <p className="text-muted-foreground text-xs">
            取消後該群組就讀不到這一餐與它的照片，聊天室裡的留言會留著。
          </p>
        </section>
      ) : null}

      {meal.is_owner ? (
        <div className="flex flex-col gap-2 border-t pt-5">
          {confirmingDelete ? (
            <>
              <Message tone="error">刪除後就不會出現在你的紀錄裡。</Message>
              <div className="grid grid-cols-2 gap-3">
                <Button
                  variant="outline"
                  onClick={() => setConfirmingDelete(false)}
                >
                  取消
                </Button>
                <Button
                  variant="destructive"
                  loading={busy}
                  onClick={handleDelete}
                >
                  確認刪除
                </Button>
              </div>
            </>
          ) : (
            <Button
              variant="destructive"
              onClick={() => setConfirmingDelete(true)}
            >
              刪除這筆紀錄
            </Button>
          )}
        </div>
      ) : null}
    </main>
  );
}
