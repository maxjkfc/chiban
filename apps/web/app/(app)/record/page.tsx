"use client";

import { CameraIcon, ImageIcon, XIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Message } from "@/components/ui/message";
import { apiUrl, ApiRequestError, type Meal } from "@/lib/api";
import { cn } from "@/lib/utils";

const MAX_PHOTOS = 4;

const mealTypes = [
  { value: "", label: "不指定" },
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
 * Recording is the product's main job, so the required path is photo then
 * publish: meal type, time and note are all optional and sit below the fold.
 *
 * Taking a photo and choosing one are deliberately two separate inputs. The
 * `capture` attribute asks the browser for a capture-type picker instead of a
 * file picker, so one input can offer the camera or the library but never
 * both; and with `capture` set a phone hands back exactly one photo, which
 * would put the 1-4 photo range out of reach on the device this product is
 * actually used on.
 */
export default function RecordPage() {
  const [photos, setPhotos] = useState<Picked[]>([]);
  const [mealType, setMealType] = useState("");
  const [description, setDescription] = useState("");
  const [published, setPublished] = useState<Meal | null>(null);
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
      for (const photo of photosRef.current) URL.revokeObjectURL(photo.previewUrl);
    };
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

      setPublished((await response.json()) as Meal);
      for (const photo of photos) URL.revokeObjectURL(photo.previewUrl);
      setPhotos([]);
      setMealType("");
      setDescription("");
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
    <main className="flex flex-1 flex-col gap-5 p-6">
      <h1>記錄</h1>

      {/* Labels, not buttons with an onClick that calls input.click().
          Opening the picker is then done by the browser itself, so the app's
          primary action still works if the page has not hydrated — which is
          exactly the state a tab left open across a redeploy ends up in. */}
      <div className="grid grid-cols-2 gap-3">
        <label className={pickerClassName(remaining === 0)}>
          <CameraIcon aria-hidden />
          拍照
          <input
            type="file"
            accept="image/*"
            capture="environment"
            disabled={remaining === 0}
            className="sr-only"
            onChange={addFiles}
          />
        </label>
        <label className={pickerClassName(remaining === 0, "outline")}>
          <ImageIcon aria-hidden />
          從相簿選
          <input
            type="file"
            accept="image/*"
            multiple
            disabled={remaining === 0}
            className="sr-only"
            onChange={addFiles}
          />
        </label>
      </div>

      <p className="text-muted-foreground text-xs">
        {remaining > 0 ? `還可以加 ${remaining} 張` : `已達 ${MAX_PHOTOS} 張上限`}
      </p>

      {photos.length > 0 ? (
        <ul className="grid grid-cols-4 gap-2">
          {photos.map((photo, index) => (
            <li key={photo.previewUrl} className="relative">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={photo.previewUrl}
                alt={`第 ${index + 1} 張照片`}
                className="aspect-square w-full rounded-xl object-cover"
              />
              <button
                type="button"
                onClick={() => removePhoto(index)}
                aria-label={`移除第 ${index + 1} 張照片`}
                className="bg-background/90 absolute -top-1.5 -right-1.5 rounded-full border p-1"
              >
                <XIcon className="size-3.5" aria-hidden />
              </button>
            </li>
          ))}
        </ul>
      ) : null}

      <Button
        onClick={handlePublish}
        loading={submitting}
        disabled={photos.length === 0}
      >
        {submitting ? "發布中…" : "發布"}
      </Button>

      <details className="flex flex-col gap-3">
        <summary className="text-muted-foreground text-sm">
          加上餐別與備註
        </summary>
        <div className="mt-3 flex flex-col gap-4">
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
              placeholder="例如：今天外食"
              value={description}
              onChange={(event) => setDescription(event.target.value)}
            />
          </div>
        </div>
      </details>

      {error ? <Message tone="error">{error}</Message> : null}

      {published ? (
        <section className="flex flex-col gap-3 border-t pt-5">
          <Message tone="success">已記錄</Message>
          <ul className="grid grid-cols-2 gap-2">
            {published.photo_ids.map((id) => (
              <li key={id}>
                {/* Photos are addressed by application ID and streamed by the
                    API after it checks authorization. */}
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src={apiUrl(`/api/v1/meal-images/${id}`)}
                  alt="這一餐的照片"
                  className="aspect-square w-full rounded-md object-cover"
                />
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </main>
  );
}
