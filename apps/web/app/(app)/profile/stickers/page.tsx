"use client";

import { ChevronLeftIcon, StarIcon, TrashIcon, UploadIcon } from "lucide-react";
import Link from "next/link";
import { useEffect, useState } from "react";

import { buttonVariants } from "@/components/ui/button";
import { Message } from "@/components/ui/message";
import {
  apiFetch,
  ApiRequestError,
  stickerUrl,
  uploadSticker,
  type Sticker,
} from "@/lib/api";
import { cn } from "@/lib/utils";

/**
 * A sticker library is a list you tidy, so this page is only ever add and
 * remove. There is nothing to name or arrange: the picker shows newest first,
 * which is the order people reach for.
 */
export default function StickersPage() {
  const [stickers, setStickers] = useState<Sticker[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    const controller = new AbortController();

    apiFetch<Sticker[]>("/api/v1/me/stickers", { signal: controller.signal })
      .then(setStickers)
      .catch(() => {
        if (!controller.signal.aborted) setError("讀取失敗，請重新整理");
      });

    return () => controller.abort();
  }, []);

  async function handleAdd(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    // Clear it so picking the same file twice still fires a change.
    event.target.value = "";
    if (!file) return;

    setError(null);
    setBusy(true);
    try {
      const added = await uploadSticker(file);
      // Newest first, matching what the picker will show.
      setStickers((current) => [added, ...(current ?? [])]);
    } catch (caught) {
      setError(
        caught instanceof ApiRequestError
          ? caught.message
          : "上傳失敗，請稍後再試",
      );
    } finally {
      setBusy(false);
    }
  }

  // No confirmation step: deleting takes it out of the picker and leaves every
  // message already sent with it untouched, so there is nothing here to lose
  // by mistake that uploading it again would not undo.
  async function handleDelete(stickerId: string) {
    setError(null);
    setBusy(true);
    try {
      await apiFetch<void>(`/api/v1/me/stickers/${stickerId}`, {
        method: "DELETE",
      });
      setStickers((current) =>
        (current ?? []).filter((sticker) => sticker.id !== stickerId),
      );
    } catch {
      setError("刪除失敗，請稍後再試");
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="flex flex-1 flex-col gap-5 px-5 pt-7 pb-5">
      <header className="flex items-center gap-3">
        <Link
          href="/profile"
          aria-label="回到「我的」"
          className={buttonVariants({ variant: "outline", size: "icon" })}
        >
          <ChevronLeftIcon aria-hidden />
        </Link>
        <div className="flex flex-col gap-0.5">
          <h1 className="text-2xl">我的貼圖</h1>
          <p className="text-muted-foreground text-xs">
            聊天室的快捷列從這裡挑。
          </p>
        </div>
      </header>

      {/* A label, so the browser opens the picker itself. */}
      <label
        className={cn(
          buttonVariants({ variant: "outline" }),
          "cursor-pointer",
          busy && "pointer-events-none opacity-50",
        )}
      >
        <UploadIcon aria-hidden />
        上傳貼圖
        <input
          type="file"
          accept="image/*"
          disabled={busy}
          className="sr-only"
          onChange={handleAdd}
        />
      </label>

      {error ? <Message tone="error">{error}</Message> : null}

      {stickers !== null && stickers.length > 0 ? (
        <p className="text-muted-foreground text-xs">
          刪除只會把貼圖收回這裡，已經傳出去的訊息還是看得到。
        </p>
      ) : null}

      {stickers === null ? (
        <p className="text-muted-foreground text-xs">載入中…</p>
      ) : stickers.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          還沒有貼圖。GIF 也可以，傳進來會繼續動。
        </p>
      ) : (
        <ul className="grid grid-cols-3 gap-3">
          {stickers.map((sticker) => (
            <li key={sticker.id} className="bg-card border-border relative rounded-md border p-2">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={stickerUrl(sticker.id)}
                alt="貼圖"
                className="aspect-square w-full object-contain"
              />
              {sticker.pin_order ? (
                <span className="bg-primary text-primary-foreground absolute -top-2 -left-2 grid size-6 place-items-center rounded-full">
                  <StarIcon className="size-3 fill-current" aria-hidden />
                  <span className="sr-only">在快捷列第 {sticker.pin_order} 格</span>
                </span>
              ) : null}
              <button
                type="button"
                disabled={busy}
                onClick={() => handleDelete(sticker.id)}
                aria-label="刪除這個貼圖"
                className="bg-card text-muted-foreground absolute -top-2 -right-2 rounded-full border p-1.5 disabled:opacity-50"
              >
                <TrashIcon className="size-3.5" aria-hidden />
              </button>
            </li>
          ))}
        </ul>
      )}

    </main>
  );
}
