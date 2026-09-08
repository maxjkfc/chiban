import type { Group } from "./api";

export type UnreadGroup = {
  has_unread?: boolean;
  unread_count?: number;
};

export type UnreadState = {
  groups: Group[] | null;
  error: boolean;
  /** Local mutations invalidate refreshes that started before them. */
  revision: number;
  /** Updates received while a groups refresh is in flight. */
  overrides: Record<string, { group: Group; revision: number }>;
};

/** Returns whether a group should show an unread indicator. */
export function groupHasUnread(group: UnreadGroup): boolean {
  return group.has_unread === true || (group.unread_count ?? 0) > 0;
}

/** Returns the newest message id in an API page ordered oldest-first. */
export function latestMessageId(
  messages: readonly { id: string }[],
): string | undefined {
  return messages.at(-1)?.id;
}

export function createUnreadState(): UnreadState {
  return { groups: null, error: false, revision: 0, overrides: {} };
}

/** Capture the version a GET /groups request must still match to be applied. */
export function unreadRefreshRevision(state: UnreadState): number {
  return state.revision;
}

/**
 * Applies a groups response only when it is still the newest request and no
 * local read/message mutation happened after that request started.
 *
 * Overrides are retained when the response was already stale. This matters
 * while the initial groups request and the first mark-read request overlap:
 * the read response can arrive first even though the groups response started
 * first and still contains the old unread count.
 */
export function applyUnreadRefresh(
  state: UnreadState,
  loaded: Group[],
  requestRevision: number,
  requestId: number,
  latestRequestId: number,
): UnreadState {
  if (requestId !== latestRequestId) return state;

  const stale = state.revision !== requestRevision;
  const nextGroups = [...loaded];
  const seen = new Set(nextGroups.map((group) => group.id));
  const nextOverrides: UnreadState["overrides"] = {};

  for (const [groupId, override] of Object.entries(state.overrides)) {
    if (override.revision <= requestRevision) continue;
    nextOverrides[groupId] = override;
    const index = nextGroups.findIndex((group) => group.id === groupId);
    if (index >= 0) {
      nextGroups[index] = override.group;
    } else if (stale && !seen.has(groupId)) {
      // Preserve an authorized group returned by a mark-read response if the
      // older list response did not include it yet.
      nextGroups.push(override.group);
      seen.add(groupId);
    }
  }

  return {
    groups: nextGroups,
    error: false,
    revision: state.revision,
    overrides: stale ? nextOverrides : {},
  };
}

/** Keeps the existing UI usable while a refresh fails. */
export function applyUnreadRefreshFailure(
  state: UnreadState,
  requestRevision: number,
  requestId: number,
  latestRequestId: number,
): UnreadState {
  if (requestId !== latestRequestId || state.revision !== requestRevision) {
    return state;
  }
  return { ...state, error: true };
}

/** Applies a successful POST /groups/{id}/read response. */
export function applyGroupReadResult(
  state: UnreadState,
  updated: Group,
): UnreadState {
  const groups = state.groups?.map((group) =>
    group.id === updated.id ? updated : group,
  ) ?? state.groups;
  const revision = state.revision + 1;
  return {
    groups,
    error: false,
    revision,
    overrides: {
      ...state.overrides,
      [updated.id]: { group: updated, revision },
    },
  };
}

/** Applies a read response only if no newer local message/mutation won the race. */
export function applyGroupReadResultIfCurrentRevision(
  state: UnreadState,
  updated: Group,
  requestRevision: number,
): UnreadState {
  return state.revision === requestRevision
    ? applyGroupReadResult(state, updated)
    : state;
}

/** Records a realtime message locally before the read request resolves. */
export function recordGroupMessage(
  state: UnreadState,
  groupId: string,
  isOwnMessage = false,
): UnreadState {
  if (isOwnMessage) return state;

  const group =
    state.overrides[groupId]?.group ??
    state.groups?.find((candidate) => candidate.id === groupId);
  if (!group) return state;

  const updated: Group = {
    ...group,
    unread_count: (group.unread_count ?? (group.has_unread ? 1 : 0)) + 1,
    has_unread: true,
  };
  const revision = state.revision + 1;
  return {
    ...state,
    groups: state.groups?.map((candidate) =>
      candidate.id === groupId ? updated : candidate,
    ) ?? state.groups,
    revision,
    overrides: {
      ...state.overrides,
      [groupId]: { group: updated, revision },
    },
  };
}

/** Adds a newly-created group without allowing a stale GET to remove it. */
export function addGroupToUnreadState(
  state: UnreadState,
  group: Group,
): UnreadState {
  const revision = state.revision + 1;
  return {
    ...state,
    groups: [...(state.groups ?? []), group],
    revision,
    overrides: {
      ...state.overrides,
      [group.id]: { group, revision },
    },
  };
}
