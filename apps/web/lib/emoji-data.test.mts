import assert from "node:assert/strict";
import test from "node:test";

import {
  DEFAULT_REACTION_EMOJIS,
  EMOJI_CATEGORIES,
} from "./emoji-data.ts";

test("includes a symbol category with the common punctuation emoji", () => {
  const symbols = EMOJI_CATEGORIES.find((category) => category.id === "symbols");

  assert.ok(symbols);
  assert.equal(symbols.name, "符號");
  assert.equal(symbols.icon, "❓");
  assert.ok(symbols.emojis.length >= 30 && symbols.emojis.length <= 40);
  assert.deepEqual(
    ["❓", "❔", "⁉️", "❗", "❕", "‼️", "✅", "❌", "✔️", "✖️"].map(
      (emoji) => symbols.emojis.includes(emoji),
    ),
    Array(10).fill(true),
  );
});

test("keeps the existing category order and default reactions unchanged", () => {
  assert.deepEqual(
    EMOJI_CATEGORIES.map((category) => category.id),
    ["smileys", "gestures", "food", "activity", "objects", "symbols"],
  );
  assert.deepEqual(DEFAULT_REACTION_EMOJIS, ["❤️", "😂", "🔥", "👏", "👀"]);
});
