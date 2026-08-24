"use client";

import { useEffect, useState } from "react";
import { apiUrl } from "@/lib/api";
import { getOrCreateDeviceId } from "@/lib/device";

function urlBase64ToUint8Array(base64String: string) {
  const padding = "=".repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding).replace(/-/g, "+").replace(/_/g, "/");
  const rawData = window.atob(base64);
  const outputArray = new Uint8Array(rawData.length);
  for (let i = 0; i < rawData.length; ++i) {
    outputArray[i] = rawData.charCodeAt(i);
  }
  return outputArray;
}

export function usePushSubscription() {
  const [isSupported, setIsSupported] = useState(false);
  const [isSubscribed, setIsSubscribed] = useState(false);
  const [permission, setPermission] = useState<NotificationPermission>("default");
  const [isStandalone, setIsStandalone] = useState(false);
  const [isIOS, setIsIOS] = useState(false);

  useEffect(() => {
    if (typeof window === "undefined") return;

    setIsIOS(/iphone|ipad|ipod/i.test(window.navigator.userAgent));
    setIsStandalone(window.matchMedia("(display-mode: standalone)").matches);

    if ("serviceWorker" in navigator && "PushManager" in window) {
      setIsSupported(true);
      setPermission(Notification.permission);
      navigator.serviceWorker.register("/sw.js").then((reg) => {
        reg.pushManager.getSubscription().then((sub) => {
          setIsSubscribed(!!sub);
        });
      });
    }
  }, []);

  const subscribe = async () => {
    if (!isSupported) return false;

    try {
      const perm = await Notification.requestPermission();
      setPermission(perm);
      if (perm !== "granted") {
        return false;
      }

      const reg = await navigator.serviceWorker.ready;
      const keyRes = await fetch(apiUrl("/api/v1/push/vapid-public-key"), {
        credentials: "include",
      });
      const { public_key } = await keyRes.json();
      if (!public_key) return false;

      const sub = await reg.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(public_key),
      });

      const p256dh = sub.getKey ? btoa(String.fromCharCode(...new Uint8Array(sub.getKey("p256dh")!))) : "";
      const auth = sub.getKey ? btoa(String.fromCharCode(...new Uint8Array(sub.getKey("auth")!))) : "";
      const deviceId = await getOrCreateDeviceId();

      await fetch(apiUrl("/api/v1/push/subscribe"), {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          device_id: deviceId,
          endpoint: sub.endpoint,
          keys: {
            p256dh,
            auth,
          },
        }),
      });

      setIsSubscribed(true);
      return true;
    } catch (err) {
      console.error("push subscribe error:", err);
      return false;
    }
  };

  return {
    isSupported,
    isSubscribed,
    permission,
    isStandalone,
    isIOS,
    subscribe,
  };
}
