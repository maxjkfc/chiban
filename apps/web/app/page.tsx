"use client";

import { useEffect, useState } from "react";

import { apiUrl } from "@/lib/api";

type Health = "checking" | "ok" | "unreachable";

/**
 * Slice 0 placeholder. It exists to prove one thing end to end: the browser can
 * reach the Go API through the configured public base URL. Real navigation
 * (Today / Record / Groups / Me) arrives with the authenticated shell.
 */
export default function Home() {
  const [health, setHealth] = useState<Health>("checking");

  useEffect(() => {
    const controller = new AbortController();

    fetch(apiUrl("/healthz"), { signal: controller.signal })
      .then((response) => setHealth(response.ok ? "ok" : "unreachable"))
      .catch(() => setHealth("unreachable"));

    return () => controller.abort();
  }, []);

  return (
    <main className="flex flex-1 flex-col justify-center gap-3 p-6">
      <h1 className="text-2xl font-semibold">吃伴</h1>
      <p className="text-muted-foreground text-sm">和朋友一起記錄飲食</p>
      <p className="text-sm" role="status">
        API:{" "}
        {health === "checking"
          ? "檢查中…"
          : health === "ok"
            ? "已連線"
            : "無法連線"}
      </p>
    </main>
  );
}
