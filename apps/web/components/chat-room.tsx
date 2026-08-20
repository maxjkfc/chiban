"use client";

import {
  ArrowDownIcon,
  ImageIcon,
  SendHorizonalIcon,
  XIcon,
} from "lucide-react";
import Link from "next/link";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";

import { Avatar } from "@/components/avatar";
import { MealCard } from "@/components/meal-card";
import { StickerFaceIcon, StickerPicker } from "@/components/sticker-picker";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Message } from "@/components/ui/message";
import {
  apiFetch,
  ApiRequestError,
  chatMediaUrl,
  chatSocketUrl,
  stickerUrl,
  timeOfDay,
  uploadChatMedia,
  type ChatMessage,
  type GroupMember,
  type MessagePage,
  type ReactionChange,
  type SocketEvent,
  type User,
} from "@/lib/api";
import { cn } from "@/lib/utils";

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

// How close to the bottom still counts as reading the newest message. Wide
// enough to survive a fractional layout and a half-scrolled thumb, narrow
// enough that anyone who has deliberately scrolled up is left alone.
const followSlack = 64;

/**
 * Brings messages already on screen up to date with what the server just said.
 *
 * merge only ever adds, which is right for ordering but leaves a message that
 * changed while the socket was down — deleted, or reacted to — showing what it
 * looked like before. A history page is the server's current answer for every
 * message in it, so it is also the only chance to correct one.
 */
function reconcile(
  current: ChatMessage[],
  incoming: ChatMessage[],
  settling: Set<string>,
): ChatMessage[] {
  const truth = new Map(incoming.map((message) => [message.id, message]));

  let changed = false;
  const next = current.map((message) => {
    const server = truth.get(message.id);
    // A message with a reaction still in flight is deliberately ahead of the
    // server: the page was read before that reaction landed, so trusting it
    // here would wipe out what the reader just tapped.
    if (!server || settling.has(message.id) || !differs(message, server)) {
      return message;
    }
    changed = true;
    return server;
  });
  return changed ? next : current;
}

function differs(mine: ChatMessage, server: ChatMessage): boolean {
  return (
    mine.deleted !== server.deleted ||
    mine.content !== server.content ||
    mine.reply_to?.deleted !== server.reply_to?.deleted ||
    mine.reactions.length !== server.reactions.length ||
    mine.reactions.some((reaction, index) => {
      const theirs = server.reactions[index];
      return (
        reaction.reaction_type !== theirs.reaction_type ||
        reaction.count !== theirs.count ||
        reaction.mine !== theirs.mine
      );
    })
  );
}

/** Applies one person's reaction change to the message it belongs to. */
function applyReaction(
  messages: ChatMessage[],
  change: ReactionChange,
  myUserId: string | undefined,
): ChatMessage[] {
  return messages.map((message) => {
    if (message.id !== change.message_id) return message;

    const existing = message.reactions.find(
      (reaction) => reaction.reaction_type === change.reaction_type,
    );
    if (!existing && !change.added) {
      // Taking back a reaction this screen never saw arrive. There is nothing
      // to decrement, and inventing an entry would render a count of -1.
      return message;
    }
    if (
      change.user_id === myUserId &&
      (existing?.mine ?? false) === change.added
    ) {
      // Already reflected — this is the socket echoing back the change this
      // screen made itself, which must not count a second time.
      return message;
    }
    // Only the reactor knows whether it was theirs; everyone else keeps their
    // own answer to that question untouched.
    const mine = change.user_id === myUserId ? change.added : existing?.mine;
    const count = (existing?.count ?? 0) + (change.added ? 1 : -1);

    const reactions = existing
      ? message.reactions
          .map((reaction) =>
            reaction.reaction_type === change.reaction_type
              ? { ...reaction, count, mine: mine ?? false }
              : reaction,
          )
          .filter((reaction) => reaction.count > 0)
      : [
          ...message.reactions,
          {
            reaction_type: change.reaction_type,
            count,
            mine: mine ?? false,
          },
        ];

    return { ...message, reactions };
  });
}

/** Turns a message into a tombstone, including everywhere it is quoted. */
function applyDeletion(
  messages: ChatMessage[],
  messageId: string,
): ChatMessage[] {
  return messages.map((message) => {
    if (message.id === messageId) {
      return { ...message, deleted: true, content: "", reactions: [] };
    }
    if (message.reply_to?.id === messageId) {
      return {
        ...message,
        reply_to: { ...message.reply_to, deleted: true, content: "" },
      };
    }
    return message;
  });
}

type ChatRoomProps = {
  groupId: string;
  members: GroupMember[];
};

/**
 * What a quote of a message reads as.
 *
 * Only a text message has anything to quote. A picture, a sticker and a meal
 * card all store no content at all, so naming the kind is the entire preview —
 * without it the quote is an empty box and the reply answers nothing.
 */
