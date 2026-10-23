import assert from "node:assert/strict";
import test from "node:test";
import { nodeDetailBackground, nodeDetailState } from "./node-detail-route.ts";

const dashboard = { pathname: "/", search: "?demo=1", hash: "#servers", key: "home", state: { previous: "discard" } };

test("node overlay keeps its exact dashboard return location without recursive state", () => {
  const state = nodeDetailState(dashboard);
  assert.deepEqual(state.nodeBackground, { ...dashboard, state: null });
  const detail = { pathname: "/node/tokyo-edge", search: "", hash: "", key: "detail", state };
  assert.deepEqual(nodeDetailBackground(detail), { ...dashboard, state: null });
  assert.equal(nodeDetailState(detail), undefined);
});

test("direct detail links and unrelated or malformed history entries stay full pages", () => {
  const detail = { pathname: "/node/tokyo-edge", state: null };
  assert.equal(nodeDetailBackground(detail), undefined);
  for (const background of [null, "bad", {}, { pathname: "/" }, { ...dashboard, pathname: "/admin" }]) {
    assert.equal(nodeDetailBackground({ ...detail, state: { nodeBackground: background } }), undefined);
  }
  for (const pathname of ["/", "/compare", "/node/a/extra", "/admin"]) {
    assert.equal(nodeDetailBackground({ pathname, state: nodeDetailState(dashboard) }), undefined);
  }
});
