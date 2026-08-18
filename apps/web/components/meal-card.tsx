"use client";

import { useEffect, useState } from "react";

import {
  fetchMeal,
  localTimeOfDay,
  mealImageUrl,
  mealTypeLabel,
  type Meal,
} from "@/lib/api";

type MealCardProps = {
  mealId: string;
};

/**
 * A shared meal, fetched by id rather than carried in the message.
 *
 * The card holds no copy of the meal: everything shown here is read through
 * the meal API, which decides on each request whether this reader may see it.
 * That is what makes unsharing and deleting take effect — the card cannot
 * outlive the permission that justified it, because it never held the data.
 */
export function MealCard({ mealId }: MealCardProps) {
  const [meal, setMeal] = useState<Meal | null>(null);
  const [state, setState] = useState<"loading" | "shown" | "gone" | "failed">(
    "loading",
  );

  useEffect(() => {
    const controller = new AbortController();

    fetchMeal(mealId, controller.signal)
      .then((found) => {
        setMeal(found);
        setState(found ? "shown" : "gone");
      })
      .catch(() => {
        // fetchMeal already turns 404 into null, so reaching here means the
        // request itself failed. Saying "deleted" would be a guess, and the
        // wrong one on a flaky network.
        if (!controller.signal.aborted) setState("failed");
      });

    return () => controller.abort();
  }, [mealId]);

  if (state === "loading") {
    return (
      <div
        className="bg-muted h-32 w-56 animate-pulse rounded-2xl"
        aria-hidden
      />
    );
  }

  if (state === "failed") {
    return (
      <p className="text-muted-foreground rounded-2xl border border-dashed px-3 py-2 text-sm">
        飲食紀錄載入失敗
      </p>
    );
  }

  // Deleted, unshared, or never visible — all the same from here, and all
  // honestly described by saying there is nothing to show.
  if (state === "gone" || !meal) {
    return (
      <p className="text-muted-foreground rounded-2xl border border-dashed px-3 py-2 text-sm italic">
        此飲食紀錄已刪除
      </p>
    );
  }

  const label = mealTypeLabel(meal.meal_type);

  return (
    <div className="bg-muted flex w-56 flex-col gap-2 overflow-hidden rounded-2xl p-2">
      <div className="grid grid-cols-2 gap-1">
        {meal.photo_ids.slice(0, 4).map((id) => (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            key={id}
            src={mealImageUrl(id)}
            alt=""
            className={`aspect-square w-full rounded-lg object-cover ${
              meal.photo_ids.length === 1 ? "col-span-2 aspect-video" : ""
            }`}
          />
        ))}
      </div>
      <p className="flex items-center gap-2 px-1 text-xs">
        <span className="font-medium">{label ?? "飲食紀錄"}</span>
        <span className="text-muted-foreground">
          {localTimeOfDay(meal.eaten_at_local)}
        </span>
      </p>
      {meal.description ? (
        <p className="line-clamp-2 px-1 text-xs">{meal.description}</p>
      ) : null}
    </div>
  );
}
