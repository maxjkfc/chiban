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

// How many failed handshakes in a row before the chat stops trying. Enough
// tries, with the backoff below, to ride out about a minute offline.
const maxReconnectAttempts = 6;

/** Doubling backoff, so a server refusing on purpose is not hammered. */
function reconnectDelay(failures: number): number {
  return Math.min(2000 * 2 ** (failures - 1), 30_000);
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
  const [sending, setSending] = useState(false);
  const bottom = useRef<HTMLDivElement>(null);
  // The id of the send currently in doubt, kept with the text it belongs to so
  // a retry of the same message reuses it and an edited one does not.
  const pending = useRef<{ id: string; content: string } | null>(null);
  // What is on screen, readable from a callback that must not depend on the
  // render it was created in. Only ever written by the effect below.
  const held = useRef<ChatMessage[]>([]);

  const loadLatest = useCallback(
    (mode: "initial" | "reconnect", signal?: AbortSignal) => {
      // Snapshot now, not when the response lands: the socket keeps delivering
      // while this request is in flight, and a message that arrives meanwhile
      // would otherwise look like proof that no gap opened during the outage.
      const beforeFetch = new Set(held.current.map((message) => message.id));
      // The last message as of the snapshot. Live deliveries land after it and
      // "load earlier" prepends before it, so this marks exactly where what
      // arrived during the request begins — which "not in the snapshot" does
      // not, since that would also catch history the reader paged back into.
      const newestBeforeFetch = held.current.at(-1)?.id;

      return apiFetch<MessagePage>(`/api/v1/groups/${groupId}/messages`, {
        signal,
      }).then((page) => {
        if (mode === "initial") {
          // Anything already held arrived on the socket while this request was
          // in flight, so it is newer than every message in this page.
          setMessages((current) => merge(current, page.messages, "older"));
          setBefore(page.before);
          return;
        }

        const overlaps = page.messages.some((message) =>
          beforeFetch.has(message.id),
        );
        if (beforeFetch.size > 0 && !overlaps) {
          // Nothing in the newest page was on screen when the outage ended, so
          // more was said during it than one page holds and the two ends are
          // separated by a gap this page cannot bridge. Starting again from
          // this page keeps history walkable: its cursor leads back across the
          // gap, where merging would strand those messages out of reach.
          setMessages((current) => {
            // Whatever the socket delivered while this request was in flight
            // is newer than this page, so it survives the restart.
            const anchor = current.findIndex(
              (message) => message.id === newestBeforeFetch,
            );
            const arrivedSince = anchor < 0 ? [] : current.slice(anchor + 1);
            return merge(page.messages, arrivedSince, "newer");
          });
          setBefore(page.before);
          return;
        }
        setMessages((current) => merge(current, page.messages, "newer"));
        // The button walks back from where the first page ended; a reconnect
        // must not rewind it past what is already on screen.
        setBefore((current) => current ?? page.before);
      });
    },
    [groupId],
  );

  useEffect(() => {
    held.current = messages;
  }, [messages]);

  useEffect(() => {
    const controller = new AbortController();

    Promise.all([
      apiFetch<User>("/api/v1/auth/me", { signal: controller.signal }),
      loadLatest("initial", controller.signal),
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
    let failures = 0;
    let backfilling = false;
    let backfillAgain = false;

    // One backfill at a time. Two overlapping ones would work from the same
    // snapshot, and the first to finish would replace the list out from under
    // the second — taking the anchor its suffix depends on with it. A drop
    // that happens mid-backfill is not lost: it queues one more pass, which
    // takes its own fresh snapshot.
    function backfill() {
      if (backfilling) {
        backfillAgain = true;
        return;
      }
      backfilling = true;
      void loadLatest("reconnect")
        .catch(() => {})
        .finally(() => {
          backfilling = false;
          if (backfillAgain && !stopped) {
            backfillAgain = false;
            backfill();
          }
        });
    }

    function connect(isReconnect: boolean) {
      if (stopped) return;
      socket = new WebSocket(chatSocketUrl(groupId));

      socket.onopen = () => {
        failures = 0;
        if (isReconnect) backfill();
      };
      socket.onmessage = (event) => {
        const message = JSON.parse(event.data as string) as ChatMessage;
        setMessages((current) => merge(current, [message], "newer"));
      };
      socket.onclose = () => {
        // A phone that locked its screen drops the socket; without a retry the
        // chat would look alive and silently stop receiving.
        if (stopped) return;

        failures += 1;
        if (failures >= maxReconnectAttempts) {
          // The browser cannot see why a handshake failed, so a refusal that
          // will never succeed — the server no longer counts this user as a
          // member — looks exactly like a flaky network. Giving up after
          // enough tries turns a silent forever-loop of 403s into something
          // the reader can act on.
          setError("連線中斷了，重新整理看看");
          return;
        }
        retry = setTimeout(() => connect(true), reconnectDelay(failures));
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
    if (!content || sending) return;

    // A send whose response never arrived may well have been stored. Retrying
    // it under the same id resolves to that message; a fresh id would post a
    // second copy and defeat the whole point of the client id.
    const clientMessageID =
      pending.current?.content === content
        ? pending.current.id
        : crypto.randomUUID();
    pending.current = { id: clientMessageID, content };

    // One send at a time. Overlapping sends would each want the retry slot,
    // and the loser would lose the id its retry depends on.
    setSending(true);
    setError(null);
    setDraft("");
    try {
      const sent = await apiFetch<ChatMessage>(
        `/api/v1/groups/${groupId}/messages`,
        {
          method: "POST",
          body: { content, client_message_id: clientMessageID },
        },
      );
      pending.current = null;
      setMessages((current) => merge(current, [sent], "newer"));
    } catch (caught) {
      setDraft(content);
      setError(
        caught instanceof ApiRequestError
          ? caught.message
          : "送不出去，請稍後再試",
      );
    } finally {
      setSending(false);
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
        <Button type="submit" size="icon" disabled={!draft.trim() || sending}>
          <SendHorizonalIcon aria-hidden />
          <span className="sr-only">送出</span>
        </Button>
      </form>
    </div>
  );
}
