"use client";

import {
  ImagePlusIcon,
  LayoutGridIcon,
  PlusIcon,
  RotateCwIcon,
  StarIcon,
  XIcon,
} from "lucide-react";
import { Dialog } from "radix-ui";
import { useEffect, useState } from "react";

import { Button, buttonVariants } from "@/components/ui/button";
import {
  apiFetch,
  MAX_STICKER_PINS,
  setStickerPins,
  stickerUrl,
  uploadSticker,
  type Sticker,
} from "@/lib/api";
import { cn } from "@/lib/utils";

type StickerPickerProps = {
  /**
   * The tray is controlled by the chat room, which owns the composer button
   * that opens it. The rail and the tray sit in different places in the
   * composer, so a self-contained trigger could only be in one of them.
   */
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Posts one sticker. Resolves true when it actually went out. */
  onSend: (stickerId: string) => Promise<boolean>;
  /** True while any send is in flight; a second tap is refused. */
  sending: boolean;
  /**
   * The rail is hidden while there is something in the composer: once you are
   * typing, the keyboard and the draft are what matter, and a permanent rail
   * would cost 76px of conversation on every screen for the rest of the chat.
   */
  showRail: boolean;
};

/**
 * The quick rail and the sticker tray.
 *
 * The rail is the point: the stickers someone reaches for are one tap away
 * instead of two-taps-and-a-scan. Which four they are is a choice, not a usage
 * statistic — the API keeps the pinned list, so it follows the person to a new
 * phone. Until someone picks, the rail shows the four most recent stickers, so
 * nobody has to visit a settings screen before their first sticker.
 *
 * Unlike the picker this replaces, the library is fetched on mount rather than
 * on first open: the rail cannot draw itself without it, and the list is ids
 * and types only.
 */
