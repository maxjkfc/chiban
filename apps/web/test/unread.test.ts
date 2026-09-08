import assert from "node:assert/strict";
import test from "node:test";

import { markGroupRead, type Group } from "../lib/api.ts";
import {
  addGroupToUnreadState,
  applyGroupReadResult,
  applyGroupReadResultIfCurrentRevision,
  applyUnreadRefresh,
  applyUnreadRefreshFailure,
  createUnreadState,
  groupHasUnread,
  latestMessageId,
  recordGroupMessage,
  unreadRefreshRevision,
} from "../lib/unread.ts";

function group(overrides: Partial<Group> = {}): Group {
  return {
    id: "group-1",
    name: "Dinner",
    role: "member",
    is_owner: false,
    unread_count: 2,
    has_unread: true,
    ...overrides,
  };
}

test("groupHasUnread treats either unread API signal as unread", () => {
  assert.equal(groupHasUnread({ has_unread: true, unread_count: 0 }), true);
  assert.equal(groupHasUnread({ has_unread: false, unread_count: 3 }), true);
  assert.equal(groupHasUnread({ has_unread: false, unread_count: 0 }), false);
});

test("groupHasUnread fails closed for missing or invalid unread fields", () => {
  assert.equal(groupHasUnread({}), false);
  assert.equal(groupHasUnread({ has_unread: undefined, unread_count: -1 }), false);
});

test("latestMessageId returns the newest message in an API page", () => {
  assert.equal(latestMessageId([{ id: "old" }, { id: "new" }]), "new");
  assert.equal(latestMessageId([]), undefined);
});

test("markGroupRead sends the merged API contract and returns the updated group", async () => {
  const originalFetch = globalThis.fetch;
  let request: { url: string; init?: RequestInit } | undefined;
  globalThis.fetch = async (input, init) => {
    request = { url: String(input), init };
    return new Response(JSON.stringify(group({ unread_count: 0, has_unread: false })), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  };

  try {
    const updated = await markGroupRead("group-1", "message-9");
    assert.equal(updated.has_unread, false);
    assert.equal(request?.url, "http://localhost:18080/api/v1/groups/group-1/read");
    assert.equal(request?.init?.method, "POST");
    assert.deepEqual(JSON.parse(String(request?.init?.body)), {
      message_id: "message-9",
    });
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("a successful mark-read result clears the provider badge", () => {
  const loaded = applyUnreadRefresh(
    createUnreadState(),
    [group()],
    0,
    1,
    1,
  );
  const cleared = applyGroupReadResult(
    loaded,
    group({ unread_count: 0, has_unread: false }),
  );

  assert.equal(cleared.groups?.[0].has_unread, false);
  assert.equal(cleared.groups?.[0].unread_count, 0);
});

test("a stale groups response cannot resurrect a badge after mark-read", () => {
  const initial = createUnreadState();
  const requestRevision = unreadRefreshRevision(initial);
  const afterRead = applyGroupReadResult(
    initial,
    group({ unread_count: 0, has_unread: false }),
  );
  const resolved = applyUnreadRefresh(
    afterRead,
    [group()],
    requestRevision,
    1,
    1,
  );

  assert.equal(resolved.groups?.[0].has_unread, false);
  assert.equal(resolved.groups?.[0].unread_count, 0);
});

test("a late mark-read response cannot clear a newer realtime unread message", () => {
  const loaded = applyUnreadRefresh(
    createUnreadState(),
    [group({ unread_count: 0, has_unread: false })],
    0,
    1,
    1,
  );
  const requestRevision = loaded.revision;
  const withNewMessage = recordGroupMessage(loaded, "group-1");
  const resolved = applyGroupReadResultIfCurrentRevision(
    withNewMessage,
    group({ unread_count: 0, has_unread: false }),
    requestRevision,
  );

  assert.strictEqual(resolved, withNewMessage);
  assert.equal(resolved.groups?.[0].has_unread, true);
});

test("a realtime message sets unread and a sender's own message does not", () => {
  const loaded = applyUnreadRefresh(
    createUnreadState(),
    [group({ unread_count: 0, has_unread: false })],
    0,
    1,
    1,
  );
  const incoming = recordGroupMessage(loaded, "group-1");
  const own = recordGroupMessage(incoming, "group-1", true);

  assert.equal(incoming.groups?.[0].has_unread, true);
  assert.equal(incoming.groups?.[0].unread_count, 1);
  assert.strictEqual(own, incoming);
});

test("a realtime message before the initial refresh remains unread", () => {
  const initial = createUnreadState();
  const duringRefresh = recordGroupMessage(initial, "group-1");
  const resolved = applyUnreadRefresh(
    duringRefresh,
    [group({ unread_count: 0, has_unread: false })],
    0,
    1,
    1,
  );

  assert.equal(resolved.groups?.[0].has_unread, true);
  assert.equal(resolved.groups?.[0].unread_count, 1);
  assert.deepEqual(resolved.pendingUnread, {});
});

test("mark-read clears a pending realtime message before refresh completion", () => {
  const duringRefresh = recordGroupMessage(createUnreadState(), "group-1");
  const markedRead = applyGroupReadResult(
    duringRefresh,
    group({ unread_count: 0, has_unread: false }),
  );
  const resolved = applyUnreadRefresh(markedRead, [group()], 0, 1, 1);

  assert.equal(resolved.groups?.[0].has_unread, false);
  assert.equal(resolved.groups?.[0].unread_count, 0);
  assert.deepEqual(resolved.pendingUnread, {});
});

test("refresh failure keeps the existing groups as a graceful fallback", () => {
  const loaded = applyUnreadRefresh(
    createUnreadState(),
    [group({ unread_count: 0, has_unread: false })],
    0,
    1,
    1,
  );
  const failed = applyUnreadRefreshFailure(loaded, 0, 2, 2);

  assert.equal(failed.error, true);
  assert.deepEqual(failed.groups, loaded.groups);
});

test("new groups stay visible after a refresh started before creation", () => {
  const initial = applyUnreadRefresh(
    createUnreadState(),
    [group()],
    0,
    1,
    1,
  );
  const requestRevision = unreadRefreshRevision(initial);
  const withNewGroup = addGroupToUnreadState(initial, group({ id: "group-2", name: "Lunch" }));
  const resolved = applyUnreadRefresh(
    withNewGroup,
    [group()],
    requestRevision,
    2,
    2,
  );

  assert.deepEqual(resolved.groups?.map((item) => item.id), ["group-1", "group-2"]);
});
