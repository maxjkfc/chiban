"use client";

import { CameraIcon, ChevronLeftIcon, ChevronRightIcon } from "lucide-react";
import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

import { Tape, tapePlacement, tapeTone, tiltClass } from "@/components/tape";
import { Button, buttonVariants } from "@/components/ui/button";
import { Message } from "@/components/ui/message";
import {
  apiFetch,
  apiUrl,
  localTimeOfDay,
  mealTypeLabel,
  shiftDate,
  type Meal,
  type MealDay,
  type Profile,
} from "@/lib/api";
import { isTodaySelection } from "@/lib/today-page-state.mts";
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
  const [timezone, setTimezone] = useState<string | null>(null);
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

  useEffect(() => {
    const controller = new AbortController();
    apiFetch<Profile>("/api/v1/me/profile", {
      signal: controller.signal,
    })
      .then((profile) => setTimezone(profile.timezone))
      .catch(() => {
        if (!controller.signal.aborted) setError("讀取失敗，請重新整理");
      });
    return () => controller.abort();
  }, []);

  const shown = day?.date ?? null;
  const isToday = isTodaySelection(date, timezone);

  // Newest first. The backend answers in the order the meals were eaten, which
  // is the right order for a diary but puts the meal most likely to have been
  // shared — and so to have replies on it — furthest from the thumb.
  const meals = day ? [...day.meals].reverse() : [];

  return (
    <main className="flex flex-1 flex-col gap-4 px-5 pt-7 pb-5">
      <header className="flex items-end justify-between gap-3">
        <div className="flex flex-col gap-0.5">
          <span className="text-primary-ink text-xs font-bold tracking-[0.14em]">
            {shown ? shown.replaceAll("-", " / ") : " "}
          </span>
          <h1>{isToday ? "今日" : "那一天"}</h1>
        </div>
        {day ? (
          <p className="flex items-center gap-2 pb-1.5">
            <span className="text-muted-foreground text-xs">已記錄</span>
            {/* Tilted like a sticker pressed onto the page — the one place a
                number gets to be decorative, because it is the only number in
                the product that is not a measurement. */}
            <span className="bg-primary text-primary-foreground font-heading shadow-pop grid size-10 -rotate-6 place-items-center rounded-full text-[1.05rem] font-black">
              {day.meals.length}
            </span>
          </p>
        ) : null}
      </header>

      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="icon"
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
          className="border-input bg-card h-11 flex-1 rounded-lg border px-3 text-sm"
        />
        <Button
          variant="outline"
          size="icon"
          aria-label="後一天"
          onClick={() => setDate(shiftDate(shown ?? "", 1))}
          disabled={!shown}
        >
          <ChevronRightIcon aria-hidden />
        </Button>
        {!isToday ? (
          <Button variant="ghost" onClick={() => setDate(null)}>
            回到今天
          </Button>
        ) : null}
      </div>

      {error ? <Message tone="error">{error}</Message> : null}

      {day === null ? (
        <Message>載入中…</Message>
      ) : meals.length === 0 ? (
        <div className="flex flex-col items-start gap-4 pt-2">
          <Message>
            {isToday
              ? "今天還沒有紀錄。拍一張照片就完成一餐。"
              : "這一天沒有紀錄。"}
          </Message>
          {isToday ? (
            <Link href="/record" className={cn(buttonVariants({ size: "lg" }))}>
              <CameraIcon aria-hidden />
              記第一餐
            </Link>
          ) : null}
        </div>
      ) : (
        <ul className="flex flex-col gap-5 pt-1">
          {meals.map((meal, index) => (
            <li key={meal.id}>
              <MealPolaroid meal={meal} index={index} />
            </li>
          ))}
        </ul>
      )}

      {meals.length > 0 ? (
        <Link
          href="/record"
          className={cn(buttonVariants({ size: "lg" }), "mt-1 w-full")}
        >
          <CameraIcon aria-hidden />
          再記一餐
        </Link>
      ) : null}
    </main>
  );
}

/**
 * One meal, mounted on paper.
 *
 * The photo is the card: at least 118px tall even when there are four of them,
 * because the whole point of the redesign is that the food is the content and
 * the metadata is the caption underneath it.
 */
function MealPolaroid({ meal, index }: { meal: Meal; index: number }) {
  const single = meal.photo_ids.length === 1;

  return (
    <Link
      href={`/meals/${meal.id}`}
      className={cn(
        "polaroid relative block transition-transform active:scale-[0.99]",
        tiltClass(index),
      )}
    >
      <Tape tone={tapeTone(index)} className={tapePlacement(index)} />
      <ul
        className={cn(
          "grid gap-1.5",
          single ? "grid-cols-1" : "grid-cols-2",
        )}
      >
        {meal.photo_ids.map((id) => (
          <li key={id}>
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src={apiUrl(`/api/v1/meal-images/${id}`)}
              alt="這一餐的照片"
              className={cn(
                "w-full rounded-[2px] object-cover",
                single ? "h-[8.5rem]" : "aspect-square",
              )}
            />
          </li>
        ))}
      </ul>
      <div className="mt-2.5 flex items-baseline justify-between gap-2">
        <span className="font-heading text-[0.95rem] font-black">
          {mealTypeLabel(meal.meal_type) ?? "這一餐"}
        </span>
        {/* The server already rendered this in the profile zone; formatting it
            here would use the browser's instead. */}
        <time
          dateTime={meal.eaten_at}
          className="text-muted-foreground text-xs"
        >
          {localTimeOfDay(meal.eaten_at_local)}
        </time>
      </div>
      {meal.description ? (
        <p className="mt-1 text-sm leading-relaxed">{meal.description}</p>
      ) : null}
    </Link>
  );
}
