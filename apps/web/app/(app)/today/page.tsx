"use client";

import { ChevronLeftIcon, ChevronRightIcon, PlusIcon } from "lucide-react";
import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

import { Button, buttonVariants } from "@/components/ui/button";
import { Message } from "@/components/ui/message";
import {
  apiFetch,
  apiUrl,
  mealTypeLabel,
  shiftDate,
  type MealDay,
} from "@/lib/api";
import { cn } from "@/lib/utils";

/**
 * Today and history are one page: the same list, a different date. Which meals
 * belong to a date is decided entirely by the backend using the profile
 * timezone, so this page never does date arithmetic on instants — only on the
 * calendar date it asks for.
 *
 * There are deliberately no calories here. V0.1 is about whether recording
 * with friends sticks, not about counting.
 */
export default function TodayPage() {
  const [day, setDay] = useState<MealDay | null>(null);
  // null means "whatever today is for me"; the backend answers with the date.
  const [date, setDate] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback((requested: string | null, signal?: AbortSignal) => {
    const query = requested ? `?date=${requested}` : "";
    return apiFetch<MealDay>(`/api/v1/meals${query}`, { signal })
      .then((loaded) => {
        setDay(loaded);
        setError(null);
      })
      .catch((caught: unknown) => {
        if (signal?.aborted) return;
        setError("讀取失敗，請重新整理");
        throw caught;
      });
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    load(date, controller.signal).catch(() => {});
    return () => controller.abort();
  }, [date, load]);

  const shown = day?.date ?? null;
  const isToday = date === null;

  return (
    <main className="flex flex-1 flex-col gap-5 p-6">
      <div className="flex items-center justify-between gap-2">
        <h1>{isToday ? "今日" : shown}</h1>
        <Link href="/record" className={cn(buttonVariants({ size: "sm" }))}>
          <PlusIcon aria-hidden />
          記錄
        </Link>
      </div>

      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="icon-sm"
          aria-label="前一天"
          onClick={() => setDate(shiftDate(shown ?? "", -1))}
          disabled={!shown}
        >
          <ChevronLeftIcon aria-hidden />
        </Button>
        <input
          type="date"
          aria-label="選擇日期"
          value={shown ?? ""}
          onChange={(event) => setDate(event.target.value || null)}
          className="border-input bg-card h-9 flex-1 rounded-2xl border px-3 text-sm"
        />
        <Button
          variant="outline"
          size="icon-sm"
          aria-label="後一天"
          onClick={() => setDate(shiftDate(shown ?? "", 1))}
          disabled={!shown}
        >
          <ChevronRightIcon aria-hidden />
        </Button>
        {!isToday ? (
          <Button variant="ghost" size="sm" onClick={() => setDate(null)}>
            回到今天
          </Button>
        ) : null}
      </div>

      {error ? <Message tone="error">{error}</Message> : null}

      {day === null ? (
        <Message>載入中…</Message>
      ) : day.meals.length === 0 ? (
        <div className="flex flex-col items-start gap-3">
          <Message>
            {isToday ? "今天還沒有紀錄。拍一張照片就完成一餐。" : "這一天沒有紀錄。"}
          </Message>
          {isToday ? (
            <Link href="/record" className={cn(buttonVariants())}>
              記錄第一餐
            </Link>
          ) : null}
        </div>
      ) : (
        <>
          <p className="text-muted-foreground text-sm">
            已記錄 {day.meals.length} 次
          </p>
          <ul className="flex flex-col gap-3">
            {day.meals.map((meal) => (
              <li key={meal.id}>
                <Link href={`/meals/${meal.id}`} className="card-surface flex flex-col gap-2">
                  <div className="flex items-baseline justify-between gap-2">
                    <span className="font-semibold">
                      {mealTypeLabel(meal.meal_type) ?? "這一餐"}
                    </span>
                    <time
                      dateTime={meal.eaten_at}
                      className="text-muted-foreground text-xs"
                    >
                      {new Date(meal.eaten_at).toLocaleTimeString([], {
                        hour: "2-digit",
                        minute: "2-digit",
                      })}
                    </time>
                  </div>
                  {meal.description ? (
                    <p className="text-sm">{meal.description}</p>
                  ) : null}
                  <ul className="grid grid-cols-4 gap-2">
                    {meal.photo_ids.map((id) => (
                      <li key={id}>
                        {/* eslint-disable-next-line @next/next/no-img-element */}
                        <img
                          src={apiUrl(`/api/v1/meal-images/${id}`)}
                          alt="這一餐的照片"
                          className="aspect-square w-full rounded-xl object-cover"
                        />
                      </li>
                    ))}
                  </ul>
                </Link>
              </li>
            ))}
          </ul>
        </>
      )}
    </main>
  );
}
