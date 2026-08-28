"use client";

import {
  PinIcon,
  RotateCcwIcon,
  SearchIcon,
  SmileIcon,
  XIcon,
} from "lucide-react";
import { Dialog } from "radix-ui";
import { useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  DEFAULT_REACTION_EMOJIS,
  EMOJI_CATEGORIES,
  MAX_PINNED_REACTIONS,
  getRecentReactions,
  type EmojiCategory,
} from "@/lib/emoji-data";
import { cn } from "@/lib/utils";

type EmojiPickerModalProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pinnedReactions: string[];
  onUpdatePinned: (newPinned: string[]) => void;
  onSelectEmoji: (emoji: string) => void;
};

export function EmojiPickerModal({
  open,
  onOpenChange,
  pinnedReactions,
  onUpdatePinned,
  onSelectEmoji,
}: EmojiPickerModalProps) {
  const [activeTab, setActiveTab] = useState<string>("all");
  const [search, setSearch] = useState("");
  const [editPinned, setEditPinned] = useState(false);

  const recents = useMemo(() => {
    if (!open) return [];
    return getRecentReactions();
  }, [open]);

  const allCategories: EmojiCategory[] = useMemo(() => {
    const list: EmojiCategory[] = [];
    if (recents.length > 0) {
      list.push({
        id: "recent",
        name: "最近使用",
        icon: "🕒",
        emojis: recents,
      });
    }
    return [...list, ...EMOJI_CATEGORIES];
  }, [recents]);

  const filteredCategories = useMemo(() => {
    const query = search.trim();
    if (!query) {
      if (activeTab === "all") return allCategories;
      return allCategories.filter((c) => c.id === activeTab);
    }
    return allCategories
      .map((cat) => {
        const matchesCategory = cat.name.includes(query);
        return {
          ...cat,
          emojis: matchesCategory
            ? cat.emojis
            : cat.emojis.filter((emoji) => emoji.includes(query)),
        };
      })
      .filter((cat) => cat.emojis.length > 0);
  }, [allCategories, activeTab, search]);

  function handleTogglePin(emoji: string) {
    if (pinnedReactions.includes(emoji)) {
      if (pinnedReactions.length <= 1) return; // Keep at least one
      onUpdatePinned(pinnedReactions.filter((e) => e !== emoji));
    } else {
      if (pinnedReactions.length >= MAX_PINNED_REACTIONS) {
        // Replace the last one
        onUpdatePinned([
          ...pinnedReactions.slice(0, MAX_PINNED_REACTIONS - 1),
          emoji,
        ]);
      } else {
        onUpdatePinned([...pinnedReactions, emoji]);
      }
    }
  }

  function handleSelect(emoji: string) {
    if (editPinned) {
      handleTogglePin(emoji);
      return;
    }
    onSelectEmoji(emoji);
    onOpenChange(false);
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="bg-background/80 fixed inset-0 z-50 backdrop-blur-sm transition-opacity" />
        <Dialog.Content className="bg-card text-card-foreground fixed inset-x-0 bottom-0 z-50 flex max-h-[85vh] min-h-[50vh] flex-col rounded-t-2xl border-t shadow-2xl transition-all sm:inset-auto sm:top-1/2 sm:left-1/2 sm:w-full sm:max-w-md sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-2xl sm:border">
          {/* Header */}
          <div className="flex items-center justify-between border-b px-4 py-3">
            <div className="flex items-center gap-2">
              <SmileIcon className="text-primary size-5" />
              <Dialog.Title className="text-base font-semibold">
                {editPinned ? "自訂快捷 Reaction" : "選擇表情回應"}
              </Dialog.Title>
            </div>
            <div className="flex items-center gap-1">
              <Button
                type="button"
                variant={editPinned ? "default" : "ghost"}
                size="sm"
                className="h-8 gap-1 px-2.5 text-xs"
                onClick={() => setEditPinned((prev) => !prev)}
              >
                <PinIcon className="size-3.5" />
                {editPinned ? "完成" : "自訂快捷"}
              </Button>
              <Dialog.Close asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-8 rounded-full"
                >
                  <XIcon className="size-4" />
                  <span className="sr-only">關閉</span>
                </Button>
              </Dialog.Close>
            </div>
          </div>

          {/* Current Pinned Bar */}
          <div className="bg-muted/40 flex flex-col gap-2 border-b px-4 py-2.5">
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground text-xs font-medium">
                {editPinned
                  ? `點擊下方表情以加入或移除快捷（最多 ${MAX_PINNED_REACTIONS} 個）：`
                  : "目前訊息快捷列："}
              </span>
              {editPinned && (
                <button
                  type="button"
                  onClick={() => onUpdatePinned(DEFAULT_REACTION_EMOJIS)}
                  className="text-muted-foreground hover:text-foreground flex items-center gap-1 text-xs underline cursor-pointer"
                >
                  <RotateCcwIcon className="size-3" />
                  重設預設
                </button>
              )}
            </div>
            <div className="flex items-center gap-2">
              {pinnedReactions.map((emoji) => (
                <button
                  key={emoji}
                  type="button"
                  onClick={() => handleSelect(emoji)}
                  className={cn(
                    "relative flex size-10 items-center justify-center rounded-xl text-xl transition-transform active:scale-95 cursor-pointer",
                    editPinned
                      ? "border-primary bg-primary/10 border"
                      : "bg-card hover:bg-muted border shadow-xs",
                  )}
                >
                  {emoji}
                  {editPinned && (
                    <span className="bg-destructive text-destructive-foreground absolute -top-1 -right-1 flex size-4 items-center justify-center rounded-full text-[10px] font-bold">
                      ×
                    </span>
                  )}
                </button>
              ))}
            </div>
          </div>

          {/* Search Bar & Category Filter */}
          <div className="flex flex-col gap-2 border-b px-4 py-2">
            <div className="relative">
              <SearchIcon className="text-muted-foreground absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
              <input
                type="text"
                placeholder="搜尋或直接點選表情..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="bg-muted/50 focus:bg-background placeholder:text-muted-foreground focus:ring-ring h-9 w-full rounded-lg pr-3 pl-8 text-sm outline-none focus:ring-1"
              />
            </div>

            {!search && (
              <div className="scrollbar-none flex items-center gap-1 overflow-x-auto pb-1">
                <button
                  type="button"
                  onClick={() => setActiveTab("all")}
                  className={cn(
                    "flex shrink-0 items-center gap-1 rounded-full px-2.5 py-1 text-xs font-medium transition-colors cursor-pointer",
                    activeTab === "all"
                      ? "bg-primary text-primary-foreground"
                      : "bg-muted text-muted-foreground hover:text-foreground",
                  )}
                >
                  全部
                </button>
                {allCategories.map((cat) => (
                  <button
                    key={cat.id}
                    type="button"
                    onClick={() => setActiveTab(cat.id)}
                    className={cn(
                      "flex shrink-0 items-center gap-1 rounded-full px-2.5 py-1 text-xs font-medium transition-colors cursor-pointer",
                      activeTab === cat.id
                        ? "bg-primary text-primary-foreground"
                        : "bg-muted text-muted-foreground hover:text-foreground",
                    )}
                  >
                    <span>{cat.icon}</span>
                    <span>{cat.name}</span>
                  </button>
                ))}
              </div>
            )}
          </div>

          {/* Emoji Grid Scroller */}
          <div className="flex-1 overflow-y-auto px-4 py-3 min-h-0">
            {filteredCategories.length === 0 ? (
              <div className="text-muted-foreground flex h-32 items-center justify-center text-sm">
                找不到相關的表情
              </div>
            ) : (
              <div className="flex flex-col gap-4">
                {filteredCategories.map((cat) => (
                  <div key={cat.id} className="flex flex-col gap-2">
                    <span className="text-muted-foreground flex items-center gap-1 text-xs font-semibold">
                      <span>{cat.icon}</span>
                      <span>{cat.name}</span>
                    </span>
                    <div className="grid grid-cols-7 gap-1.5 sm:grid-cols-8">
                      {cat.emojis.map((emoji, idx) => {
                        const isPinned = pinnedReactions.includes(emoji);
                        return (
                          <button
                            key={`${cat.id}-${emoji}-${idx}`}
                            type="button"
                            onClick={() => handleSelect(emoji)}
                            className={cn(
                              "relative flex aspect-square items-center justify-center rounded-lg text-2xl transition-transform hover:bg-muted active:scale-90 cursor-pointer",
                              isPinned &&
                                editPinned &&
                                "ring-2 ring-primary bg-primary/10",
                            )}
                            title={
                              isPinned && editPinned
                                ? "點擊移除快捷"
                                : undefined
                            }
                          >
                            <span>{emoji}</span>
                            {editPinned && isPinned && (
                              <span className="bg-primary absolute top-0.5 right-0.5 size-1.5 rounded-full" />
                            )}
                          </button>
                        );
                      })}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
