import assert from "node:assert/strict";
import test from "node:test";

import { isActiveNavItem, primaryNavItems } from "./navigation.mts";

test("primary navigation has an odd number of unique destinations", () => {
  assert.equal(primaryNavItems.length % 2, 1);
  assert.equal(
    new Set(primaryNavItems.map((item) => item.href)).size,
    primaryNavItems.length,
  );
});

test("Record is the centered primary action", () => {
  const center = primaryNavItems[Math.floor(primaryNavItems.length / 2)];

  assert.equal(center.href, "/record");
  assert.equal(center.primary, true);
  assert.deepEqual(
    primaryNavItems.map((item) => item.href),
    ["/groups", "/record", "/profile"],
  );
});

test("nested destinations keep their parent navigation item active", () => {
  assert.equal(isActiveNavItem("/profile/stickers", "/profile"), true);
  assert.equal(isActiveNavItem("/groups/group-1", "/groups"), true);
  assert.equal(isActiveNavItem("/recording", "/record"), false);
});