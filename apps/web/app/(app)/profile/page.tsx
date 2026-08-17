"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Message } from "@/components/ui/message";
import { apiFetch, ApiRequestError, type Profile } from "@/lib/api";

export default function ProfilePage() {
  const router = useRouter();
  const [displayName, setDisplayName] = useState("");
  const [timezone, setTimezone] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [status, setStatus] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    const controller = new AbortController();

    apiFetch<Profile>("/api/v1/me/profile", { signal: controller.signal })
      .then((profile) => {
        setDisplayName(profile.display_name);
        setTimezone(profile.timezone);
        setLoaded(true);
      })
      .catch(() => {
        if (!controller.signal.aborted) setError("讀取失敗，請重新整理");
      });

    return () => controller.abort();
  }, []);

  async function handleSubmit(event: React.SyntheticEvent) {
    event.preventDefault();
    setError(null);
    setStatus(null);
    setSubmitting(true);

    try {
      const saved = await apiFetch<Profile>("/api/v1/me/profile", {
        method: "PATCH",
        body: { display_name: displayName, timezone },
      });
      setDisplayName(saved.display_name);
      setTimezone(saved.timezone);
      setStatus("已儲存");
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

  async function handleLogout() {
    try {
      await apiFetch<void>("/api/v1/auth/logout", { method: "POST" });
    } finally {
      // Even if the request failed, sending the user to login is the honest
      // next step; the server session expires on its own.
      router.replace("/login");
    }
  }

  return (
    <main className="flex flex-1 flex-col gap-6 p-6">
      <h1>我的</h1>

      {loaded ? (
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label htmlFor="display-name">暱稱</Label>
            <Input
              id="display-name"
              required
              maxLength={50}
              value={displayName}
              onChange={(event) => setDisplayName(event.target.value)}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="timezone">時區</Label>
            <Input
              id="timezone"
              required
              value={timezone}
              onChange={(event) => setTimezone(event.target.value)}
            />
            <p className="text-muted-foreground text-xs">
              決定「今天」從幾點開始。
            </p>
          </div>

          {error ? <Message tone="error">{error}</Message> : null}
          {status ? <Message tone="success">{status}</Message> : null}

          <Button type="submit" loading={submitting}>
            {submitting ? "儲存中…" : "儲存"}
          </Button>
        </form>
      ) : (
        <Message tone={error ? "error" : "info"}>{error ?? "載入中…"}</Message>
      )}

      <Button variant="outline" onClick={handleLogout}>
        登出
      </Button>
    </main>
  );
}
