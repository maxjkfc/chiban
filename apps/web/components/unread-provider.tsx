"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";

import {
  apiFetch,
  markGroupRead as markGroupReadRequest,
  type Group,
  type User,
} from "@/lib/api";
import {
  addGroupToUnreadState,
  applyGroupReadResultIfCurrentRevision,
  applyUnreadRefresh,
  applyUnreadRefreshFailure,
  createUnreadState,
  GROUP_MESSAGE_EVENT,
  recordGlobalGroupMessage,
  recordGroupMessage,
  unreadGroupRevision,
  unreadRefreshRevision,
  type GroupMessageEventDetail,
  type UnreadState,
} from "@/lib/unread";

export type UnreadContextValue = {
  groups: Group[] | null;
  error: boolean;
  refreshGroups: (signal?: AbortSignal) => Promise<void>;
  addGroup: (group: Group) => void;
  /** Reflect a realtime message before a best-effort mark-read request. */
  noteGroupMessage: (groupId: string, isOwnMessage?: boolean) => void;
  markGroupRead: (groupId: string, messageId: string) => Promise<boolean>;
};

const UnreadContext = createContext<UnreadContextValue | null>(null);

export function UnreadProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<UnreadState>(createUnreadState);
  const stateRef = useRef(state);
  const refreshRequestId = useRef(0);
  // Read requests for different groups must not cancel one another. A user
  // can open group A, then group B before A's response arrives; ignoring A's
  // response would leave its badge stale even though the server cursor moved.
  const readRequestIds = useRef<Record<string, number>>({});

  useEffect(() => {
    stateRef.current = state;
  }, [state]);

  const refreshGroups = useCallback(async (signal?: AbortSignal) => {
    const requestId = ++refreshRequestId.current;
    const requestRevision = unreadRefreshRevision(stateRef.current);
    try {
      const loaded = await apiFetch<Group[]>("/api/v1/groups", { signal });
      setState((current) =>
        applyUnreadRefresh(
          current,
          loaded,
          requestRevision,
          requestId,
          refreshRequestId.current,
        ),
      );
    } catch (caught) {
      if (caught instanceof DOMException && caught.name === "AbortError") return;
      setState((current) =>
        applyUnreadRefreshFailure(
          current,
          requestRevision,
          requestId,
          refreshRequestId.current,
        ),
      );
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void refreshGroups(controller.signal);
    return () => controller.abort();
  }, [refreshGroups]);

  useEffect(() => {
    let cancelled = false;
    let currentUserId: string | undefined;
    const pendingEvents: GroupMessageEventDetail[] = [];

    function handleGlobalMessage(event: Event) {
      const detail = (event as CustomEvent<GroupMessageEventDetail>).detail;
      if (
        !detail ||
        typeof detail.groupId !== "string" ||
        typeof detail.messageId !== "string" ||
        typeof detail.userId !== "string"
      ) {
        return;
      }
      const userId = currentUserId;
      if (!userId) {
        pendingEvents.push(detail);
        return;
      }
      setState((current) => recordGlobalGroupMessage(current, detail, userId));
    }

    window.addEventListener(GROUP_MESSAGE_EVENT, handleGlobalMessage);

    async function subscribeToGlobalMessages() {
      try {
        const user = await apiFetch<User>("/api/v1/auth/me");
        if (cancelled) return;
        currentUserId = user.id;
        const queuedEvents = pendingEvents.splice(0);
        if (queuedEvents.length > 0) {
          setState((current) =>
            queuedEvents.reduce(
              (next, detail) => recordGlobalGroupMessage(next, detail, user.id),
              current,
            ),
          );
        }
      } catch {
        // The groups refresh remains the source of truth when identity lookup
        // is unavailable, so do not process global events without an identity.
      }
    }

    void subscribeToGlobalMessages();
    return () => {
      cancelled = true;
      window.removeEventListener(GROUP_MESSAGE_EVENT, handleGlobalMessage);
      pendingEvents.length = 0;
    };
  }, []);

  const addGroup = useCallback((group: Group) => {
    setState((current) => addGroupToUnreadState(current, group));
  }, []);

  const noteGroupMessage = useCallback(
    (groupId: string, isOwnMessage = false) => {
      setState((current) =>
        recordGroupMessage(current, groupId, isOwnMessage),
      );
    },
    [],
  );

  const markGroupRead = useCallback(async (groupId: string, messageId: string) => {
    const requestId = (readRequestIds.current[groupId] ?? 0) + 1;
    readRequestIds.current[groupId] = requestId;
    const requestRevision = unreadGroupRevision(stateRef.current, groupId);
    try {
      const updated = await markGroupReadRequest(groupId, messageId);
      // A newer message/read request has a newer cursor and must win even if
      // this response happens to return last.
      if (requestId !== readRequestIds.current[groupId]) return true;
      setState((current) =>
        applyGroupReadResultIfCurrentRevision(
          current,
          updated,
          requestRevision,
        ),
      );
      return true;
    } catch {
      // Read markers are best effort. The local realtime update remains visible
      // so a failed request cannot silently hide a new unread message.
      return false;
    }
  }, []);

  const value = useMemo(
    () => ({
      groups: state.groups,
      error: state.error,
      refreshGroups,
      addGroup,
      noteGroupMessage,
      markGroupRead,
    }),
    [state.groups, state.error, refreshGroups, addGroup, noteGroupMessage, markGroupRead],
  );

  return <UnreadContext.Provider value={value}>{children}</UnreadContext.Provider>;
}

export function useUnreadGroups(): UnreadContextValue {
  const value = useContext(UnreadContext);
  if (!value) {
    throw new Error("useUnreadGroups must be used inside UnreadProvider");
  }
  return value;
}
