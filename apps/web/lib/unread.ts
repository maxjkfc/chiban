export type UnreadGroup = {
  has_unread?: boolean;
  unread_count?: number;
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
