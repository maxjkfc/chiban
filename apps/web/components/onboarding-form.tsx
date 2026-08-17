"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  apiFetch,
  ApiRequestError,
  detectTimezone,
  type Profile,
} from "@/lib/api";

/** Only same-site paths, so a crafted ?next= cannot bounce users off-site. */
function safeNext(next: string | null): string | null {
  if (!next || !next.startsWith("/") || next.startsWith("//")) return null;
  return next;
}

/**
 * V0.1 onboarding asks for a nickname and nothing else the user has to think
 * about: the timezone is prefilled from the browser, and there is no height,
 * weight or calorie goal. The avatar is deliberately absent — it arrives with
 * the media pipeline, and requiring it here would slow down the first record.
 */
export function OnboardingForm() {
  const router = useRouter();
  const next = useSearchParams().get("next");
  const [displayName, setDisplayName] = useState("");
  const [timezone, setTimezone] = useState(detectTimezone);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: React.SyntheticEvent) {
    event.preventDefault();
    setError(null);
    setSubmitting(true);

    try {
      await apiFetch<Profile>("/api/v1/me/profile", {
        method: "PATCH",
        body: { display_name: displayName, timezone },
      });
      router.replace(safeNext(next) ?? "/today");
    } catch (caught) {
      if (caught instanceof ApiRequestError && caught.status === 401) {
        router.replace("/login");
        return;
      }
      setError(
        caught instanceof ApiRequestError ? caught.message : "無法連線，請稍後再試",
      );
      setSubmitting(false);
    }
  }

  return (
    <main className="flex flex-1 flex-col justify-center gap-6 p-6">
      <div>
        <h1 className="text-2xl font-semibold">設定暱稱</h1>
        <p className="text-muted-foreground text-sm">朋友會在群組裡看到這個名字。</p>
      </div>

      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <div className="flex flex-col gap-2">
          <Label htmlFor="display-name">暱稱</Label>
          <Input
            id="display-name"
            required
            maxLength={50}
            autoComplete="nickname"
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
            決定「今天」從幾點開始。已依裝置設定填好。
          </p>
        </div>

        {error ? (
          <p role="alert" className="text-destructive text-sm">
            {error}
          </p>
        ) : null}

        <Button type="submit" disabled={submitting}>
          {submitting ? "儲存中…" : "開始使用"}
        </Button>
      </form>
    </main>
  );
}
