"use client";

import { SendHorizonalIcon } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import { Avatar } from "@/components/avatar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Message } from "@/components/ui/message";
import {
  apiFetch,
  ApiRequestError,
  chatSocketUrl,
  timeOfDay,
  type ChatMessage,
  type GroupMember,
  type MessagePage,
  type User,
} from "@/lib/api";

/**
 * Adds messages the list does not already have, keeping the order they came in.
 *
 * The order is the server's, never recomputed here: `created_at` is only
 * accurate to the second on the wire, so several messages in one second look
 * simultaneous to the client even though the database orders them exactly.
 * Every source arrives already ordered — a history page oldest-first, the
 * socket in broadcast order — so position is decided by where it came from.
 *
 * Deduplication is by id because the same message arrives twice by design:
 * the send response returns it and the socket pushes it to everyone, sender
 * included. Returning the current list unchanged when nothing is new keeps a
 * duplicate from re-rendering the conversation.
 */
function merge(
  current: ChatMessage[],
  incoming: ChatMessage[],
  where: "older" | "newer",
): ChatMessage[] {
  const known = new Set(current.map((message) => message.id));
  const fresh = incoming.filter((message) => !known.has(message.id));

  if (fresh.length === 0) return current;
  return where === "older" ? [...fresh, ...current] : [...current, ...fresh];
}

type ChatRoomProps = {
  groupId: string;
  members: GroupMember[];
};

export function ChatRoom({ groupId, members }: ChatRoomProps) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [before, setBefore] = useState<string | undefined>();
  const [me, setMe] = useState<User | null>(null);
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const bottom = useRef<HTMLDivElement>(null);

  const loadLatest = useCallback(
    (signal?: AbortSignal) =>
      apiFetch<MessagePage>(`/api/v1/groups/${groupId}/messages`, {
        signal,
      }).then((page) => {
        // A reconnect refetch can only bring messages missed while the socket
        // was down, and those are newer than everything already held.
        setMessages((current) => merge(current, page.messages, "newer"));
        // Only the first load defines where "earlier" starts; a reconnect
        // refetch must not rewind the button past what is already on screen.
        setBefore((current) => current ?? page.before);
      }),
    [groupId],
  );

  useEffect(() => {
    const controller = new AbortController();

    Promise.all([
      apiFetch<User>("/api/v1/auth/me", { signal: controller.signal }),
      loadLatest(controller.signal),
    ])
      .then(([user]) => setMe(user))
      .catch((caught: unknown) => {
        if (controller.signal.aborted) return;
        setError(
          caught instanceof ApiRequestError && caught.status === 403
            ? "你不是這個群組的成員"
            : "讀取訊息失敗，請重新整理",
        );
      });

    return () => controller.abort();
  }, [loadLatest]);

  // The socket carries new messages; history covers everything before it and
  // everything missed while it was down, which is why a reconnect refetches.
  useEffect(() => {
    let socket: WebSocket | null = null;
    let retry: ReturnType<typeof setTimeout> | undefined;
    let stopped = false;

    function connect(isReconnect: boolean) {
      if (stopped) return;
      socket = new WebSocket(chatSocketUrl(groupId));

      socket.onopen = () => {
        if (isReconnect) void loadLatest().catch(() => {});
      };
      socket.onmessage = (event) => {
        const message = JSON.parse(event.data as string) as ChatMessage;
        setMessages((current) => merge(current, [message], "newer"));
      };
      socket.onclose = () => {
        // A phone that locked its screen drops the socket; without this the
        // chat would look alive and silently stop receiving.
        // ponytail: fixed delay, add backoff if it ever reconnect-storms.
        if (!stopped) retry = setTimeout(() => connect(true), 2000);
      };
    }

    connect(false);

    return () => {
      stopped = true;
      clearTimeout(retry);
      socket?.close();
    };
  }, [groupId, loadLatest]);

  // Following the conversation means staying at the newest message. Keyed on
  // the newest id so loading earlier messages leaves the reader where they are
  // instead of yanking them back down.
  const newest = messages.at(-1)?.id;
  useEffect(() => {
    bottom.current?.scrollIntoView({ block: "end" });
  }, [newest]);

  async function handleSend(event: React.SyntheticEvent) {
    event.preventDefault();

    const content = draft.trim();
    if (!content) return;

    setError(null);
    setDraft("");
    try {
      const sent = await apiFetch<ChatMessage>(
        `/api/v1/groups/${groupId}/messages`,
        {
          method: "POST",
          body: { content, client_message_id: crypto.randomUUID() },
        },
      );
      setMessages((current) => merge(current, [sent], "newer"));
    } catch (caught) {
      setDraft(content);
      setError(
        caught instanceof ApiRequestError
          ? caught.message
          : "送不出去，請稍後再試",
      );
    }
  }

  async function handleLoadEarlier() {
    if (!before) return;

    setLoadingMore(true);
    try {
      const page = await apiFetch<MessagePage>(
        `/api/v1/groups/${groupId}/messages?before=${encodeURIComponent(before)}`,
      );
      setMessages((current) => merge(current, page.messages, "older"));
      setBefore(page.before);
    } catch {
      setError("讀取更早的訊息失敗");
    } finally {
      setLoadingMore(false);
    }
  }

  const senders = new Map(members.map((member) => [member.user_id, member]));

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <ol className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
        {before ? (
          <li className="self-center">
            <Button
              variant="outline"
              size="sm"
              onClick={handleLoadEarlier}
              loading={loadingMore}
            >
              載入更早的訊息
            </Button>
          </li>
        ) : null}

        {messages.length === 0 ? (
          <li className="text-muted-foreground m-auto text-sm">
            還沒有人說話，先開個頭吧。
          </li>
        ) : null}

        {messages.map((message) => {
          const mine = message.user_id === me?.id;
          const sender = senders.get(message.user_id);

          return (
            <li
              key={message.id}
              className={`flex max-w-[85%] items-end gap-2 ${
                mine ? "flex-row-reverse self-end" : "self-start"
              }`}
            >
              {!mine ? (
                <Avatar
                  mediaId={sender?.avatar_media_id}
                  displayName={sender?.display_name || "這位成員"}
                  className="size-8"
                />
              ) : null}
              <div className={`flex flex-col gap-1 ${mine ? "items-end" : ""}`}>
                {/* Name and time sit together above the bubble: split across
                    it, the name reads as if it belonged to the message above. */}
                <span className="text-muted-foreground flex gap-2 text-xs">
                  {!mine ? (
                    <span>{sender?.display_name || "（尚未設定暱稱）"}</span>
                  ) : null}
                  <time dateTime={message.created_at}>
                    {timeOfDay(message.created_at)}
                  </time>
                </span>
                <p
                  className={`rounded-2xl px-3 py-2 text-sm whitespace-pre-wrap ${
                    mine ? "bg-primary text-primary-foreground" : "bg-muted"
                  }`}
                >
                  {message.content}
                </p>
              </div>
            </li>
          );
        })}
        <div ref={bottom} />
      </ol>

      {error ? (
        <Message tone="error" className="mx-4">
          {error}
        </Message>
      ) : null}

      <form onSubmit={handleSend} className="flex gap-2 border-t p-4">
        <Input
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          placeholder="說點什麼…"
          aria-label="訊息"
          maxLength={2000}
        />
        <Button type="submit" size="icon" disabled={!draft.trim()}>
          <SendHorizonalIcon aria-hidden />
          <span className="sr-only">送出</span>
        </Button>
      </form>
    </div>
  );
}
