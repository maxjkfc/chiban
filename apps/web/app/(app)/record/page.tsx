"use client";

import { CameraIcon, CheckIcon, ClockIcon, ImageIcon, PlusIcon, XIcon } from "lucide-react";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";

import { Tape } from "@/components/tape";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Message } from "@/components/ui/message";
import {
  apiFetch,
  apiUrl,
  ApiRequestError,
  localTimeOfDay,
  type Group,
  type Meal,
} from "@/lib/api";
import { cn } from "@/lib/utils";

const MAX_PHOTOS = 4;

const mealTypes = [
  { value: "breakfast", label: "早餐" },
  { value: "lunch", label: "午餐" },
  { value: "dinner", label: "晚餐" },
  { value: "snack", label: "點心" },
  { value: "other", label: "其他" },
] as const;

type Picked = {
  file: File;
  previewUrl: string;
};

// A label cannot be disabled, so at the limit it is taken out of the pointer
// flow and dimmed to match a disabled button.
function pickerClassName(atLimit: boolean, variant?: "outline") {
  return cn(
    buttonVariants({ variant }),
    "cursor-pointer",
    atLimit && "pointer-events-none opacity-50",
  );
}

/**
 * Recording is the product's main job, so everything it needs is on one
 * screen: the photo, a line to write on, the meal type, and who sees it.
 *
 * Meal type used to sit inside a collapsed section below the publish button,
 * which put the two-second decision after the commitment. It is five stickers
 * now — small enough to be free, close enough to be answered before publishing.
 *
 * The empty card offers the camera and the library as two separate inputs,
 * because one input cannot be both: the `capture` attribute asks the browser
 * for a capture-type picker, and a phone with it set hands back exactly one
 * photo. Making it the only input would put the 1-4 photo range out of reach.
 *
 * Photos 2-4 go through the library input on the empty slots, which has no
 * `capture` and so takes several at once. A phone's own sheet still offers the
 * camera from there, so nothing is lost by not repeating the split.
 */
