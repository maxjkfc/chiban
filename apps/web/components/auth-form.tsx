"use client";

import { useRouter } from "next/navigation";
import Link from "next/link";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { apiFetch, ApiRequestError, type User } from "@/lib/api";

type Mode = "login" | "register";

const copy = {
  login: {
    title: "登入",
    submit: "登入",
    path: "/api/v1/auth/login",
    switchHref: "/register",
    switchText: "還沒有帳號？註冊",
  },
  register: {
    title: "註冊",
    submit: "建立帳號",
    path: "/api/v1/auth/register",
    switchHref: "/login",
    switchText: "已經有帳號？登入",
  },
} as const;

/**
 * Login and registration differ only in which endpoint they post to, so they
 * share one form. Both land on /today; the gate there redirects to onboarding
 * when the profile is still missing.
 */
export function AuthForm({ mode }: { mode: Mode }) {
  const router = useRouter();
  const text = copy[mode];

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: React.SyntheticEvent) {
    event.preventDefault();
    setError(null);
    setSubmitting(true);

    try {
      await apiFetch<User>(text.path, { method: "POST", body: { email, password } });
      router.replace("/today");
    } catch (caught) {
      setError(
        caught instanceof ApiRequestError ? caught.message : "無法連線，請稍後再試",
      );
      setSubmitting(false);
    }
  }

  return (
    <main className="flex flex-1 flex-col justify-center gap-6 p-6">
      <div>
        <h1 className="text-2xl font-semibold">吃伴</h1>
        <p className="text-muted-foreground text-sm">和朋友一起記錄飲食</p>
      </div>

      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <h2 className="text-lg font-medium">{text.title}</h2>

        <div className="flex flex-col gap-2">
          <Label htmlFor="email">Email</Label>
          <Input
            id="email"
            type="email"
            autoComplete="email"
            required
            value={email}
            onChange={(event) => setEmail(event.target.value)}
          />
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="password">密碼</Label>
          <Input
            id="password"
            type="password"
            autoComplete={mode === "login" ? "current-password" : "new-password"}
            required
            minLength={8}
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </div>

        {error ? (
          <p role="alert" className="text-destructive text-sm">
            {error}
          </p>
        ) : null}

        <Button type="submit" disabled={submitting}>
          {submitting ? "處理中…" : text.submit}
        </Button>
      </form>

      <Link href={text.switchHref} className="text-muted-foreground text-sm underline">
        {text.switchText}
      </Link>
    </main>
  );
}
