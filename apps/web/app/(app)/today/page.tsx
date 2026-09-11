"use client";

import {
  CalendarDaysIcon,
  CameraIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  TrophyIcon,
} from "lucide-react";
import Link from "next/link";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type PointerEvent,
} from "react";

import { Tape, tapePlacement, tapeTone, tiltClass } from "@/components/tape";
import { Button, buttonVariants } from "@/components/ui/button";
import { Message } from "@/components/ui/message";
import {
  apiFetch,
  apiUrl,
  fetchDailySummary,
  localTimeOfDay,
  mealTypeLabel,
  shiftDate,
  type Meal,
  type MealDay,
  type Profile,
} from "@/lib/api";
import {
  calendarCells,
  canShiftTrophyAnchor,
  dateRangeDates,
  isDailySummaryRangeWithinLimit,
  monthRange,
  shiftMonth,
  trophyAnchorBounds,
  trophyProgress,
  weekRange,
  type DailySummaryDay,
  type DailySummaryResponse,
} from "@/lib/daily-summary.mts";
import { dateInTimezone } from "@/lib/date.mts";
import {
  isCurrentAsyncRequest,
  isTodaySelection,
} from "@/lib/today-page-state.mts";
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
  const [mealError, setMealError] = useState<string | null>(null);
  const [profileError, setProfileError] = useState<string | null>(null);
  const [summary, setSummary] = useState<DailySummaryResponse | null>(null);
  const [summaryKey, setSummaryKey] = useState<string | null>(null);
  const [summaryFailedKey, setSummaryFailedKey] = useState<string | null>(null);
  const [view, setView] = useState<"day" | "calendar">("day");
  const [calendarMonth, setCalendarMonth] = useState<string | null>(null);
  const mealRequestKeyRef = useRef<string | null>(null);
  const summaryRequestKeyRef = useRef<string | null>(null);

  const load = useCallback((requested: string | null, signal?: AbortSignal) => {
    const requestKey = requested ?? "__today__";
    const query = requested ? `?date=${requested}` : "";
    return apiFetch<MealDay>(`/api/v1/meals${query}`, { signal })
      .then((loaded) => {
        if (
          !isCurrentAsyncRequest(
            signal,
            requestKey,
            mealRequestKeyRef.current,
          )
        ) {
          return;
        }
        setDay(loaded);
        setMealError(null);
      })
      .catch((caught: unknown) => {
        if (
          !isCurrentAsyncRequest(
            signal,
            requestKey,
            mealRequestKeyRef.current,
          )
        ) {
          return;
        }
        setMealError("讀取失敗，請重新整理");
        throw caught;
      });
  }, []);

  useEffect(() => {
    mealRequestKeyRef.current = date ?? "__today__";
    const controller = new AbortController();
    load(date, controller.signal).catch(() => {});
    return () => controller.abort();
  }, [date, load]);

  useEffect(() => {
    const controller = new AbortController();
    apiFetch<Profile>("/api/v1/me/profile", {
      signal: controller.signal,
    })
      .then((profile) => {
        if (controller.signal.aborted) return;
        setTimezone(profile.timezone);
        setProfileError(null);
      })
      .catch(() => {
        if (!controller.signal.aborted) {
          setProfileError("個人設定讀取失敗，里程碑暫時無法顯示");
        }
      });
    return () => controller.abort();
  }, []);

  const localToday = timezone
    ? dateInTimezone(new Date(), timezone)
    : day?.date ?? null;
  const selectedDate = date ?? localToday;
  const trophyBounds = localToday ? trophyAnchorBounds(localToday) : null;
  const activeCalendarMonth = calendarMonth ?? localToday?.slice(0, 7) ?? null;
  const requestedSummaryRange = timezone
    ? view === "calendar"
      ? activeCalendarMonth ? monthRange(activeCalendarMonth) : null
      : selectedDate ? weekRange(selectedDate) : null
    : null;
  const summaryRange = requestedSummaryRange &&
    isDailySummaryRangeWithinLimit(requestedSummaryRange)
    ? requestedSummaryRange
    : null;
  const summaryStart = summaryRange?.start ?? null;
  const summaryEnd = summaryRange?.end ?? null;
  const currentSummaryKey = summaryStart && summaryEnd
    ? `${summaryStart}:${summaryEnd}`
    : null;
  const summaryLoading = currentSummaryKey !== null &&
    summaryKey !== currentSummaryKey && summaryFailedKey !== currentSummaryKey;
  const visibleSummary = summaryKey === currentSummaryKey ? summary : null;
  const summaryError =
    currentSummaryKey !== null && summaryFailedKey === currentSummaryKey
      ? "里程碑資料讀取失敗，請重新整理"
      : null;

  useEffect(() => {
    summaryRequestKeyRef.current = currentSummaryKey;
    if (!summaryStart || !summaryEnd || !currentSummaryKey) return;

    const requestKey = currentSummaryKey;
    const controller = new AbortController();
    fetchDailySummary(summaryStart, summaryEnd, controller.signal)
      .then((loaded) => {
        if (
          !isCurrentAsyncRequest(
            controller.signal,
            requestKey,
            summaryRequestKeyRef.current,
          )
        ) {
          return;
        }
        setSummary(loaded);
        setSummaryKey(requestKey);
        setSummaryFailedKey(null);
      })
      .catch(() => {
        if (
          isCurrentAsyncRequest(
            controller.signal,
            requestKey,
            summaryRequestKeyRef.current,
          )
        ) {
          setSummaryFailedKey(requestKey);
        }
      });

    return () => controller.abort();
  }, [currentSummaryKey, summaryEnd, summaryStart]);

  const isToday = isTodaySelection(date, timezone);
  const summaryByDate = new Map(
    (visibleSummary?.days ?? []).map((summaryDay) => [summaryDay.date, summaryDay]),
  );
  const trophyDates = selectedDate
    ? dateRangeDates(weekRange(selectedDate))
    : [];
  const selectDate = (next: string | null) => {
    let normalized = next;
    if (normalized && localToday) {
      const bounds = trophyAnchorBounds(localToday);
      normalized = normalized < bounds.start
        ? bounds.start
        : normalized > bounds.end
          ? bounds.end
          : normalized;
    }
    if (normalized !== selectedDate) {
      setDay(null);
    }
    if (!normalized || !localToday) {
      setDate(normalized);
      return;
    }
    setDate(normalized);
  };
  const shiftSelectedDate = (amount: number) => {
    if (!selectedDate) return;
    if (
      localToday &&
      !canShiftTrophyAnchor(selectedDate, amount, localToday)
    ) return;
    selectDate(shiftDate(selectedDate, amount));
  };
  const openCalendar = () => {
    setCalendarMonth(selectedDate?.slice(0, 7) ?? null);
    setView("calendar");
  };
  const selectCalendarDate = (selectedDate: string) => {
    selectDate(selectedDate);
    setView("day");
  };

  // Newest first. The backend answers in the order the meals were eaten, which
  // is the right order for a diary but puts the meal most likely to have been
  // shared — and so to have replies on it — furthest from the thumb.
  const meals = day ? [...day.meals].reverse() : [];

  return (
    <main className="flex flex-1 flex-col gap-4 px-5 pt-7 pb-5">
      <header className="flex items-end justify-between gap-3">
        <div className="flex flex-col gap-0.5">
          <span className="text-primary-ink text-xs font-bold tracking-[0.14em]">
            {selectedDate ? selectedDate.replaceAll("-", " / ") : " "}
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

      {localToday ? (
        <section
          aria-label="七天里程碑"
          className="bg-card shadow-pop flex flex-col gap-3 rounded-2xl p-3"
        >
          <div className="flex items-center justify-between gap-3">
            <div>
              <p className="text-primary-ink text-xs font-bold tracking-[0.14em]">
                RECORDING RITUAL
              </p>
              <h2 className="text-lg">{view === "day" ? "七天里程碑" : "典藏日曆"}</h2>
            </div>
            <Button
              variant="outline"
              size="sm"
              aria-pressed={view === "calendar"}
              aria-label={view === "day" ? "切換月曆檢視" : "返回每日檢視"}
              onClick={() => view === "day" ? openCalendar() : setView("day")}
            >
              <CalendarDaysIcon aria-hidden />
              {view === "day" ? "月曆" : "每日"}
            </Button>
          </div>

          {view === "day" ? (
            summaryError ? (
              <Message tone="error">{summaryError}</Message>
            ) : summaryLoading && visibleSummary === null ? (
              <Message>里程碑載入中…</Message>
            ) : (
              <TrophyRail
                dates={trophyDates}
                selectedDate={selectedDate}
                days={summaryByDate}
                onSelectDate={selectDate}
                onShiftDate={(amount) => {
                  if (
                    selectedDate &&
                    (!localToday || canShiftTrophyAnchor(selectedDate, amount, localToday))
                  ) {
                    selectDate(shiftDate(selectedDate, amount));
                  }
                }}
                minDate={trophyBounds?.start ?? null}
                maxDate={trophyBounds?.end ?? null}
              />
            )
          ) : null}
        </section>
      ) : null}

      {view === "day" ? (
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="icon"
            aria-label="前一天"
            onClick={() => shiftSelectedDate(-1)}
            disabled={
              !selectedDate ||
              (localToday !== null &&
                !canShiftTrophyAnchor(selectedDate, -1, localToday))
            }
          >
            <ChevronLeftIcon aria-hidden />
          </Button>
          <input
            type="date"
            aria-label="選擇日期"
            value={selectedDate ?? ""}
            min={trophyBounds?.start ?? undefined}
            max={trophyBounds?.end ?? undefined}
            onChange={(event) => selectDate(event.target.value || null)}
            className="border-input bg-card h-11 flex-1 rounded-lg border px-3 text-sm"
          />
          <Button
            variant="outline"
            size="icon"
            aria-label="後一天"
            onClick={() => shiftSelectedDate(1)}
            disabled={
              !selectedDate ||
              (localToday !== null &&
                !canShiftTrophyAnchor(selectedDate, 1, localToday))
            }
          >
            <ChevronRightIcon aria-hidden />
          </Button>
          {!isToday ? (
            <Button variant="ghost" onClick={() => selectDate(null)}>
              回到今天
            </Button>
          ) : null}
        </div>
      ) : null}

      {mealError ? <Message tone="error">{mealError}</Message> : null}
      {profileError ? <Message tone="error">{profileError}</Message> : null}

      {view === "calendar" && activeCalendarMonth ? (
        summaryError ? (
          <Message tone="error">{summaryError}</Message>
        ) : (
          <div
            aria-busy={summaryLoading || undefined}
            className="flex flex-col gap-2"
          >
            <MonthCalendar
              month={activeCalendarMonth}
              today={localToday}
              selectedDate={selectedDate}
              days={summaryByDate}
              onChangeMonth={(amount) => setCalendarMonth(shiftMonth(activeCalendarMonth, amount))}
              onSelectDate={selectCalendarDate}
            />
            {summaryLoading ? <Message>月曆載入中…</Message> : null}
          </div>
        )
      ) : mealError ? null : day === null ? (
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

      {view === "day" && meals.length > 0 ? (
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

function dateAtNoonUTC(date: string): Date {
  return new Date(`${date}T12:00:00Z`);
}

function formatDayNumber(date: string): string {
  return new Intl.DateTimeFormat("zh-TW", {
    month: "numeric",
    day: "numeric",
    timeZone: "UTC",
  }).format(dateAtNoonUTC(date));
}

function formatWeekday(date: string): string {
  return new Intl.DateTimeFormat("zh-TW", {
    weekday: "short",
    timeZone: "UTC",
  }).format(dateAtNoonUTC(date));
}

function formatMonth(month: string): string {
  return new Intl.DateTimeFormat("zh-TW", {
    year: "numeric",
    month: "long",
    timeZone: "UTC",
  }).format(dateAtNoonUTC(`${month}-01`));
}

function TrophyRail({
  dates,
  selectedDate,
  days,
  onSelectDate,
  onShiftDate,
  minDate,
  maxDate,
}: {
  dates: string[];
  selectedDate: string | null;
  days: ReadonlyMap<string, DailySummaryDay>;
  onSelectDate: (date: string) => void;
  onShiftDate: (amount: number) => void;
  minDate: string | null;
  maxDate: string | null;
}) {
  const pointerStart = useRef<{ id: number; x: number } | null>(null);
  const suppressClick = useRef(false);

  const handlePointerDown = (event: PointerEvent<HTMLDivElement>) => {
    if (event.pointerType === "mouse" && event.button !== 0) return;
    pointerStart.current = { id: event.pointerId, x: event.clientX };
    event.currentTarget.setPointerCapture(event.pointerId);
  };
  const handlePointerUp = (event: PointerEvent<HTMLDivElement>) => {
    const start = pointerStart.current;
    pointerStart.current = null;
    if (!start || start.id !== event.pointerId) return;
    const delta = event.clientX - start.x;
    if (Math.abs(delta) < 40) return;
    suppressClick.current = true;
    onShiftDate(delta < 0 ? 1 : -1);
  };
  const handlePointerCancel = () => {
    pointerStart.current = null;
  };

  return (
    <div
      aria-label="七天里程碑，可左右滑動"
      className="grid grid-cols-7 gap-1 [touch-action:pan-y]"
      data-anchor-date={selectedDate ?? undefined}
      data-end-date={dates.at(-1) ?? undefined}
      data-max-date={maxDate ?? undefined}
      data-min-date={minDate ?? undefined}
      data-start-date={dates[0]}
      data-testid="trophy-rail"
      onPointerCancel={handlePointerCancel}
      onPointerDown={handlePointerDown}
      onPointerUp={handlePointerUp}
      role="group"
    >
      {dates.map((date) => {
        const mealCount = days.get(date)?.meal_count ?? 0;
        const progress = trophyProgress(mealCount);
        return (
          <button
            key={date}
            type="button"
            data-progress={progress}
            aria-label={`${formatDayNumber(date)}，${progress}/3，${mealCount} 筆紀錄`}
            aria-current={date === selectedDate ? "date" : undefined}
            onClick={() => {
              if (suppressClick.current) {
                suppressClick.current = false;
                return;
              }
              onSelectDate(date);
            }}
            className={cn(
              "flex min-w-0 flex-col items-center gap-1 rounded-xl px-0.5 py-1.5 text-xs transition-colors hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50",
              date === selectedDate && "bg-secondary font-bold ring-2 ring-primary/30",
            )}
          >
            <span className="text-muted-foreground text-[0.68rem] leading-none">
              {formatWeekday(date)}
            </span>
            <TrophyIcon
              aria-hidden
              className={cn(
                "size-5 transition-colors",
                progress > 0 ? "text-mango fill-mango" : "text-muted-foreground/35",
              )}
            />
            <span className="font-semibold leading-none">{progress}/3</span>
            <span className="text-muted-foreground text-[0.65rem] leading-none">
              {formatDayNumber(date)}
            </span>
          </button>
        );
      })}
    </div>
  );
}

function MonthCalendar({
  month,
  today,
  selectedDate,
  days,
  onChangeMonth,
  onSelectDate,
}: {
  month: string;
  today: string | null;
  selectedDate: string | null;
  days: ReadonlyMap<string, DailySummaryDay>;
  onChangeMonth: (amount: number) => void;
  onSelectDate: (date: string) => void;
}) {
  const cells = calendarCells(month);
  const weekdays = ["一", "二", "三", "四", "五", "六", "日"];

  return (
    <section aria-label={`${formatMonth(month)}月曆`} className="flex flex-col gap-3">
      <header className="flex items-center justify-between">
        <h2 className="text-lg">{formatMonth(month)}</h2>
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="上一個月"
            onClick={() => onChangeMonth(-1)}
          >
            <ChevronLeftIcon aria-hidden />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="下一個月"
            onClick={() => onChangeMonth(1)}
          >
            <ChevronRightIcon aria-hidden />
          </Button>
        </div>
      </header>
      <div className="grid grid-cols-7 gap-1 text-center text-xs">
        {weekdays.map((weekday) => (
          <span
            key={weekday}
            aria-hidden
            className="text-muted-foreground py-1 font-semibold"
          >
            {weekday}
          </span>
        ))}
        {cells.map((date, index) => (
          date ? (
            <CalendarDay
              key={date}
              date={date}
              today={today}
              selected={date === selectedDate}
              summary={days.get(date)}
              onSelectDate={onSelectDate}
            />
          ) : (
            <span key={`empty-${index}`} aria-hidden className="min-h-16 rounded-xl" />
          )
        ))}
      </div>
    </section>
  );
}

function CalendarDay({
  date,
  today,
  selected,
  summary,
  onSelectDate,
}: {
  date: string;
  today: string | null;
  selected: boolean;
  summary: DailySummaryDay | undefined;
  onSelectDate: (date: string) => void;
}) {
  const mealCount = summary?.meal_count ?? 0;
  return (
    <button
      type="button"
      data-meal-count={mealCount}
      aria-label={`${formatDayNumber(date)}，${mealCount ? `${mealCount} 筆紀錄` : "無紀錄"}`}
      aria-current={date === today ? "date" : undefined}
      onClick={() => onSelectDate(date)}
      className={cn(
        "bg-card flex min-h-16 flex-col items-center justify-between rounded-xl border p-1.5 text-sm transition-colors hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50",
        date === today && "border-primary",
        selected && "bg-secondary border-primary-ink",
      )}
    >
      <span className="font-semibold">{Number(date.slice(-2))}</span>
      <span
        className={cn(
          "grid min-w-5 place-items-center rounded-full px-1 text-[0.68rem] font-bold",
          mealCount ? "bg-primary text-primary-foreground" : "text-muted-foreground/50",
        )}
      >
        {mealCount || "·"}
      </span>
    </button>
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