export default function RecordPage() {
  const [photos, setPhotos] = useState<Picked[]>([]);
  const [mealType, setMealType] = useState("");
  const [description, setDescription] = useState("");
  const [published, setPublished] = useState<Meal | null>(null);
  const [groups, setGroups] = useState<Group[]>([]);
  // Which groups this meal goes to. Sharing is explicit: nothing is published
  // to anyone unless it is ticked here.
  const [shareWith, setShareWith] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  // Release preview URLs when the page goes. Keeping them in a ref rather than
  // depending on `photos` matters: an effect keyed on the state would run its
  // cleanup on every add and revoke URLs the next render still shows. Removal
  // and publish revoke their own URLs already.
  const photosRef = useRef<Picked[]>([]);
  useEffect(() => {
    photosRef.current = photos;
  }, [photos]);
  useEffect(() => {
    return () => {
      for (const photo of photosRef.current)
        URL.revokeObjectURL(photo.previewUrl);
    };
  }, []);

  useEffect(() => {
    const controller = new AbortController();

    apiFetch<Group[]>("/api/v1/groups", { signal: controller.signal })
      .then(setGroups)
      .catch(() => {
        // Sharing is optional; a failed group list must not block recording.
      });

    return () => controller.abort();
  }, []);

  const remaining = MAX_PHOTOS - photos.length;

  // Both inputs land here: a photo taken now and one chosen from the library
  // are the same thing once picked, and either way they add to what is already
  // selected rather than replacing it.
  function addFiles(event: React.ChangeEvent<HTMLInputElement>) {
    const chosen = Array.from(event.target.files ?? []);
    // Clear the input so picking the same file twice still fires a change.
    event.target.value = "";
    if (chosen.length === 0) return;

    setPublished(null);
    setError(
      chosen.length > remaining
        ? `最多 ${MAX_PHOTOS} 張，只加入了前 ${remaining} 張`
        : null,
    );

    setPhotos((current) => [
      ...current,
      ...chosen.slice(0, MAX_PHOTOS - current.length).map((file) => ({
        file,
        previewUrl: URL.createObjectURL(file),
      })),
    ]);
  }

  function removePhoto(index: number) {
    setPhotos((current) => {
      const removed = current[index];
      if (removed) URL.revokeObjectURL(removed.previewUrl);
      return current.filter((_, i) => i !== index);
    });
    setError(null);
  }

  async function handlePublish() {
    if (photos.length === 0) {
      setError("至少需要一張照片");
      return;
    }

    setError(null);
    setSubmitting(true);

    const form = new FormData();
    for (const photo of photos) form.append("photos", photo.file);
    if (mealType) form.append("meal_type", mealType);
    if (description.trim()) form.append("description", description.trim());

    try {
      // Multipart, so this cannot go through the JSON helper.
      const response = await fetch(apiUrl("/api/v1/meals"), {
        method: "POST",
        credentials: "include",
        body: form,
      });
      if (!response.ok) {
        const detail = (await response.json().catch(() => null)) as {
          error?: string;
        } | null;
        throw new ApiRequestError(response.status, undefined, detail?.error);
      }

      const meal = (await response.json()) as Meal;

      // Shares are posted after the meal exists, never before: a card that
      // arrived first would point at a meal nobody could open yet.
      if (shareWith.length > 0) {
        try {
          await apiFetch<void>(`/api/v1/meals/${meal.id}/shares`, {
            method: "POST",
            body: { group_ids: shareWith },
          });
        } catch {
          // The meal is safely recorded; only the sharing failed. The meal's
          // own page can share it, so this points there instead of stranding
          // the reader with an apology.
          setError("已記錄，但分享到群組失敗——可以打開這一餐再分享一次");
        }
      }

      setPublished(meal);
      for (const photo of photos) URL.revokeObjectURL(photo.previewUrl);
      setPhotos([]);
      setMealType("");
      setDescription("");
      // Sharing is decided per meal, so the next one starts private again.
      setShareWith([]);
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

  const hero = photos[0];

  return (
    <main className="flex flex-1 flex-col gap-4 px-5 pt-7 pb-5">
      <header className="flex flex-col gap-1">
        <h1>記一餐</h1>
        <p className="text-muted-foreground text-sm">拍一張，寫一句，貼上去。</p>
      </header>

      {/* The card being made. Empty, it is the picker; filled, it is the meal
          with its caption line — the same object either way, which is what
          makes the flow read as one step instead of three. */}
      <div className="polaroid relative -rotate-[1deg] p-2.5 pb-0">
        {hero ? (
          <>
            <Tape tone={1} className="-top-2.5 left-1/2 w-[74px] -translate-x-[37px] -rotate-[2deg]" />
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src={hero.previewUrl}
              alt="第 1 張照片"
              className="h-52 w-full rounded-[2px] object-cover"
            />
          </>
        ) : (
          <div className="border-input grid h-52 place-items-center rounded-[2px] border-2 border-dashed">
            <div className="flex flex-col items-center gap-3">
              <CameraIcon className="text-muted-foreground size-8" aria-hidden />
              <div className="flex gap-2">
                <label className={pickerClassName(false)}>
                  <CameraIcon aria-hidden />
                  拍照
                  <input
                    type="file"
                    accept="image/*"
                    capture="environment"
                    className="sr-only"
                    onChange={addFiles}
                  />
                </label>
                <label className={pickerClassName(false, "outline")}>
                  <ImageIcon aria-hidden />
                  相簿
                  <input
                    type="file"
                    accept="image/*"
                    multiple
                    className="sr-only"
                    onChange={addFiles}
                  />
                </label>
              </div>
            </div>
          </div>
        )}

        {/* The caption goes on the card's white margin, where you would write
            it on the real thing. */}
        <div className="py-1">
          <Label htmlFor="description" className="sr-only">
            備註
          </Label>
          <Input
            id="description"
            variant="ruled"
            maxLength={500}
            placeholder="寫點什麼…"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
          />
        </div>
      </div>

      {/* Every slot, always: the row says "up to four" without a sentence. */}
      <ul className="flex gap-2">
        {Array.from({ length: MAX_PHOTOS }, (_, index) => {
          const photo = photos[index];
          if (photo) {
            return (
              <li key={photo.previewUrl} className="relative flex-1">
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src={photo.previewUrl}
                  alt={`第 ${index + 1} 張照片`}
                  className="aspect-square w-full rounded-md object-cover"
                />
                <button
                  type="button"
                  onClick={() => removePhoto(index)}
                  aria-label={`移除第 ${index + 1} 張照片`}
                  className="bg-card border-border text-muted-foreground absolute -top-1.5 -right-1.5 grid size-6 place-items-center rounded-full border"
                >
                  <XIcon className="size-3.5" aria-hidden />
                </button>
              </li>
            );
          }
          return (
            <li key={`empty-${index}`} className="flex-1">
              <label
                className={cn(
                  "border-input text-muted-foreground grid aspect-square w-full cursor-pointer place-items-center rounded-md border-[1.5px] border-dashed",
                  remaining === 0 && "pointer-events-none opacity-50",
                )}
              >
                <PlusIcon className="size-4" aria-hidden />
                <span className="sr-only">再加一張照片</span>
                <input
                  type="file"
                  accept="image/*"
                  multiple
                  disabled={remaining === 0}
                  className="sr-only"
                  onChange={addFiles}
                />
              </label>
            </li>
          );
        })}
      </ul>

      <fieldset className="flex flex-col gap-2">
        <legend className="sr-only">餐別</legend>
        <ul className="flex justify-between gap-2">
          {mealTypes.map((type) => {
            const chosen = mealType === type.value;
            return (
              <li key={type.value}>
                <button
                  type="button"
                  aria-pressed={chosen}
                  // Toggling off is how "不指定" is said now: the old select
                  // needed a row for it, a sticker just comes back off.
                  onClick={() =>
                    setMealType((current) =>
                      current === type.value ? "" : type.value,
                    )
                  }
                  className={cn(
                    "grid size-[3.6rem] cursor-pointer place-items-center rounded-full text-[0.8rem] transition-transform active:scale-95",
                    chosen
                      ? "bg-primary text-primary-foreground shadow-pop -rotate-[4deg] font-bold"
                      : "bg-card border-border text-muted-foreground border",
                  )}
                >
                  {type.label}
                </button>
              </li>
            );
          })}
        </ul>
      </fieldset>

      <p className="text-muted-foreground flex items-center gap-2 text-xs">
        <ClockIcon className="size-4" aria-hidden />
        用餐時間記為現在。記錄完可以在這一餐裡改。
      </p>

      {/* Sharing is the difference between a private note and the point of the
          product, so it is a decision made before publishing, not after. */}
      {groups.length > 0 ? (
        <fieldset className="flex flex-col gap-2">
          <legend className="text-muted-foreground text-xs font-bold">
            貼到哪個群組
          </legend>
          <ul className="flex flex-wrap gap-2">
            {groups.map((group, index) => {
              const chosen = shareWith.includes(group.id);
              return (
                <li key={group.id}>
                  <button
                    type="button"
                    aria-pressed={chosen}
                    onClick={() =>
                      setShareWith((current) =>
                        chosen
                          ? current.filter((id) => id !== group.id)
                          : [...current, group.id],
                      )
                    }
                    className={cn(
                      "flex h-10 cursor-pointer items-center gap-1.5 rounded-md px-3.5 text-sm",
                      index % 2 === 0 ? "-rotate-[1deg]" : "rotate-[0.8deg]",
                      chosen
                        ? "bg-primary text-primary-foreground font-bold"
                        : "bg-card border-border text-muted-foreground border",
                    )}
                  >
                    {chosen ? <CheckIcon className="size-3.5" aria-hidden /> : null}
                    {group.name}
                  </button>
                </li>
              );
            })}
          </ul>
          <p className="text-muted-foreground text-xs">沒選就只有自己看得到。</p>
        </fieldset>
      ) : null}

      {error ? <Message tone="error">{error}</Message> : null}

      <Button
        size="lg"
        onClick={handlePublish}
        loading={submitting}
        disabled={photos.length === 0}
      >
        {submitting ? "貼上去…" : "貼上去"}
      </Button>

      {published ? (
        <section className="border-border flex flex-col gap-3 border-t pt-5">
          <Message tone="success">已記錄</Message>
          <Link
            href={`/meals/${published.id}`}
            className="polaroid relative block rotate-[1.1deg]"
          >
            <Tape tone={3} className="-top-2.5 right-8 w-[74px] rotate-[5deg]" />
            <ul className="grid grid-cols-2 gap-1.5">
              {published.photo_ids.map((id) => (
                <li key={id}>
                  {/* Photos are addressed by application ID and streamed by the
                      API after it checks authorization. */}
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img
                    src={apiUrl(`/api/v1/meal-images/${id}`)}
                    alt="這一餐的照片"
                    className="aspect-square w-full rounded-[2px] object-cover"
                  />
                </li>
              ))}
            </ul>
            <p className="text-muted-foreground mt-2.5 text-xs">
              {localTimeOfDay(published.eaten_at_local)} · 點一下打開這一餐
            </p>
          </Link>
        </section>
      ) : null}
    </main>
  );
}
