import assert from "node:assert/strict";
import test from "node:test";

import { groupHasUnread, latestMessageId } from "../lib/unread.ts";

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
