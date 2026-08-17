"use client";

import { useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { apiUrl, ApiRequestError, type Meal } from "@/lib/api";

const MAX_PHOTOS = 4;

const mealTypes = [
  { value: "", label: "不指定" },
  { value: "breakfast", label: "早餐" },
  { value: "lunch", label: "午餐" },
  { value: "dinner", label: "晚餐" },
  { value: "snack", label: "點心" },
  { value: "other", label: "其他" },
] as const;

/**
 * Recording is the product's main job, so the required path is photo then
 * publish: meal type, time and note are all optional and sit below the fold.
 */
export default function RecordPage() {
  const fileInput = useRef<HTMLInputElement>(null);
  const [photos, setPhotos] = useState<File[]>([]);
  const [mealType, setMealType] = useState("");
  const [description, setDescription] = useState("");
  const [published, setPublished] = useState<Meal | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  function handleFiles(event: React.ChangeEvent<HTMLInputElement>) {
    const chosen = Array.from(event.target.files ?? []).slice(0, MAX_PHOTOS);
    setPhotos(chosen);
    setPublished(null);
    setError(chosen.length === 0 ? null : null);
  }

  async function handlePublish() {
    if (photos.length === 0) {
      setError("至少需要一張照片");
      return;
    }

    setError(null);
    setSubmitting(true);

    const form = new FormData();
    for (const photo of photos) form.append("photos", photo);
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
        const detail = (await response.json().catch(() => null)) as { error?: string } | null;
        throw new ApiRequestError(response.status, undefined, detail?.error);
      }

      setPublished((await response.json()) as Meal);
      setPhotos([]);
      setMealType("");
      setDescription("");
      if (fileInput.current) fileInput.current.value = "";
    } catch (caught) {
      setError(caught instanceof ApiRequestError ? caught.message : "無法連線，請稍後再試");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="flex flex-1 flex-col gap-5 p-6">
      <h1 className="text-2xl font-semibold">記錄</h1>

      <div className="flex flex-col gap-2">
        <Label htmlFor="photos">照片</Label>
        <Input
          id="photos"
          ref={fileInput}
          type="file"
          accept="image/*"
          // capture opens the camera directly on a phone, which is the whole
          // point of the flow.
          capture="environment"
          multiple
          onChange={handleFiles}
        />
        <p className="text-muted-foreground text-xs">最多 {MAX_PHOTOS} 張。</p>
        {photos.length > 0 ? (
          <p className="text-sm">已選擇 {photos.length} 張</p>
        ) : null}
      </div>

      <Button onClick={handlePublish} disabled={submitting || photos.length === 0}>
        {submitting ? "發布中…" : "發布"}
      </Button>

      <details className="flex flex-col gap-3">
        <summary className="text-muted-foreground text-sm">加上餐別與備註</summary>
        <div className="mt-3 flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="meal-type">餐別</Label>
            <select
              id="meal-type"
              className="border-input h-9 rounded-md border bg-transparent px-3 text-sm"
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

      {error ? (
        <p role="alert" className="text-destructive text-sm">
          {error}
        </p>
      ) : null}

      {published ? (
        <section className="flex flex-col gap-3 border-t pt-5">
          <p role="status" className="text-sm">
            已記錄
          </p>
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
