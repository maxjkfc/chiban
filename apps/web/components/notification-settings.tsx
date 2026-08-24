"use client";

import { BellIcon, BellOffIcon, BellRingIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Message } from "@/components/ui/message";
import { usePushSubscription } from "@/lib/use-push";

/** Lets the user opt into background push notifications from their profile.
 *
 * Permission has to be requested from a direct tap: browsers refuse a
 * background call, and asking on every page load would train people to
 * dismiss it before they understand why "吃伴" wants to notify them. */
export function NotificationSettings() {
  const { isSupported, isSubscribed, permission, isStandalone, isIOS, subscribe } =
    usePushSubscription();

  if (isIOS && !isStandalone) {
    return (
      <div className="card-surface flex items-center gap-3">
        <BellOffIcon className="text-muted-foreground size-5 shrink-0" aria-hidden />
        <p className="text-muted-foreground text-xs leading-relaxed">
          請先點擊分享按鈕 ➔ 加入主畫面，才能啟用通知。
        </p>
      </div>
    );
  }

  if (!isSupported) {
    return (
      <div className="card-surface flex items-center gap-3">
        <BellOffIcon className="text-muted-foreground size-5 shrink-0" aria-hidden />
        <p className="text-muted-foreground text-xs leading-relaxed">
          此瀏覽器不支援推播通知。
        </p>
      </div>
    );
  }

  if (permission === "denied") {
    return (
      <div className="card-surface flex items-center gap-3">
        <BellOffIcon className="text-muted-foreground size-5 shrink-0" aria-hidden />
        <p className="text-muted-foreground text-xs leading-relaxed">
          通知已被封鎖，請至瀏覽器設定重新開啟。
        </p>
      </div>
    );
  }

  if (isSubscribed) {
    return (
      <div className="card-surface flex items-center gap-3">
        <BellRingIcon className="text-muted-foreground size-5 shrink-0" aria-hidden />
        <span className="flex-1 text-sm font-bold">通知已啟用</span>
      </div>
    );
  }

  return (
    <div className="card-surface flex flex-col gap-3">
      <div className="flex items-center gap-3">
        <BellIcon className="text-muted-foreground size-5 shrink-0" aria-hidden />
        <span className="flex-1 text-sm font-bold">開啟通知</span>
      </div>
      <p className="text-muted-foreground text-xs leading-relaxed">
        群組有新訊息或有人分享一餐時，即使沒開著吃伴也能收到通知。
      </p>
      <Button
        variant="outline"
        onClick={async () => {
          await subscribe();
        }}
      >
        啟用通知
      </Button>
      {permission === "default" ? null : (
        <Message tone="info">上次選擇是「{permission === "granted" ? "允許" : "拒絕"}」</Message>
      )}
    </div>
  );
}
