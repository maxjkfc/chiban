"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";

import { apiFetch, markGroupRead as markGroupReadRequest, type Group } from "@/lib/api";
import {
  addGroupToUnreadState,
  applyGroupReadResultIfCurrentRevision,
  applyUnreadRefresh,
  applyUnreadRefreshFailure,
  createUnreadState,
  recordGroupMessage,
  unreadRefreshRevision,
  type UnreadState,
} from "@/lib/unread";

export type UnreadContextValue = {
  groups: Group[] | null;
  error: boolean;
  refreshGroups: (signal?: AbortSignal) => Promise<void>;
  addGroup: (group: Group) => void;
  /** Reflect a realtime message before a best-effort mark-read request. */
  noteGroupMessage: (groupId: string, isOwnMessage?: boolean) => void;
  markGroupRead: (groupId: string, messageId: string) => Promise<void>;
};

const UnreadContext = createContext<UnreadContextValue | null>(null);

export function UnreadProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<UnreadState>(createUnreadState);
  const stateRef = useRef(state);
  const refreshRequestId = useRef(0);
  const readRequestId = useRef(0);

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
    const requestId = ++readRequestId.current;
    const requestRevision = stateRef.current.revision;
    try {
      const updated = await markGroupReadRequest(groupId, messageId);
      // A newer message/read request has a newer cursor and must win even if
      // this response happens to return last.
      if (requestId !== readRequestId.current) return;
      setState((current) =>
        applyGroupReadResultIfCurrentRevision(
          current,
          updated,
          requestRevision,
        ),
      );
    } catch {
      // Read markers are best effort. The local realtime update remains visible
      // so a failed request cannot silently hide a new unread message.
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
