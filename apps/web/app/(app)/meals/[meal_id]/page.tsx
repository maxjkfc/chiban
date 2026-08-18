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
