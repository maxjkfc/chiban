import assert from "node:assert/strict";
import test from "node:test";
import * as React from "react";
import { JSDOM } from "jsdom";

import { pinGroup, unpinGroup, type Group } from "./api.ts";
import { pinnedGroups, replaceGroup } from "./groups-page-state.mts";
import {
  createUnreadState,
  recordGroupMessage,
  updateGroupPinnedInUnreadState,
} from "./unread.ts";

const dom = new JSDOM("<!doctype html><html><body></body></html>", {
  url: "http://localhost:3000/groups",
});
for (const property of [
  "window",
  "document",
  "navigator",
  "HTMLElement",
  "HTMLInputElement",
  "HTMLButtonElement",
  "Node",
  "NodeFilter",
  "MutationObserver",
  "getComputedStyle",
  "requestAnimationFrame",
  "cancelAnimationFrame",
  "DOMException",
  "Event",
  "CustomEvent",
  "PointerEvent",
] as const) {
  Object.defineProperty(globalThis, property, {
    configurable: true,
    value: dom.window[property],
  });
}

const { cleanup, render, screen, waitFor } = await import(
  "@testing-library/react"
);
const { default: GroupsPage } = await import("../app/(app)/groups/page.tsx");
const { UnreadProvider } = await import("../components/unread-provider.tsx");
const { default: userEvent } = await import("@testing-library/user-event");

function renderGroupsPage() {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (input) => {
    const url = String(input);
    if (url.endsWith("/api/v1/groups")) {
      return new Response(JSON.stringify([]), {
        headers: { "Content-Type": "application/json" },
      });
    }
    if (url.endsWith("/api/v1/auth/me")) {
      return new Response(
        JSON.stringify({ id: "user-1", email: "user@example.com" }),
        { headers: { "Content-Type": "application/json" } },
      );
    }
    return new Response(null, { status: 404 });
  };
  const result = render(
    React.createElement(
      UnreadProvider,
      null,
      React.createElement(GroupsPage),
    ),
  );
  return {
    ...result,
    restoreFetch: () => {
      globalThis.fetch = originalFetch;
    },
  };
}

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

  const updated = updateGroupPinnedInUnreadState(state, original.id, true);

  assert.equal(updated.groups?.[0].pinned, true);
  assert.equal(updated.overrides.one.group.pinned, true);
  assert.equal(updated.revision, 1);
});

test("pin rollback preserves a realtime unread update received while the request is pending", () => {
  const original = group("one", false);
  const loaded = { ...createUnreadState(), groups: [original] };
  const optimisticallyPinned = updateGroupPinnedInUnreadState(
    loaded,
    original.id,
    true,
  );
  const withUnreadMessage = recordGroupMessage(optimisticallyPinned, original.id);
  const rolledBack = updateGroupPinnedInUnreadState(
    withUnreadMessage,
    original.id,
    false,
  );

  assert.equal(rolledBack.groups?.[0].pinned, false);
  assert.equal(rolledBack.groups?.[0].unread_count, 1);
  assert.equal(rolledBack.groups?.[0].has_unread, true);
});

test("create-group dialog traps forward and backward keyboard focus", async () => {
  const rendered = renderGroupsPage();
  const user = userEvent.setup();

  try {
    const trigger = screen.getByRole("button", { name: "建立群組" });
    await user.click(trigger);
    const dialog = await screen.findByRole("dialog", { name: "建立群組" });

    for (let index = 0; index < 4; index += 1) {
      await user.tab();
      assert.ok(dialog.contains(document.activeElement));
    }
    for (let index = 0; index < 4; index += 1) {
      await user.tab({ shift: true });
      assert.ok(dialog.contains(document.activeElement));
    }
  } finally {
    rendered.unmount();
    rendered.restoreFetch();
    cleanup();
  }
});

test("create-group dialog isolates the groups page from interaction", async () => {
  const rendered = renderGroupsPage();
  const user = userEvent.setup();

  try {
    await user.click(screen.getByRole("button", { name: "建立群組" }));
    await screen.findByRole("dialog", { name: "建立群組" });

    const background = document.querySelector("main")?.closest(
      '[aria-hidden="true"]',
    );
    assert.ok(background?.contains(document.querySelector("main")));
    assert.equal(background?.getAttribute("data-aria-hidden"), "true");
  } finally {
    rendered.unmount();
    rendered.restoreFetch();
    cleanup();
  }
});

test("Escape closes create-group dialog and restores focus to its trigger", async () => {
  const rendered = renderGroupsPage();
  const user = userEvent.setup();

  try {
    const trigger = screen.getByRole("button", { name: "建立群組" });
    await user.click(trigger);
    await screen.findByRole("dialog", { name: "建立群組" });
    await user.keyboard("{Escape}");

    await waitFor(() => {
      assert.equal(screen.queryByRole("dialog", { name: "建立群組" }), null);
    });
    assert.equal(document.activeElement, trigger);
  } finally {
    rendered.unmount();
    rendered.restoreFetch();
    cleanup();
  }
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
