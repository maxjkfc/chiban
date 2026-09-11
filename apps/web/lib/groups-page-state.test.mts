import assert from "node:assert/strict";
import test from "node:test";

import { pinGroup, unpinGroup, type Group } from "./api.ts";
import { pinnedGroups, replaceGroup } from "./groups-page-state.mts";
import {
  createUnreadState,
  replaceGroupInUnreadState,
} from "./unread.ts";

function group(id: string, pinned: boolean): Group {
  return {
    id,
    name: `群組 ${id}`,
    role: "member",
    is_owner: false,
    pinned,
  };
}

test("pinnedGroups keeps pinned groups in the API order", () => {
  const groups = [group("one", false), group("two", true), group("three", true)];

  assert.deepEqual(
    pinnedGroups(groups).map((item) => item.id),
    ["two", "three"],
  );
});

test("replaceGroup updates one group without dropping the rest", () => {
  const groups = [group("one", false), group("two", false)];

  assert.deepEqual(
    replaceGroup(groups, { ...groups[1], pinned: true }),
    [group("one", false), group("two", true)],
  );
});

test("the provider mutation keeps an optimistic pin visible", () => {
  const original = group("one", false);
  const state = { ...createUnreadState(), groups: [original] };

  const updated = replaceGroupInUnreadState(state, { ...original, pinned: true });

  assert.equal(updated.groups?.[0].pinned, true);
  assert.equal(updated.overrides.one.group.pinned, true);
  assert.equal(updated.revision, 1);
});

test("pin and unpin use the persisted group pin endpoints", async () => {
  const originalFetch = globalThis.fetch;
  const requests: Array<{ url: string; method: string }> = [];
  globalThis.fetch = async (input, init) => {
    requests.push({ url: String(input), method: init?.method ?? "GET" });
    return new Response(null, { status: 204 });
  };

  try {
    await pinGroup("group-1");
    await unpinGroup("group-1");
  } finally {
    globalThis.fetch = originalFetch;
  }

  assert.deepEqual(requests, [
    { url: "http://localhost:18080/api/v1/groups/group-1/pin", method: "POST" },
    { url: "http://localhost:18080/api/v1/groups/group-1/pin", method: "DELETE" },
  ]);
});