function quoteOf(parent: {
  type: string;
  content: string;
  deleted?: boolean;
}): string {
  if (parent.deleted) return "（訊息已刪除）";
  switch (parent.type) {
    case "image":
      return "圖片";
    case "gif":
      return "GIF";
    case "sticker":
      return "貼圖";
    case "meal":
      return "一餐的紀錄";
    default:
      return parent.content;
  }
}

export function ChatRoom({ groupId, members }: ChatRoomProps) {
  // Held in state, not read straight from the prop: the list is fetched once
  // when the page loads, and someone invited a minute ago is not on it. Their
  // first message is exactly when their name is needed.
  const [roster, setRoster] = useState(members);
  // Ids already looked up, so an unresolvable one is asked about only once.
  const asked = useRef(new Set<string>());
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [before, setBefore] = useState<string | undefined>();
  const [me, setMe] = useState<User | null>(null);
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [sending, setSending] = useState(false);
  const [replyTo, setReplyTo] = useState<ChatMessage | null>(null);
  // Which message has its actions open. Tapping a bubble is how a phone gets
  // at reply and reactions without a hover state or a long-press gesture.
  const [openActions, setOpenActions] = useState<string | null>(null);
  const [reactionTypes, setReactionTypes] = useState<string[]>([]);
  // The sticker tray's contents live in the picker: the quick rail cannot draw
  // itself without them, so they are no longer loaded lazily on first open.
  const [pickerOpen, setPickerOpen] = useState(false);
  const scroller = useRef<HTMLDivElement>(null);
  // What the reader is looking at while they are not being carried along: an
  // element and how far below the scroller's top edge it sat. Content growing
  // anywhere outside it — a prepended page, a picture finishing its decode —
  // is then corrected by putting that element back where it was.
  //
  // Distance from the bottom would be simpler and is wrong: it is only
  // invariant while everything that grows is above the reader, so a message
  // arriving underneath them would push them down by its height.
  const viewAnchor = useRef<{ element: Element; top: number } | null>(null);
  // Whether the reader is at the newest message and wants to stay there.
  // Starts true so the first page lands at the bottom.
  const following = useRef(true);
  // The previous scroll offset, so a scroll event can tell "the reader moved
  // up" from "the content grew downward beneath them".
  const lastScrollTop = useRef(0);
  // The offset this component last wrote. A scroll event carrying it is this
  // component's own doing and must not be read as the reader taking over.
  const lastWritten = useRef(-1);
  // client_message_ids this room sent, each consumed by the arrival it names.
  //
  // Sending is asking to see the result, so it overrides having scrolled up.
  // Two simpler signals were both wrong. Deciding from the author of the newest
  // message cannot tell this reader from the same account on another device.
  // A boolean set after the POST resolves is armed too late and never fires:
  // the server broadcasts before it answers (internal/chat/chat.go), so the
  // socket copy usually arrives first, and the response's merge is then a no-op
  // that leaves the flag cocked for whoever speaks next.
  const sentHere = useRef(new Set<string>());
  // Bumped when the room restarts after an outage too long to bridge, so the
  // effect that scrolls runs even though the newest message may be unchanged.
  const [restarts, setRestarts] = useState(0);
  // Which page of history the room is on. A restart invalidates any older-page
  // request still in flight: merging it would splice a stretch of history onto
  // a list it has a gap from, with nothing to show the seam.
  const epoch = useRef(0);
  // The newest message the reader has actually been shown. Anything after it in
  // the list is what the unread pill counts. Undefined until the first page
  // lands, when the follow effect below sets it.
  const [seen, setSeen] = useState<string | undefined>(undefined);
  // The id of the send currently in doubt, kept with the text it belongs to so
  // a retry of the same message reuses it and an edited one does not.
  const pending = useRef<{ id: string; content: string } | null>(null);
  // An uploaded picture whose message did not get through. The upload already
  // succeeded, so retrying re-sends the message rather than the file.
  const attachment = useRef<{
    clientMessageID: string;
    mediaID: string;
    replyToID?: string;
  } | null>(null);
  // The same rule as a text message, for stickers: an unconfirmed send keeps
  // its id, so tapping the same sticker again after a lost response resolves
  // to the message that already exists instead of posting a second one.
  const pendingSticker = useRef<{ id: string; stickerID: string } | null>(null);
  // What is on screen, readable from a callback that must not depend on the
  // render it was created in. Only ever written by the effect below.
  const held = useRef<ChatMessage[]>([]);
  // The reader's own id, held in a ref rather than read from state inside the
  // socket effect: depending on it there would tear the connection down and
  // rebuild it the moment the profile finishes loading.
  const myId = useRef<string | undefined>(undefined);
  // Messages whose reaction request has not come back yet.
  const settling = useRef(new Set<string>());

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
          setMessages((current) =>
            merge(
              reconcile(current, page.messages, settling.current),
              page.messages,
              "older",
            ),
          );
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
          //
          // Everything the reader's position meant is discarded along with the
          // list, so the room restarts the way it opens: at the newest message.
          // Leaving it alone would keep a scrollTop pointing into replaced
          // content, and an unread boundary that is no longer in the array at
          // all — a pill counting messages nobody can name.
          following.current = true;
          viewAnchor.current = null;
          epoch.current += 1;
          setRestarts((count) => count + 1);
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
        setMessages((current) =>
          merge(
            reconcile(current, page.messages, settling.current),
            page.messages,
            "newer",
          ),
        );
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
    myId.current = me?.id;
  }, [me]);

  useEffect(() => {
    const controller = new AbortController();

    Promise.all([
      apiFetch<User>("/api/v1/auth/me", { signal: controller.signal }),
      loadLatest("initial", controller.signal),
      // The server decides which reactions exist; asking keeps the UI from
      // offering an emoji the API would refuse.
      apiFetch<{ reaction_types: string[] }>("/api/v1/reaction-types", {
        signal: controller.signal,
      }).then((available) => setReactionTypes(available.reaction_types)),
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
      socket.onmessage = (raw) => {
        const event = JSON.parse(raw.data as string) as SocketEvent;
        switch (event.type) {
          case "message":
            setMessages((current) => merge(current, [event.message], "newer"));
            break;
          case "reaction":
            setMessages((current) =>
              applyReaction(current, event.reaction, myId.current),
            );
            break;
          case "deleted":
            setMessages((current) => applyDeletion(current, event.message_id));
            break;
        }
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

  // One helper writes the scroll offset, so every scroll event can be asked
  // whether it came from the reader or from this component.
  const scrollToOffset = useCallback((el: HTMLDivElement, top: number) => {
    el.scrollTop = top;
    lastWritten.current = el.scrollTop;
    lastScrollTop.current = el.scrollTop;
  }, []);

  // Puts the element the reader was looking at back where it was. This is what
  // absorbs a prepended page and, afterwards, each picture in it finishing its
  // decode.
  const restoreAnchor = useCallback(
    (el: HTMLDivElement) => {
      const held = viewAnchor.current;
      if (!held || !el.contains(held.element)) return;
      const now = held.element.getBoundingClientRect().top;
      const drift = now - el.getBoundingClientRect().top - held.top;
      if (drift !== 0) scrollToOffset(el, el.scrollTop + drift);
    },
    [scrollToOffset],
  );

  // Following the conversation means staying at the newest message. Scrolling
  // up opts out: an arriving message must not snatch the screen away from
  // someone reading history. This room's own sends are the exception, and they
  // say so by name through `sentHere`.
  //
  // `restarts` is in the dependency list because the newest id is not enough.
  // A room rebuilt after an outage keeps whatever the socket delivered while
  // the page was being fetched, so its last message — and therefore `newest` —
  // can be exactly what it already was. Without its own trigger the restart
  // would only scroll if some other effect happened to move the list.
  //
  // The scroller is driven directly rather than by scrolling a sentinel into
  // view: a sentinel stops at its own edge, which leaves the list's bottom
  // padding below the fold and reads as "there is more down there".
  const newest = messages.at(-1)?.id;
  useEffect(() => {
    const el = scroller.current;
    // Checking only the newest message misses this room's own send when it
    // lands in the same commit as someone else's — two socket frames handled
    // in one React batch, both newer than what the reader had. The token still
    // deserves to be consumed and still deserves to carry the reader down; it
    // is just not necessarily last.
    let asked = false;
    for (const id of sentHere.current) {
      if (messages.some((m) => m.client_message_id === id)) {
        sentHere.current.delete(id);
        asked = true;
      }
    }
    if (!el || !(following.current || asked)) return;
    scrollToOffset(el, el.scrollHeight);
    following.current = true;
    viewAnchor.current = null;
    setSeen(newest);
    // messages is read for the scan above; the effect is meant to run once per
    // arriving commit, which `newest` already identifies, not once per
    // reference change to a list whose contents (reactions, edits) can change
    // without a new message arriving.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [newest, restarts, scrollToOffset]);

  // What arrived while the reader was up in the history. Derived from the last
  // message they were carried to rather than accumulated in a counter: a
  // counter has to be right about every path that adds messages — a socket
  // event, a reconnect replaying a page, a send — and drifts the first time one
  // of them is missed.
  //
  // A missing boundary means the list no longer contains what the reader last
  // saw. `findIndex` answers that with -1, which the arithmetic would turn into
  // the whole list — a pill claiming fifty unread messages, most of them read
  // long ago. The reconnect gap branch restarts the room instead of letting
  // that happen, so this is the guard for a case that should not arise.
  const seenIndex =
    seen === undefined ? -1 : messages.findIndex((m) => m.id === seen);
  const unread =
    seen === newest || seenIndex < 0 ? 0 : messages.length - 1 - seenIndex;

  // A page of older messages is inserted above the reader, which would push
  // what they were reading down by the height of the whole page — at the top of
  // the list that is the entire viewport and then some, so the reader lands
  // thirty messages further back than where they asked to continue from.
  //
  // Before paint: doing this in a passive effect shows the reader one frame at
  // the wrong offset.
  useLayoutEffect(() => {
    const el = scroller.current;
    if (el && !following.current) restoreAnchor(el);
  });

  // Pictures and meal cards reach their final height after the commit that
  // added them: an <img> with no reserved box is zero-high until it decodes,
  // and a MealCard renders a placeholder while it fetches. Growing content
  // dispatches no scroll event, so nothing above would run again.
  //
  // Both invariants are held here, because both are broken by the same thing.
  // A reader at the bottom is kept there; a reader up in the history keeps the
  // message they were reading, which is what makes a page of older photographs
  // load without walking them backwards.
  useEffect(() => {
    const el = scroller.current;
    const list = el?.firstElementChild;
    if (!el || !list || typeof ResizeObserver === "undefined") return;

    const observer = new ResizeObserver(() => {
      if (following.current) scrollToOffset(el, el.scrollHeight);
      else restoreAnchor(el);
    });
    observer.observe(list);
    return () => observer.disconnect();
  }, [restoreAnchor, scrollToOffset]);

  // Someone who joined after this page loaded is not on the roster, so their
  // first message would be drawn as an anonymous stranger until a reload. That
  // is the ordinary case of inviting a friend and watching them arrive, so the
  // list is refetched the moment a name is actually missing rather than polled
  // on a timer.
  useEffect(() => {
    const known = new Set(roster.map((member) => member.user_id));
    const missing = messages
      .flatMap((message) => [message.user_id, message.reply_to?.user_id])
      .filter(
        (id): id is string => !!id && !known.has(id) && !asked.current.has(id),
      );
    if (missing.length === 0) return;

    // Remembered before the request, not after: someone who has left the group
    // will never appear in the list, and without this every new message would
    // send another lookup for them.
    for (const id of missing) asked.current.add(id);

    // Deliberately not aborted when this effect re-runs. The next message
    // arriving is not a reason to give up on the lookup it started — and
    // because the id is already in `asked`, the run that replaces this one
    // finds nothing missing and would not start a replacement. Cancelling
    // here means the name never resolves at all.
    apiFetch<GroupMember[]>(`/api/v1/groups/${groupId}/members`)
      .then(setRoster)
      .catch(() => {
        // A name is a nicety. Failing to get one leaves the fallback in place
        // rather than interrupting the conversation with an error.
      });
  }, [messages, roster, groupId]);

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

    // Armed before the request, not after it. The server broadcasts inside the
    // same call that answers, so the socket copy usually wins the race; a token
    // laid down on the response would be too late for the arrival it describes
    // and would still be lying there when somebody else spoke.
    sentHere.current.add(clientMessageID);

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
          body: {
            content,
            client_message_id: clientMessageID,
            reply_to_message_id: replyTo?.id,
          },
        },
      );
      pending.current = null;
      setReplyTo(null);
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

  async function handleAttach(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    // Clear it so picking the same file twice still fires a change.
    event.target.value = "";
    if (!file || sending) return;

    // Read before the upload, not after. The reply controls stay live while
    // the picture is uploading, so a reply target read on the far side of the
    // await is whatever the composer drifted to in the meantime rather than
    // what the reader was looking at when they picked the file.
    const replyingTo = replyTo?.id;

    setError(null);
    setSending(true);
    try {
      // Two steps on purpose: the upload has to exist before a message can
      // point at it, so a failed send never leaves a message showing nothing.
      const uploaded = await uploadChatMedia(file);
      // Same retry rule as a text message: the id is kept so a send whose
      // response was lost resolves to the message that already exists rather
      // than posting the picture twice. The reply target is kept for the same
      // reason -- a retry that resolves to an already-stored message would
      // otherwise claim a reply that message does not have.
      attachment.current = {
        clientMessageID: crypto.randomUUID(),
        mediaID: uploaded.id,
        replyToID: replyingTo,
      };

      await postAttachment(attachment.current);
    } catch (caught) {
      setError(
        caught instanceof ApiRequestError
          ? caught.message
          : "圖片送不出去，請稍後再試",
      );
    } finally {
      setSending(false);
    }
  }

  // Posting an upload that already exists. Separate from picking a file so a
  // retry re-sends the same attempt instead of uploading the picture again.
  async function postAttachment(attempt: {
    clientMessageID: string;
    mediaID: string;
    replyToID?: string;
  }) {
    sentHere.current.add(attempt.clientMessageID);
    const sent = await apiFetch<ChatMessage>(
      `/api/v1/groups/${groupId}/messages`,
      {
        method: "POST",
        body: {
          client_message_id: attempt.clientMessageID,
          chat_media_id: attempt.mediaID,
          reply_to_message_id: attempt.replyToID,
        },
      },
    );
    attachment.current = null;
    // Only the reply this picture actually used is cleared. One picked while
    // the upload was in flight belongs to a message the reader has not sent
    // yet, and clearing it would throw their choice away.
    setReplyTo((current) =>
      current?.id === attempt.replyToID ? null : current,
    );
    setMessages((current) => merge(current, [sent], "newer"));
  }

  // Answers whether the sticker actually went out, so the picker knows to
  // close itself or to keep the tray up with the failure on it — an error line
  // behind an open sheet is an error nobody reads.
  async function handleSendSticker(stickerID: string): Promise<boolean> {
    if (sending) return false;

    const clientMessageID =
      pendingSticker.current?.stickerID === stickerID
        ? pendingSticker.current.id
        : crypto.randomUUID();
    pendingSticker.current = { id: clientMessageID, stickerID };

    sentHere.current.add(clientMessageID);
    setError(null);
    setSending(true);
    try {
      const sent = await apiFetch<ChatMessage>(
        `/api/v1/groups/${groupId}/messages`,
        {
          method: "POST",
          body: {
            client_message_id: clientMessageID,
            sticker_id: stickerID,
            reply_to_message_id: replyTo?.id,
          },
        },
      );
      pendingSticker.current = null;
      setReplyTo(null);
      setMessages((current) => merge(current, [sent], "newer"));
      return true;
    } catch (caught) {
      setError(
        caught instanceof ApiRequestError
          ? caught.message
          : "貼圖送不出去，再試一次",
      );
      return false;
    } finally {
      setSending(false);
    }
  }

  async function handleRetryAttachment() {
    const pendingAttachment = attachment.current;
    if (!pendingAttachment || sending) return;

    setError(null);
    setSending(true);
    try {
      await postAttachment(pendingAttachment);
    } catch (caught) {
      setError(
        caught instanceof ApiRequestError ? caught.message : "圖片還是送不出去",
      );
    } finally {
      setSending(false);
    }
  }

  async function handleLoadEarlier() {
    if (!before) return;

    setLoadingMore(true);
    const requestedAt = epoch.current;
    try {
      const page = await apiFetch<MessagePage>(
        `/api/v1/groups/${groupId}/messages?before=${encodeURIComponent(before)}`,
      );
      // A restart happened while this was in flight, so `before` was a cursor
      // into a list that no longer exists. Merging this page would splice a
      // stretch of history onto one it has a gap from, with nothing to show the
      // seam, and would wind the cursor back to where it already was.
      if (requestedAt !== epoch.current) return;

      const el = scroller.current;
      if (el) captureAnchor(el);
      setMessages((current) =>
        merge(
          reconcile(current, page.messages, settling.current),
          page.messages,
          "older",
        ),
      );
      setBefore(page.before);
    } catch {
      setError("讀取更早的訊息失敗");
    } finally {
      setLoadingMore(false);
    }
  }

  async function handleReact(message: ChatMessage, reactionType: string) {
    if (!me) return;

    const mine =
      message.reactions.find((r) => r.reaction_type === reactionType)?.mine ??
      false;
    const change: ReactionChange = {
      message_id: message.id,
      user_id: me.id,
      reaction_type: reactionType,
      added: !mine,
    };

    // Applied here rather than waiting for the round trip: a tap has to feel
    // immediate. The socket echoes this same change back, which applyReaction
    // recognises as already done.
    setMessages((current) => applyReaction(current, change, me.id));
    setOpenActions(null);
    settling.current.add(message.id);

    try {
      await apiFetch<void>(
        mine
          ? `/api/v1/messages/${message.id}/reactions/${encodeURIComponent(reactionType)}`
          : `/api/v1/messages/${message.id}/reactions`,
        mine
          ? { method: "DELETE" }
          : { method: "POST", body: { reaction_type: reactionType } },
      );
    } catch {
      setMessages((current) =>
        applyReaction(current, { ...change, added: mine }, me.id),
      );
      setError("反應沒有送出去");
    } finally {
      settling.current.delete(message.id);
    }
  }

  async function handleDelete(message: ChatMessage) {
    setOpenActions(null);
    if (!window.confirm("刪除這則訊息？回覆會保留下來。")) return;

    try {
      await apiFetch<void>(`/api/v1/messages/${message.id}`, {
        method: "DELETE",
      });
      setMessages((current) => applyDeletion(current, message.id));
    } catch {
      setError("刪除失敗，請稍後再試");
    }
  }

  const senders = new Map(roster.map((member) => [member.user_id, member]));

  function nameOf(userId: string): string {
    return senders.get(userId)?.display_name || "這位成員";
  }

  // Finds the element the reader is looking at, so later growth can be
  // measured against it. Skips the pagination button and the empty-room line:
  // neither survives a render the way a message does, and anchoring the
  // button specifically fails on the page that removes it — the reader
  // reaching the true start of history.
  function findAnchor(el: HTMLDivElement) {
    const top = el.getBoundingClientRect().top;
    const list = el.firstElementChild;
    const held = list
      ? [...list.children].find(
          (child) =>
            !child.hasAttribute("data-anchor-skip") &&
            child.getBoundingClientRect().bottom > top,
        )
      : undefined;
    return held
      ? { element: held, top: held.getBoundingClientRect().top - top }
      : null;
  }

  // Used where the anchor has to exist before the mutation it will correct:
  // captured this render, read by the layout effect committing the next one.
  function captureAnchor(el: HTMLDivElement) {
    viewAnchor.current = findAnchor(el);
  }

  // Used from scroll events, which a drag can fire several times before a
  // paint. Each capture rescans every child's geometry — real cost on a long,
  // low-end-phone conversation — so this caps it at once per frame rather than
  // once per event. The layout effect only reads `viewAnchor` on commits this
  // component itself triggers, so arriving a frame late costs nothing here the
  // way it would in `handleLoadEarlier`.
  const anchorScheduled = useRef(false);
  function captureAnchorThrottled(el: HTMLDivElement) {
    if (anchorScheduled.current) return;
    anchorScheduled.current = true;
    requestAnimationFrame(() => {
      anchorScheduled.current = false;
      if (!following.current) viewAnchor.current = findAnchor(el);
    });
  }

  // Moving up ends the follow; reaching the bottom starts it again.
  //
  // Order matters and has cost a bug each way round. Testing only "not at the
  // bottom" treats a picture finishing its decode as the reader scrolling
  // away, because the bottom moves while `scrollTop` does not. Giving "at the
  // bottom" the last word is wrong too: dragging up through the slack would
  // re-arm the follow and the next decoded image would slam the screen back
  // down, fighting the hand on it.
  //
  // The exception to "moving up opts out" is landing exactly on the bottom —
  // not within the slack, at it. iOS rubber-bands past the end and springs
  // back, and every frame of that spring-back is a decreasing `scrollTop`: by
  // the naive rule, flinging to the newest message would open with the reader
  // opting out of following it.
  //
  // Scrolls this component performed are skipped entirely, and only once:
  // this component writes `scrollTop` far more often than the reader taps the
  // exact pixel it last wrote, so the marker is consumed on the very next
  // event whether or not it matches. Leaving it standing would eventually
  // coincide with a real scroll to the same offset — landing back at the
  // bottom after reading history routinely lands on it — and silently stop
  // reacting to that scroll forever after.
  function handleScroll(event: React.UIEvent<HTMLDivElement>) {
    const el = event.currentTarget;
    const ownWrite =
      lastWritten.current !== -1 && el.scrollTop === lastWritten.current;
    lastWritten.current = -1;
    if (ownWrite) return;

    const remaining = el.scrollHeight - el.clientHeight - el.scrollTop;
    const atBottom = remaining < followSlack;
    const overshotOrAtBottom = remaining <= 0;
    const movedUp = el.scrollTop < lastScrollTop.current;
    lastScrollTop.current = el.scrollTop;

    if (movedUp && !overshotOrAtBottom) following.current = false;
    else if (atBottom) following.current = true;

    if (following.current) viewAnchor.current = null;
    else captureAnchorThrottled(el);

    // Cheap rather than free: React compares the value eagerly and skips the
    // render, but only while this hook has no update already queued. The id
    // guard keeps the call out of the queue during a flick through history.
    if (atBottom && seen !== newest) setSeen(newest);
  }

  // The pill's job: put the reader back on the newest message.
  function handleJumpToNewest() {
    const el = scroller.current;
    if (el) scrollToOffset(el, el.scrollHeight);
    following.current = true;
    viewAnchor.current = null;
    setSeen(newest);
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {/* The scroller is deliberately not a flex container.

          As a flex item the list could not grow: a column flex container
          shrinks its items to fit, `min-h-full` was satisfied at exactly the
          scroller's height, and everything past that went into overflow. With
          `justify-end` that overflow lands above the top edge — measured at
          -2268px for forty messages — which no browser lets you scroll to, so
          the history was simply gone.

          As a block child the list grows with its content, and `justify-end`
          only does something while there is free space to distribute: a short
          conversation sits on the bottom, a long one scrolls. */}
      <div
        ref={scroller}
        onScroll={handleScroll}
        className="min-h-0 flex-1 overflow-y-auto"
      >
        <ol className="flex min-h-full flex-col justify-end gap-3.5 px-4 py-4">
        {before ? (
          <li className="self-center" data-anchor-skip>
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
          <li className="text-muted-foreground m-auto text-sm" data-anchor-skip>
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
                {message.reply_to ? (
                  <blockquote
                    className={`bg-muted border-primary/50 text-muted-foreground max-w-full rounded-md border-l-2 px-2.5 py-1.5 text-xs ${
                      mine ? "text-right" : ""
                    }`}
                  >
                    <span className="font-medium">
                      {nameOf(message.reply_to.user_id)}
                    </span>
                    <span className="ms-1 line-clamp-2">
                      {quoteOf(message.reply_to)}
                    </span>
                  </blockquote>
                ) : null}

                {message.deleted ? (
                  <p className="text-muted-foreground rounded-lg border border-dashed px-3 py-2 text-sm italic">
                    （訊息已刪除）
                  </p>
                ) : message.sticker_id ? (
                  // A sticker sits on the page without a bubble: it is the
                  // whole message, and a border around it would make it look
                  // like an attachment. Tapping still opens the actions, so
                  // reply and reaction work on one like on anything else.
                  <button
                    type="button"
                    aria-expanded={openActions === message.id}
                    onClick={() =>
                      setOpenActions((open) =>
                        open === message.id ? null : message.id,
                      )
                    }
                    className="cursor-pointer"
                  >
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src={stickerUrl(message.sticker_id)}
                      alt="貼圖"
                      className="size-32 object-contain"
                    />
                  </button>
                ) : message.chat_media_id ? (
                  // An image is still an ordinary message: tapping it opens
                  // the same actions as any other, so reply and reaction work
                  // on a picture too.
                  <button
                    type="button"
                    aria-expanded={openActions === message.id}
                    onClick={() =>
                      setOpenActions((open) =>
                        open === message.id ? null : message.id,
                      )
                    }
                    className={`polaroid cursor-pointer ${mine ? "rotate-[1.2deg]" : "-rotate-[1.2deg]"}`}
                  >
                    {/* A GIF animates in an img tag; nothing re-encodes it on
                        the way here. */}
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src={chatMediaUrl(message.chat_media_id)}
                      alt="傳送的圖片"
                      className="max-h-64 w-52 rounded-[2px] object-cover"
                    />
                  </button>
                ) : message.meal_record_id ? (
                  // A meal card is still an ordinary message: it can be
                  // replied to and reacted to like any other, so the actions
                  // open the same way.
                  <button
                    type="button"
                    aria-expanded={openActions === message.id}
                    onClick={() =>
                      setOpenActions((open) =>
                        open === message.id ? null : message.id,
                      )
                    }
                    className="cursor-pointer text-start"
                  >
                    <MealCard mealId={message.meal_record_id} />
                  </button>
                ) : (
                  // The bubble is the tap target: a phone has no hover, and a
                  // long press is a gesture people have to be taught.
                  <button
                    type="button"
                    aria-expanded={openActions === message.id}
                    onClick={() =>
                      setOpenActions((open) =>
                        open === message.id ? null : message.id,
                      )
                    }
                    className={`shadow-pop cursor-pointer px-3.5 py-2.5 text-start text-sm leading-relaxed whitespace-pre-wrap ${
                      mine
                        ? "bg-primary text-primary-foreground rotate-[0.4deg] rounded-[1.125rem_1.125rem_0.375rem_1.125rem]"
                        : "bg-card border-border -rotate-[0.4deg] rounded-[1.125rem_1.125rem_1.125rem_0.375rem] border"
                    }`}
                  >
                    {message.content}
                  </button>
                )}

                {message.reactions.length > 0 ? (
                  <ul
                    className={`flex flex-wrap gap-1 ${mine ? "justify-end" : ""}`}
                  >
                    {message.reactions.map((reaction) => (
                      <li key={reaction.reaction_type}>
                        <button
                          type="button"
                          aria-pressed={reaction.mine}
                          onClick={() =>
                            handleReact(message, reaction.reaction_type)
                          }
                          className={`relative flex h-8 cursor-pointer items-center gap-1 rounded-full border px-2.5 text-xs after:absolute after:-inset-x-1 after:-inset-y-1.5 after:content-[''] ${
                            reaction.mine
                              ? "border-primary/50 bg-primary/10 text-primary-ink font-bold"
                              : "bg-card border-border"
                          }`}
                        >
                          <span aria-hidden>{reaction.reaction_type}</span>
                          <span>{reaction.count}</span>
                          <span className="sr-only">
                            {reaction.mine ? "取消這個反應" : "加上這個反應"}
                          </span>
                        </button>
                      </li>
                    ))}
                  </ul>
                ) : null}

                {openActions === message.id ? (
                  <div className="bg-card shadow-pop flex flex-wrap items-center gap-1 rounded-lg border p-1.5">
                    {reactionTypes.map((reactionType) => (
                      <button
                        key={reactionType}
                        type="button"
                        onClick={() => handleReact(message, reactionType)}
                        className="hover:bg-muted size-11 cursor-pointer rounded-full text-lg"
                      >
                        <span aria-hidden>{reactionType}</span>
                        <span className="sr-only">{`用 ${reactionType} 回應`}</span>
                      </button>
                    ))}
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => {
                        setReplyTo(message);
                        setOpenActions(null);
                      }}
                    >
                      回覆
                    </Button>
                    {message.meal_record_id ? (
                      // The card itself is the tap target for reacting, so
                      // opening the meal lives here rather than as a link
                      // nested inside that button. Styled through
                      // buttonVariants, the same as the record links on the
                      // Today page.
                      <Link
                        href={`/meals/${message.meal_record_id}`}
                        className={buttonVariants({
                          variant: "ghost",
                          size: "sm",
                        })}
                      >
                        查看紀錄
                      </Link>
                    ) : null}
                    {mine ? (
                      <Button
                        variant="destructive"
                        size="sm"
                        onClick={() => handleDelete(message)}
                      >
                        刪除
                      </Button>
                    ) : null}
                  </div>
                ) : null}
              </div>
            </li>
          );
        })}
      </ol>
      </div>

      {/* A zero-height anchor, so the pill can float over the conversation
          without taking a row from it. It sits between the scroller and
          everything below, which is why `bottom-2` lands just above whatever
          the composer currently is — an error, a reply preview, the sticker
          tray — instead of needing to know how tall that stack got. */}
      <div className="relative">
        {/* The button is not the announcement. Its label carries the count, so
            a live region wrapped around it reads "3 則新訊息", "4 則新訊息",
            "5 則新訊息" — once per arrival, interrupting a screen reader with
            the same fact over and over. This says the one thing worth saying
            and says it once: the text does not change while the pill is up, so
            nothing re-announces until it has been away and come back. */}
        <p className="sr-only" role="status" aria-live="polite">
          {unread > 0 ? "下方有新訊息" : ""}
        </p>
        {unread > 0 ? (
          <Button
            size="sm"
            onClick={handleJumpToNewest}
            className="absolute bottom-2 left-1/2 -translate-x-1/2"
          >
            <ArrowDownIcon aria-hidden />
            {unread} 則新訊息
          </Button>
        ) : null}
      </div>

      {error ? (
        <Message tone="error" className="mx-4">
          {error}
        </Message>
      ) : null}

      {attachment.current && !sending ? (
        <Button
          variant="outline"
          size="sm"
          className="mx-4"
          onClick={handleRetryAttachment}
        >
          重新送出這張圖片
        </Button>
      ) : null}

      {replyTo ? (
        <div className="bg-muted border-primary/50 mx-4 flex items-center gap-2 rounded-md border-l-2 px-3 py-2 text-xs">
          <span className="text-muted-foreground shrink-0">回覆</span>
          <span className="font-medium shrink-0">
            {nameOf(replyTo.user_id)}
          </span>
          <span className="text-muted-foreground line-clamp-1 flex-1">
            {quoteOf(replyTo)}
          </span>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => setReplyTo(null)}
            aria-label="取消回覆"
          >
            <XIcon aria-hidden />
          </Button>
        </div>
      ) : null}

      <StickerPicker
        open={pickerOpen}
        onOpenChange={setPickerOpen}
        onSend={handleSendSticker}
        sending={sending}
        // Once there is a draft, the keyboard and the text are what matter; a
        // permanent rail would cost conversation height for the rest of the chat.
        showRail={draft.trim().length === 0}
      />

      <form
        onSubmit={handleSend}
        className="border-border flex gap-2 border-t px-4 py-3"
      >
        {/* A native label opens the picker without waiting for hydration,
            the same reason the record page does it this way. */}
        <label
          className={cn(
            buttonVariants({ variant: "outline", size: "icon" }),
            "cursor-pointer",
            sending && "pointer-events-none opacity-50",
          )}
        >
          <ImageIcon aria-hidden />
          <span className="sr-only">傳送圖片</span>
          <input
            type="file"
            accept="image/*"
            className="sr-only"
            onChange={handleAttach}
          />
        </label>
        <Button
          type="button"
          variant="outline"
          size="icon"
          aria-expanded={pickerOpen}
          onClick={() => setPickerOpen(true)}
        >
          <StickerFaceIcon className="size-5" />
          <span className="sr-only">貼圖</span>
        </Button>
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
