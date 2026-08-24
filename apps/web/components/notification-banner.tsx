"use client";

import { useEffect, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { getOrCreateDeviceId } from "@/lib/device";

interface InAppNotification {
  id: string;
  groupId: string;
  title: string;
  body: string;
}

export function NotificationBanner() {
  const [notification, setNotification] = useState<InAppNotification | null>(null);
  const pathname = usePathname();
  const router = useRouter();

  // Extract current focused group ID if on /groups/[id]
  const currentGroupId = pathname?.startsWith("/groups/") ? pathname.split("/")[2] : null;

  useEffect(() => {
    let ws: WebSocket | null = null;
    let heartbeatInterval: NodeJS.Timeout | null = null;
    let isClosed = false;

    async function connect() {
      const deviceId = await getOrCreateDeviceId();
      const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
      const host = window.location.host;
      const wsUrl = `${proto}//${host}/api/v1/ws/me`;

      ws = new WebSocket(wsUrl);

      ws.onopen = () => {
        // Send initial focus or ping
        if (currentGroupId) {
          ws?.send(JSON.stringify({ type: "focus", group_id: currentGroupId, device_id: deviceId }));
        } else {
          ws?.send(JSON.stringify({ type: "ping" }));
        }

        // Heartbeat every 25 seconds
        heartbeatInterval = setInterval(() => {
          if (ws?.readyState === WebSocket.OPEN) {
            if (currentGroupId) {
              ws.send(JSON.stringify({ type: "focus", group_id: currentGroupId, device_id: deviceId }));
            } else {
              ws.send(JSON.stringify({ type: "ping" }));
            }
          }
        }, 25000);
      };

      ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          if (data.type === "message" && data.message) {
            const msg = data.message;
            // Ignore if event belongs to current actively open group
            if (msg.group_id === currentGroupId) {
              return;
            }

            let text = msg.content || "傳送了一則新訊息";
            if (msg.type === "image") text = "📷 傳送了一張圖片";
            if (msg.type === "gif") text = "🎞️ 傳送了一個 GIF";
            if (msg.type === "sticker") text = "✨ 傳送了一個貼圖";
            if (msg.type === "meal") text = "🍱 分享了一餐";

            setNotification({
              id: msg.id,
              groupId: msg.group_id,
              title: "新訊息",
              body: text,
            });
          }
        } catch {
          // ignore parse errors
        }
      };

      ws.onclose = () => {
        if (!isClosed) {
          setTimeout(connect, 3000);
        }
      };
    }
    return () => {
      isClosed = true;
      clearInterval(heartbeatInterval!);
      ws?.close();
    };
  }, [currentGroupId]);

  // Auto-dismiss banner after 5 seconds
  useEffect(() => {
    if (notification) {
      const timer = setTimeout(() => {
        setNotification(null);
      }, 5000);
      return () => clearTimeout(timer);
    }
  }, [notification]);

  if (!notification) return null;

  return (
    <div className="fixed top-4 inset-x-0 z-50 flex justify-center px-4 pointer-events-none animate-in fade-in slide-in-from-top-4 duration-300">
      <div
        onClick={() => {
          const gid = notification.groupId;
          setNotification(null);
          router.push(`/groups/${gid}`);
        }}
        className="pointer-events-auto flex w-full max-w-sm cursor-pointer items-center justify-between gap-3 rounded-2xl bg-foreground/90 p-4 text-background shadow-xl backdrop-blur-md transition hover:bg-foreground active:scale-95"
      >
        <div className="flex flex-col overflow-hidden text-sm">
          <span className="font-semibold text-xs text-background/70">{notification.title}</span>
          <span className="truncate font-medium">{notification.body}</span>
        </div>
        <span className="text-xs bg-background/20 px-2 py-1 rounded-full shrink-0 font-medium">查看</span>
      </div>
    </div>
  );
}
