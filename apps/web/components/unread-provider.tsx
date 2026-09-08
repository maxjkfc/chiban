"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";

import { apiFetch, markGroupRead as markGroupReadRequest, type Group } from "@/lib/api";

export type UnreadContextValue = {
  groups: Group[] | null;
  error: boolean;
  refreshGroups: (signal?: AbortSignal) => Promise<void>;
  addGroup: (group: Group) => void;
  markGroupRead: (groupId: string, messageId: string) => Promise<void>;
};

const UnreadContext = createContext<UnreadContextValue | null>(null);

export function UnreadProvider({ children }: { children: React.ReactNode }) {
  const [groups, setGroups] = useState<Group[] | null>(null);
  const [error, setError] = useState(false);

  const refreshGroups = useCallback(async (signal?: AbortSignal) => {
    try {
      const loaded = await apiFetch<Group[]>("/api/v1/groups", { signal });
      setGroups(loaded);
      setError(false);
    } catch (caught) {
      if (caught instanceof DOMException && caught.name === "AbortError") return;
      setError(true);
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    apiFetch<Group[]>("/api/v1/groups", { signal: controller.signal })
      .then((loaded) => {
        setGroups(loaded);
        setError(false);
      })
      .catch((caught) => {
        if (controller.signal.aborted) return;
        if (caught instanceof DOMException && caught.name === "AbortError") return;
        setError(true);
      });
    return () => controller.abort();
  }, []);

  const addGroup = useCallback((group: Group) => {
    setGroups((current) => [...(current ?? []), group]);
  }, []);

  const markGroupRead = useCallback(async (groupId: string, messageId: string) => {
    try {
      const updated = await markGroupReadRequest(groupId, messageId);
      setGroups((current) =>
        current?.map((group) => (group.id === updated.id ? updated : group)) ?? current,
      );
    } catch {
      // Read markers are best effort. Keep the unread badge when the request fails.
    }
  }, []);

  const value = useMemo(
    () => ({ groups, error, refreshGroups, addGroup, markGroupRead }),
    [groups, error, refreshGroups, addGroup, markGroupRead],
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