export function StickerPicker({
  open,
  onOpenChange,
  onSend,
  sending,
  showRail,
}: StickerPickerProps) {
  const [library, setLibrary] = useState<Sticker[] | null>(null);
  // "we could not ask" and "you have none" lead to completely different next
  // actions, so they are never drawn the same way.
  const [failed, setFailed] = useState(false);
  const [editing, setEditing] = useState(false);
  // The rail being assembled. Only committed when 完成 is pressed, so backing
  // out of the editor leaves the saved rail alone.
  const [draftPins, setDraftPins] = useState<string[]>([]);
  const [uploading, setUploading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [sheetError, setSheetError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, []);

  // Every close goes through here, so the sheet always opens on the library
  // with nothing left over. The dialog's own onOpenChange is not enough: it
  // fires for the closes the dialog initiates — escape, the overlay — but not
  // for the one this component performs once a sticker has gone out.
  function closeSheet() {
    setEditing(false);
    setSheetError(null);
    onOpenChange(false);
  }

  function load(signal?: AbortSignal) {
    setFailed(false);
    return apiFetch<Sticker[]>("/api/v1/me/stickers", { signal })
      .then(setLibrary)
      .catch(() => {
        if (!signal?.aborted) setFailed(true);
      });
  }

  const pinned = (library ?? [])
    .filter((sticker) => sticker.pin_order)
    .sort((a, b) => (a.pin_order ?? 0) - (b.pin_order ?? 0));

  // Nobody has to configure anything before their first sticker: an unset rail
  // is the newest few, which is the order the library already comes in.
  const rail = pinned.length > 0 ? pinned : (library ?? []).slice(0, MAX_STICKER_PINS);

  async function send(stickerId: string) {
    if (sending) return;
    setSheetError(null);
    if (await onSend(stickerId)) {
      closeSheet();
    } else if (open) {
      // Only when the tray is on screen. A rail tap fails with the sheet shut,
      // and a message nobody can see would still be sitting there the next
      // time it opens — the chat room reports that one where it happened.
      setSheetError("沒送出去，再試一次");
    }
  }

  async function handleUpload(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    // Clear it so picking the same file twice still fires a change.
    event.target.value = "";
    if (!file) return;

    setSheetError(null);
    setUploading(true);
    try {
      const added = await uploadSticker(file);
      // Newest first, matching what the API returns on the next read.
      setLibrary((current) => [added, ...(current ?? [])]);
    } catch {
      setSheetError("上傳失敗，換一張或稍後再試");
    } finally {
      setUploading(false);
    }
  }

  function startEditing() {
    setSheetError(null);
    // Seeded with what is actually on the rail — including the unset default,
    // so "edit" starts from what the person is looking at rather than blank.
    setDraftPins(rail.map((sticker) => sticker.id));
    setEditing(true);
  }

  function toggleDraftPin(stickerId: string) {
    // Decided here rather than inside the updater: React is free to run an
    // updater more than once, and one that reports a full rail as a side
    // effect would say so twice — or during a render.
    if (
      !draftPins.includes(stickerId) &&
      draftPins.length >= MAX_STICKER_PINS
    ) {
      setSheetError(`快捷列只有 ${MAX_STICKER_PINS} 格，先拿掉一張`);
      return;
    }

    setSheetError(null);
    setDraftPins((current) =>
      current.includes(stickerId)
        ? current.filter((id) => id !== stickerId)
        : [...current, stickerId],
    );
  }

  async function saveDraftPins() {
    setSheetError(null);
    setSaving(true);
    try {
      setLibrary(await setStickerPins(draftPins));
      setEditing(false);
    } catch {
      setSheetError("儲存失敗，稍後再試");
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      {showRail && rail.length > 0 ? (
        <div className="flex items-center gap-2.5 px-4 pb-2">
          <ul className="flex gap-2.5">
            {rail.map((sticker) => (
              <li key={sticker.id}>
                <StickerButton
                  sticker={sticker}
                  size="rail"
                  disabled={sending}
                  onClick={() => send(sticker.id)}
                  label="傳送這個貼圖"
                />
              </li>
            ))}
          </ul>
          <button
            type="button"
            onClick={() => onOpenChange(true)}
            className="border-input text-muted-foreground flex size-[3.75rem] cursor-pointer flex-col items-center justify-center gap-0.5 rounded-md border-[1.5px] border-dashed"
          >
            <LayoutGridIcon className="size-4" aria-hidden />
            <span className="text-[0.66rem] font-bold">全部</span>
          </button>
        </div>
      ) : null}

      <Dialog.Root
        open={open}
        onOpenChange={(next) => (next ? onOpenChange(true) : closeSheet())}
      >
        <Dialog.Portal>
          <Dialog.Overlay className="fixed inset-0 z-40 bg-[oklch(0.2_0.02_60_/_0.28)]" />
          {/* Anchored to the app's own column rather than the viewport, so the
              sheet stays under the conversation on a desktop window. */}
          <Dialog.Content className="bg-background border-border fixed inset-x-0 bottom-0 z-50 mx-auto max-h-[72dvh] w-full max-w-md overflow-y-auto rounded-t-xl border-t px-4 pt-2.5 pb-[max(1rem,env(safe-area-inset-bottom))] shadow-[0_-14px_30px_-18px_var(--pop-shadow)]">
            <div className="grid h-4 place-items-center" aria-hidden>
              <span className="bg-input h-1 w-10 rounded-full" />
            </div>

            {/* One tap sends, which is the thing a screen reader cannot infer
                from the buttons alone — and the dialog needs a description
                either way. */}
            <Dialog.Description className="sr-only">
              點一張貼圖就直接送出。也可以在這裡編輯鍵盤上方的快捷列。
            </Dialog.Description>

            {editing ? (
              <>
                <div className="flex items-center gap-3 pt-2 pb-1.5">
                  <Dialog.Title className="font-heading flex-1 text-[1.05rem] font-black">
                    編輯快捷列
                  </Dialog.Title>
                  <Button onClick={saveDraftPins} loading={saving}>
                    完成
                  </Button>
                </div>
                <p className="text-muted-foreground pb-3 text-xs leading-relaxed">
                  選 {MAX_STICKER_PINS} 張放在鍵盤上方，點到的順序就是排列順序。
                  不選也可以——預設會放最新的 {MAX_STICKER_PINS} 張。
                </p>

                <ul className="bg-tape-2/20 flex gap-2.5 rounded-lg p-3">
                  {Array.from({ length: MAX_STICKER_PINS }, (_, slot) => {
                    const stickerId = draftPins[slot];
                    const sticker = stickerId
                      ? (library ?? []).find((one) => one.id === stickerId)
                      : undefined;
                    return (
                      <li key={slot} className="flex flex-1 flex-col items-center gap-1">
                        {sticker ? (
                          <div className="relative">
                            <StickerButton
                              sticker={sticker}
                              size="slot"
                              onClick={() => toggleDraftPin(sticker.id)}
                              label={`把這張從第 ${slot + 1} 格拿掉`}
                            />
                            <span
                              aria-hidden
                              className="bg-card border-border text-muted-foreground pointer-events-none absolute -top-1.5 -right-1.5 grid size-6 place-items-center rounded-full border"
                            >
                              <XIcon className="size-3" />
                            </span>
                          </div>
                        ) : (
                          <div className="border-input text-muted-foreground grid size-[4.6rem] place-items-center rounded-lg border-[1.5px] border-dashed text-lg font-bold">
                            {slot + 1}
                          </div>
                        )}
                        <span className="text-muted-foreground text-[0.66rem]">
                          {sticker ? slot + 1 : "空的"}
                        </span>
                      </li>
                    );
                  })}
                </ul>

                <p className="text-muted-foreground flex items-center gap-2.5 pt-4 pb-2.5 text-xs">
                  <span className="font-bold tracking-[0.06em]">
                    全部 {(library ?? []).length} 張
                  </span>
                  <span className="bg-border h-px flex-1" />
                  <span className="text-primary-ink font-bold">
                    已選 {draftPins.length} / {MAX_STICKER_PINS}
                  </span>
                </p>
              </>
            ) : (
              <>
                <div className="flex items-center gap-3 pt-2 pb-2.5">
                  <Dialog.Title className="font-heading text-[1.05rem] font-black">
                    我的貼圖
                  </Dialog.Title>
                  <span className="text-muted-foreground flex-1 text-xs">
                    {library ? `${library.length} 張` : ""}
                  </span>
                  <label
                    className={cn(
                      buttonVariants({ variant: "outline" }),
                      "text-primary-ink cursor-pointer",
                      uploading && "pointer-events-none opacity-50",
                    )}
                  >
                    <ImagePlusIcon aria-hidden />
                    {uploading ? "上傳中…" : "加一張"}
                    <input
                      type="file"
                      accept="image/*"
                      disabled={uploading}
                      className="sr-only"
                      onChange={handleUpload}
                    />
                  </label>
                </div>

                {rail.length > 0 ? (
                  <div className="bg-tape-2/20 mb-3 flex items-center gap-2.5 rounded-lg px-3 py-2.5">
                    <span className="text-foreground/70 flex shrink-0 items-center gap-1.5 text-xs font-bold">
                      <StarIcon className="size-3.5" aria-hidden />
                      快捷列
                    </span>
                    <ul className="flex gap-1.5">
                      {rail.map((sticker) => (
                        <li key={sticker.id}>
                          {/* eslint-disable-next-line @next/next/no-img-element */}
                          <img
                            src={stickerUrl(sticker.id)}
                            alt=""
                            className="size-9 rounded-sm object-contain"
                          />
                        </li>
                      ))}
                    </ul>
                    <Button
                      variant="outline"
                      className="ml-auto shrink-0"
                      onClick={startEditing}
                    >
                      編輯
                    </Button>
                  </div>
                ) : null}
              </>
            )}

            {sheetError ? (
              <p
                role="alert"
                className="bg-destructive/10 text-destructive mb-3 rounded-lg px-3 py-2 text-sm"
              >
                {sheetError}
              </p>
            ) : null}

            {failed ? (
              <div className="bg-destructive/10 flex items-center gap-3 rounded-lg px-3.5 py-3">
                <span className="text-destructive flex-1 text-sm">
                  貼圖載入失敗
                </span>
                <Button variant="outline" onClick={() => load()}>
                  <RotateCwIcon aria-hidden />
                  重試
                </Button>
              </div>
            ) : library === null ? (
              <p className="text-muted-foreground py-6 text-center text-sm">
                載入中…
              </p>
            ) : library.length === 0 ? (
              // Not a sentence pointing at another page: leaving the chat to
              // add a sticker loses your place in the conversation.
              <label
                className={cn(
                  "border-input flex cursor-pointer items-center gap-4 rounded-lg border-[1.5px] border-dashed p-3.5",
                  uploading && "pointer-events-none opacity-50",
                )}
              >
                <span className="text-primary-ink border-input grid size-[4.5rem] shrink-0 place-items-center rounded-lg border-[1.5px] border-dashed">
                  <PlusIcon className="size-6" aria-hidden />
                </span>
                <span className="flex flex-col gap-1">
                  <span className="text-sm font-bold">加第一張貼圖</span>
                  <span className="text-muted-foreground text-xs leading-relaxed">
                    從相簿選一張圖或 GIF，只有你自己用得到。
                  </span>
                </span>
                <input
                  type="file"
                  accept="image/*"
                  disabled={uploading}
                  className="sr-only"
                  onChange={handleUpload}
                />
              </label>
            ) : (
              <ul className="grid grid-cols-3 gap-2.5 pb-2">
                {library.map((sticker) => {
                  const slot = editing
                    ? draftPins.indexOf(sticker.id) + 1
                    : (sticker.pin_order ?? 0);
                  return (
                    <li key={sticker.id} className="relative">
                      <StickerButton
                        sticker={sticker}
                        size="grid"
                        disabled={!editing && sending}
                        onClick={() =>
                          editing ? toggleDraftPin(sticker.id) : send(sticker.id)
                        }
                        label={
                          editing
                            ? slot > 0
                              ? "從快捷列拿掉"
                              : "放到快捷列"
                            : "傳送這個貼圖"
                        }
                      />
                      {slot > 0 ? (
                        <span
                          aria-hidden
                          className="bg-primary text-primary-foreground pointer-events-none absolute -top-1.5 -right-1.5 grid size-6 place-items-center rounded-full text-[0.7rem] font-bold"
                        >
                          {editing ? slot : <StarIcon className="size-3 fill-current" />}
                        </span>
                      ) : null}
                    </li>
                  );
                })}
              </ul>
            )}

            {editing ? (
              <Button
                variant="ghost"
                className="w-full"
                onClick={() => setEditing(false)}
              >
                取消
              </Button>
            ) : null}
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </>
  );
}

const sizes = {
  rail: "size-[3.75rem] rounded-md",
  slot: "size-[4.6rem] rounded-lg",
  grid: "aspect-square w-full rounded-md",
} as const;

/**
 * One sticker as a control. One tap is the whole interaction — a picker that
 * needed a second confirming tap would be slower than typing, which is the one
 * thing a sticker has to beat.
 */
function StickerButton({
  sticker,
  size,
  disabled,
  onClick,
  label,
}: {
  sticker: Sticker;
  size: keyof typeof sizes;
  disabled?: boolean;
  onClick: () => void;
  label: string;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      aria-label={label}
      className={cn(
        "bg-card border-border cursor-pointer border p-1 transition-transform active:scale-95 disabled:opacity-50",
        sizes[size],
      )}
    >
      {/* A GIF animates in an img tag; nothing re-encodes it on the way here. */}
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img
        src={stickerUrl(sticker.id)}
        alt=""
        className="size-full object-contain"
      />
    </button>
  );
}

/** Matches lucide's 24px grid and stroke so it sits with the other controls. */
export function StickerFaceIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
      className={className}
    >
      <path d="M15.5 21A9 9 0 1 0 3 12" />
      <path d="M21 12a9 9 0 0 1-9 9v-4a5 5 0 0 0 5-5Z" />
      <path d="M9 10h.01M15 10h.01" />
    </svg>
  );
}
