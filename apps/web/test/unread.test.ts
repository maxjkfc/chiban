import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";

import { markGroupRead, type Group } from "../lib/api.ts";
import {
  addGroupToUnreadState,
  applyGroupReadResult,
  applyGroupReadResultIfCurrentRevision,
  applyUnreadRefresh,
  applyUnreadRefreshFailure,
  createUnreadState,
  GROUP_MESSAGE_EVENT,
  groupHasUnread,
  latestMessageId,
  recordGroupMessage,
  recordGlobalGroupMessage,
  unreadGroupRevision,
  unreadRefreshRevision,
  type GroupMessageEventDetail,
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

type UnreadContextValue = {
  groups: Group[] | null;
};

type HookSlot = {
  value: unknown;
  deps?: readonly unknown[];
  cleanup?: (() => void) | undefined;
};

/**
 * Mounts the real provider with a tiny hook runner so this node:test suite can
 * exercise browser effects without adding a second DOM/test-renderer stack.
 */
function mountUnreadProviderForTest() {
  const appRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
  const providerPath = resolve(appRoot, "components/unread-provider.tsx");
  const nodeRequire = createRequire(import.meta.url);
  const nodeModule = nodeRequire("node:module") as {
    _load: (...args: [string, object | null, boolean]) => unknown;
    _resolveFilename: (...args: [string, object | null, boolean]) => string;
  };
  const originalLoad = nodeModule._load;
  const originalResolve = nodeModule._resolveFilename;
  const originalTsxLoader = nodeRequire.extensions[".tsx"];
  const slots: HookSlot[] = [];
  let hookIndex = 0;
  let mounted = false;

  function dependenciesChanged(previous: readonly unknown[] | undefined, next: readonly unknown[]) {
    return previous === undefined || previous.length !== next.length ||
      next.some((dependency, index) => !Object.is(dependency, previous[index]));
  }

  const fakeReact = {
    createContext(defaultValue: unknown) {
      return { current: defaultValue, Provider: Symbol("provider") };
    },
    useCallback<T extends (...args: never[]) => unknown>(callback: T, deps: readonly unknown[]) {
      const slot = slots[hookIndex] ?? { value: callback };
      if (!mounted || dependenciesChanged(slot.deps, deps)) {
        slot.value = callback;
        slot.deps = deps;
      }
      slots[hookIndex++] = slot;
      return slot.value as T;
    },
    useContext<T>(context: { current: T }) {
      return context.current;
    },
    useEffect(effect: () => (() => void) | void, deps: readonly unknown[]) {
      const slot = slots[hookIndex] ?? { value: undefined };
      if (!mounted || dependenciesChanged(slot.deps, deps)) {
        slot.cleanup?.();
        slot.cleanup = effect() ?? undefined;
        slot.deps = deps;
      }
      slots[hookIndex++] = slot;
    },
    useMemo<T>(factory: () => T, deps: readonly unknown[]) {
      const slot = slots[hookIndex] ?? { value: undefined };
      if (!mounted || dependenciesChanged(slot.deps, deps)) {
        slot.value = factory();
        slot.deps = deps;
      }
      slots[hookIndex++] = slot;
      return slot.value as T;
    },
    useRef<T>(value: T) {
      const slot = slots[hookIndex] ?? { value: { current: value } };
      slots[hookIndex++] = slot;
      return slot.value as { current: T };
    },
    useState<T>(initial: T | (() => T)) {
      const slot = slots[hookIndex] ?? {
        value: typeof initial === "function" ? (initial as () => T)() : initial,
      };
      const slotIndex = hookIndex++;
      slots[slotIndex] = slot;
      return [
        slot.value as T,
        (update: T | ((current: T) => T)) => {
          slot.value = typeof update === "function"
            ? (update as (current: T) => T)(slot.value as T)
            : update;
        },
      ] as const;
    },
  };

  nodeModule._resolveFilename = (...args) => {
    const [request, parent] = args;
    const parentFilename = (parent as { filename?: string } | null)?.filename;
    const basePath = request.startsWith("@/")
      ? resolve(appRoot, request.slice(2))
      : parentFilename && request.startsWith(".")
        ? resolve(dirname(parentFilename), request)
        : undefined;
    if (basePath) {
      for (const extension of [".ts", ".tsx"]) {
        try {
          readFileSync(`${basePath}${extension}`);
          return `${basePath}${extension}`;
        } catch {
          // Try the next TypeScript extension.
        }
      }
    }
    return originalResolve(...args);
  };
  nodeRequire.extensions[".tsx"] = (module, filename) => {
    const source = readFileSync(filename, "utf8");
    const output = ts.transpileModule(source, {
      compilerOptions: {
        esModuleInterop: true,
        jsx: ts.JsxEmit.ReactJSX,
        module: ts.ModuleKind.CommonJS,
        target: ts.ScriptTarget.ES2022,
      },
      fileName: filename,
    }).outputText;
    (module as typeof module & { _compile: (code: string, filename: string) => void })
      ._compile(output, filename);
  };
  nodeModule._load = (...args) => {
    const [request] = args;
    if (request === "react") return fakeReact;
    if (request === "react/jsx-runtime") {
      return { jsx: (type: unknown, props: unknown) => ({ type, props }) };
    }
    return originalLoad(...args);
  };

  let provider: (props: { children: null }) => { props: { value: UnreadContextValue } };
  try {
    const loaded = nodeRequire(providerPath) as { UnreadProvider: typeof provider };
    provider = loaded.UnreadProvider;
  } finally {
    nodeModule._load = originalLoad;
    nodeModule._resolveFilename = originalResolve;
    if (originalTsxLoader) nodeRequire.extensions[".tsx"] = originalTsxLoader;
    else delete nodeRequire.extensions[".tsx"];
  }

  function render() {
    hookIndex = 0;
    const output = provider({ children: null });
    mounted = true;
    return output.props.value;
  }

  function unmount() {
    for (const slot of slots) slot.cleanup?.();
  }

  return { render, unmount };
}

function dispatchGroupMessage(detail: GroupMessageEventDetail) {
  const event = new CustomEvent<GroupMessageEventDetail>(GROUP_MESSAGE_EVENT, {
    detail,
  });
  window.dispatchEvent(event);
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

test("a global websocket message updates a group outside the open room", () => {
  const loaded = applyUnreadRefresh(
    createUnreadState(),
    [
      group({ id: "group-1", unread_count: 0, has_unread: false }),
      group({ id: "group-2", name: "Lunch", unread_count: 0, has_unread: false }),
    ],
    0,
    1,
    1,
  );

  const updated = recordGlobalGroupMessage(loaded, {
    groupId: "group-2",
    messageId: "message-2",
    userId: "member-2",
  }, "current-user");

  assert.equal(
    updated.groups?.find((item) => item.id === "group-1")?.has_unread,
    false,
  );
  assert.equal(
    updated.groups?.find((item) => item.id === "group-2")?.has_unread,
    true,
  );
  assert.equal(
    updated.groups?.find((item) => item.id === "group-2")?.unread_count,
    1,
  );
});

test("the global websocket path ignores a message from the authenticated user", () => {
  const loaded = applyUnreadRefresh(
    createUnreadState(),
    [group({ unread_count: 0, has_unread: false })],
    0,
    1,
    1,
  );
  const ownMessage = recordGlobalGroupMessage(
    loaded,
    {
      groupId: "group-1",
      messageId: "own-message",
      userId: "current-user",
    },
    "current-user",
  );
  const incomingMessage = recordGlobalGroupMessage(
    loaded,
    {
      groupId: "group-1",
      messageId: "incoming-message",
      userId: "member-2",
    },
    "current-user",
  );

  assert.strictEqual(ownMessage, loaded);
  assert.equal(incomingMessage.groups?.[0].has_unread, true);
  assert.equal(incomingMessage.groups?.[0].unread_count, 1);
});

test("UnreadProvider filters own global messages before and after auth resolves", async () => {
  const originalFetch = globalThis.fetch;
  const originalWindow = globalThis.window;
  const listeners = new Map<string, Set<(event: Event) => void>>();
  const windowMock = {
    addEventListener(type: string, listener: (event: Event) => void) {
      const typeListeners = listeners.get(type) ?? new Set();
      typeListeners.add(listener);
      listeners.set(type, typeListeners);
    },
    removeEventListener(type: string, listener: (event: Event) => void) {
      listeners.get(type)?.delete(listener);
    },
    dispatchEvent(event: Event) {
      listeners.get(event.type)?.forEach((listener) => listener(event));
      return true;
    },
  } as unknown as Window & typeof globalThis;
  globalThis.window = windowMock;

  let resolveAuth: ((response: Response) => void) | undefined;
  globalThis.fetch = async (input) => {
    const url = String(input);
    if (url.endsWith("/api/v1/groups")) {
      return new Response(
        JSON.stringify([group({ unread_count: 0, has_unread: false })]),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    }
    if (url.endsWith("/api/v1/auth/me")) {
      return new Promise<Response>((resolve) => {
        resolveAuth = resolve;
      });
    }
    throw new Error(`Unexpected fetch: ${url}`);
  };

  const provider = mountUnreadProviderForTest();
  const flush = () => new Promise<void>((resolve) => setImmediate(resolve));

  try {
    provider.render();
    await flush();
    provider.render();
    assert.ok(resolveAuth);

    dispatchGroupMessage({
      groupId: "group-1",
      messageId: "own-before-auth",
      userId: "current-user",
    });
    assert.equal(provider.render().groups?.[0].unread_count, 0);
    assert.equal(provider.render().groups?.[0].has_unread, false);

    resolveAuth!(
      new Response(JSON.stringify({ id: "current-user", email: "me@example.com" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    await flush();
    assert.equal(provider.render().groups?.[0].unread_count, 0);
    assert.equal(provider.render().groups?.[0].has_unread, false);

    dispatchGroupMessage({
      groupId: "group-1",
      messageId: "own-after-auth",
      userId: "current-user",
    });
    assert.equal(provider.render().groups?.[0].unread_count, 0);
    assert.equal(provider.render().groups?.[0].has_unread, false);

    dispatchGroupMessage({
      groupId: "group-1",
      messageId: "incoming-after-auth",
      userId: "member-2",
    });
    assert.equal(provider.render().groups?.[0].unread_count, 1);
    assert.equal(provider.render().groups?.[0].has_unread, true);
  } finally {
    provider.unmount();
    globalThis.fetch = originalFetch;
    globalThis.window = originalWindow;
  }
});

test("a message in another group does not invalidate a pending read", () => {
  const loaded = applyUnreadRefresh(
    createUnreadState(),
    [
      group({ id: "group-1", unread_count: 2, has_unread: true }),
      group({ id: "group-2", name: "Lunch", unread_count: 0, has_unread: false }),
    ],
    0,
    1,
    1,
  );
  const requestRevision = unreadGroupRevision(loaded, "group-1");
  const withOtherGroupMessage = recordGroupMessage(loaded, "group-2");
  const resolved = applyGroupReadResultIfCurrentRevision(
    withOtherGroupMessage,
    group({ id: "group-1", unread_count: 0, has_unread: false }),
    requestRevision,
  );

  assert.equal(resolved.groups?.find((item) => item.id === "group-1")?.has_unread, false);
  assert.equal(resolved.groups?.find((item) => item.id === "group-2")?.has_unread, true);
});

test("a stale refresh cannot resurrect a read cursor after another group changes", () => {
  const loaded = applyUnreadRefresh(
    createUnreadState(),
    [
      group({ id: "group-1", unread_count: 1, has_unread: true }),
      group({ id: "group-2", name: "Lunch", unread_count: 0, has_unread: false }),
    ],
    0,
    1,
    1,
  );
  const refreshRevision = unreadRefreshRevision(loaded);
  const read = applyGroupReadResult(
    loaded,
    group({ id: "group-1", unread_count: 0, has_unread: false }),
  );
  const withOtherGroupMessage = recordGlobalGroupMessage(read, {
    groupId: "group-2",
    messageId: "message-2",
    userId: "member-2",
  }, "current-user");

  const resolved = applyUnreadRefresh(
    withOtherGroupMessage,
    [
      group({ id: "group-1", unread_count: 1, has_unread: true }),
      group({ id: "group-2", name: "Lunch", unread_count: 0, has_unread: false }),
    ],
    refreshRevision,
    2,
    2,
  );

  assert.equal(resolved.groups?.find((item) => item.id === "group-1")?.has_unread, false);
  assert.equal(resolved.groups?.find((item) => item.id === "group-2")?.has_unread, true);
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

test("a mark-read request started after its realtime message can clear that message", () => {
  const loaded = applyUnreadRefresh(
    createUnreadState(),
    [group({ unread_count: 0, has_unread: false })],
    0,
    1,
    1,
  );
  const afterMessage = recordGroupMessage(loaded, "group-1");
  const requestRevision = unreadGroupRevision(afterMessage, "group-1");
  const resolved = applyGroupReadResultIfCurrentRevision(
    afterMessage,
    group({ unread_count: 0, has_unread: false }),
    requestRevision,
  );

  assert.equal(resolved.groups?.[0].has_unread, false);
  assert.equal(resolved.groups?.[0].unread_count, 0);
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
