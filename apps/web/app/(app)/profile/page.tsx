"use client";

import { CameraIcon, ChevronRightIcon, SmileIcon } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { Avatar } from "@/components/avatar";
import { NotificationSettings } from "@/components/notification-settings";
import { Tape } from "@/components/tape";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Message } from "@/components/ui/message";
import { apiFetch, apiUrl, ApiRequestError, type Profile } from "@/lib/api";
import { cn } from "@/lib/utils";

export default function ProfilePage() {
  const router = useRouter();
  const [profile, setProfile] = useState<Profile | null>(null);
  const [avatarError, setAvatarError] = useState<string | null>(null);
  const [uploadingAvatar, setUploadingAvatar] = useState(false);
  const [displayName, setDisplayName] = useState("");
  const [timezone, setTimezone] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [status, setStatus] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    const controller = new AbortController();

    apiFetch<Profile>("/api/v1/me/profile", { signal: controller.signal })
      .then((loadedProfile) => {
        setProfile(loadedProfile);
        setDisplayName(loadedProfile.display_name);
        setTimezone(loadedProfile.timezone);
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
      setProfile(saved);
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

  // Multipart, so this cannot go through the JSON helper.
  async function handleAvatar(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    // Clear the input so picking the same file twice still fires a change.
    event.target.value = "";
    if (!file) return;

    setAvatarError(null);
    setUploadingAvatar(true);

    const form = new FormData();
    form.append("avatar", file);

    try {
      const response = await fetch(apiUrl("/api/v1/me/avatar"), {
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
      setProfile((await response.json()) as Profile);
    } catch (caught) {
      setAvatarError(
        caught instanceof ApiRequestError
          ? caught.message
          : "無法連線，請稍後再試",
      );
    } finally {
      setUploadingAvatar(false);
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
    <main className="flex flex-1 flex-col gap-4 px-5 pt-7 pb-5">
      <h1>我的</h1>

      {loaded ? (
        <div className="flex items-center gap-5">
          {/* The portrait is mounted like any other photo in this product —
              taped onto the page, name written on the white margin. */}
          <div className="polaroid relative -rotate-[2.4deg]">
            <Tape tone={4} className="-top-2.5 left-1/2 w-16 -translate-x-8 rotate-[4deg]" />
            <Avatar
              mediaId={profile?.avatar_media_id}
              displayName={displayName}
              className="size-24 rounded-[2px]"
            />
            <p className="font-heading mt-2 text-center text-sm font-black">
              {displayName || "（尚未設定暱稱）"}
            </p>
          </div>
          <div className="flex flex-col items-start gap-2">
            {/* A label, so the browser opens the picker itself. */}
            <label
              className={cn(
                buttonVariants({ variant: "outline" }),
                "cursor-pointer",
                uploadingAvatar && "pointer-events-none opacity-50",
              )}
            >
              <CameraIcon aria-hidden />
              {uploadingAvatar ? "上傳中…" : "換一張"}
              <input
                type="file"
                accept="image/*"
                className="sr-only"
                disabled={uploadingAvatar}
                onChange={handleAvatar}
              />
            </label>
            <p className="text-muted-foreground text-xs leading-relaxed">
              可以略過，
              <br />
              之後再設定。
            </p>
          </div>
        </div>
      ) : null}

      {avatarError ? <Message tone="error">{avatarError}</Message> : null}

      {loaded ? (
        <form onSubmit={handleSubmit} className="card-surface flex flex-col gap-5">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="display-name" className="text-muted-foreground text-xs tracking-[0.06em]">
              暱稱
            </Label>
            <Input
              id="display-name"
              variant="ruled"
              required
              maxLength={50}
              value={displayName}
              onChange={(event) => setDisplayName(event.target.value)}
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="timezone" className="text-muted-foreground text-xs tracking-[0.06em]">
              時區
            </Label>
            <Input
              id="timezone"
              variant="ruled"
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

      <Link
        href="/profile/stickers"
        className="card-surface flex items-center gap-3 no-underline"
      >
        <SmileIcon className="text-muted-foreground size-5" aria-hidden />
        <span className="flex-1 text-sm font-bold">我的貼圖</span>
        <ChevronRightIcon className="text-muted-foreground size-4" aria-hidden />
      </Link>

      <NotificationSettings />

      <Button variant="outline" onClick={handleLogout}>
        登出
      </Button>
    </main>
  );
}
