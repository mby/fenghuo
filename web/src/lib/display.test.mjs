import assert from "node:assert/strict";
import test from "node:test";
import { expiryText } from "./display.ts";

const now = 1_800_000_000;
const t = (key, vars) => (vars ? `${key}:${vars.n}` : key);

test("compact expiry shortens future dates but keeps today and expired states", () => {
  assert.equal(expiryText({ expire_at: now + 120 * 86400 }, now, t), "node.expiresIn:120");
  assert.equal(expiryText({ expire_at: now + 120 * 86400 }, now, t, true), "node.expiresInShort:120");
  assert.equal(expiryText({ expire_at: now }, now, t, true), "node.expiresToday");
  assert.equal(expiryText({ expire_at: now - 86400 }, now, t, true), "node.expired");
  assert.equal(expiryText({ expire_at: 0 }, now, t, true), "");
});
