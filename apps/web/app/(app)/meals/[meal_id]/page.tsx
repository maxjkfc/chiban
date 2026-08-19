"use client";

import { CheckIcon, ChevronLeftIcon, Trash2Icon } from "lucide-react";
import { useRouter } from "next/navigation";
import { use, useEffect, useState } from "react";

import { Tape } from "@/components/tape";
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
import { cn } from "@/lib/utils";

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
  const [groupsFailed, setGroupsFailed] = useState(false);
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
      .catch(() => {
        if (!controller.signal.aborted) setGroupsFailed(true);
      });

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

  // The union of "groups I am in" and "groups this meal is shared with".
  // Leaving a group does not revoke the shares you made into it, so a share
  // can outlive the membership — and it still has to be visible and
  // revocable, which a list built from memberships alone would hide.
  const shareRows = [
    ...new Set([...groups.map((group) => group.id), ...(sharedWith ?? [])]),
  ].map((id) => ({
    id,
    name:
      groups.find((group) => group.id === id)?.name ?? "某個群組（你已離開）",
    shared: (sharedWith ?? []).includes(id),
  }));

  if (!meal) {
    return (
      <main className="flex flex-1 flex-col gap-4 px-5 pt-7 pb-5">
        <Message tone={error ? "error" : "info"}>{error ?? "載入中…"}</Message>
      </main>
    );
  }

  const single = meal.photo_ids.length === 1;

  return (
    <main className="flex flex-1 flex-col gap-4 px-5 pt-7 pb-5">
      <header className="flex items-center gap-3">
        <Button variant="outline" size="icon" onClick={() => router.back()} aria-label="回上一頁">
          <ChevronLeftIcon aria-hidden />
        </Button>
        <h1 className="text-2xl">這一餐</h1>
      </header>

      <div className="polaroid relative -rotate-[1deg]">
        <Tape tone={1} className="-top-2.5 left-1/2 w-[74px] -translate-x-[37px] -rotate-[3deg]" />
        <ul className={cn("grid gap-1.5", single ? "grid-cols-1" : "grid-cols-2")}>
          {meal.photo_ids.map((id) => (
            <li key={id}>
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={apiUrl(`/api/v1/meal-images/${id}`)}
                alt="這一餐的照片"
                className={cn(
                  "w-full rounded-[2px] object-cover",
                  single ? "h-60" : "aspect-square",
                )}
              />
            </li>
          ))}
        </ul>
        {/* The caption belongs on the card whoever is reading it. The owner's
            editable copy sits below; this is what the meal says. */}
        <div className="mt-2.5 flex items-baseline justify-between gap-2">
          <span className="font-heading text-base font-black">
            {mealTypeLabel(meal.meal_type) ?? "這一餐"}
          </span>
          <span className="text-muted-foreground text-xs">
            {meal.eaten_at_local.replace("T", " ")}
          </span>
        </div>
        {meal.description ? (
          <p className="mt-1 text-sm leading-relaxed whitespace-pre-wrap">
            {meal.description}
          </p>
        ) : null}
      </div>

      {/* A shared meal is readable by the group but editable only by whoever
          recorded it. Showing everyone the form would offer a save that the
          API is always going to refuse. */}
      {meal.is_owner ? (
        <form onSubmit={handleSave} className="card-surface flex flex-col gap-4">
          <h2 className="text-muted-foreground text-xs tracking-[0.06em]">
            改一下
          </h2>

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

          <fieldset className="flex flex-col gap-2">
            <legend className="text-sm font-medium">餐別</legend>
            <ul className="flex flex-wrap gap-2">
              {mealTypes.map((type) => {
                const chosen = mealType === type.value;
                return (
                  <li key={type.value}>
                    <button
                      type="button"
                      aria-pressed={chosen}
                      onClick={() => setMealType(type.value)}
                      className={cn(
                        "h-10 cursor-pointer rounded-full px-4 text-sm",
                        chosen
                          ? "bg-primary text-primary-foreground font-bold"
                          : "bg-card border-input text-muted-foreground border",
                      )}
                    >
                      {type.label}
                    </button>
                  </li>
                );
              })}
            </ul>
          </fieldset>

          <div className="flex flex-col gap-2">
            <Label htmlFor="description">備註</Label>
            <Input
              id="description"
              variant="ruled"
              maxLength={500}
              placeholder="寫點什麼…"
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
      ) : error ? (
        <Message tone="error">{error}</Message>
      ) : null}

      {/* Sharing has to be revocable from the product, not only from the API:
          a meal shared by mistake is exactly the case this exists for. */}
      {meal.is_owner ? (
        <section className="card-surface flex flex-col gap-3">
          <h2 className="text-muted-foreground text-xs tracking-[0.06em]">
            分享到
          </h2>
          {sharesFailed || groupsFailed ? (
            <Message tone="error">
              無法載入分享狀態，重新整理後再試——在確定目前分享給誰之前，這裡不會顯示任何按鈕。
            </Message>
          ) : sharedWith === null ? (
            <p className="text-muted-foreground text-xs">載入中…</p>
          ) : shareRows.length === 0 ? (
            <p className="text-muted-foreground text-xs">
              還沒有加入任何群組，所以只有你看得到這一餐。
            </p>
          ) : (
            <ul className="flex flex-col gap-3">
              {shareRows.map((row) => (
                <li key={row.id} className="flex items-center gap-3">
                  <span className="flex flex-1 flex-col gap-0.5">
                    <span className="text-sm font-bold">{row.name}</span>
                    {row.shared ? (
                      <span className="text-primary-ink flex items-center gap-1 text-xs font-bold">
                        <CheckIcon className="size-3.5" aria-hidden />
                        已分享
                      </span>
                    ) : null}
                  </span>
                  <Button
                    variant={row.shared ? "outline" : "default"}
                    disabled={busy}
                    onClick={() => handleToggleShare(row.id, row.shared)}
                  >
                    {row.shared ? "取消分享" : "分享"}
                  </Button>
                </li>
              ))}
            </ul>
          )}
          <p className="text-muted-foreground text-xs">
            取消後該群組就讀不到這一餐與它的照片，聊天室裡的留言會留著。
          </p>
        </section>
      ) : null}

      {meal.is_owner ? (
        <div className="flex flex-col gap-2">
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
              <Trash2Icon aria-hidden />
              刪除這筆紀錄
            </Button>
          )}
        </div>
      ) : null}
    </main>
  );
}
