import type { Group } from "./api";

/** Returns the groups that should appear in the quick-access rail. */
export function pinnedGroups(groups: readonly Group[]): Group[] {
  return groups.filter((group) => group.pinned === true);
}

/** Replaces one group while keeping the list order and all other groups. */
export function replaceGroup(
  groups: readonly Group[],
  updated: Group,
): Group[] {
  return groups.map((group) => (group.id === updated.id ? updated : group));
}
