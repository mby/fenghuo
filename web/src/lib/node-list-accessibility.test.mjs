import assert from "node:assert/strict";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { createServer } from "vite";

test("mobile list details have named controls outside the node links", async () => {
  const root = fileURLToPath(new URL("../../", import.meta.url));
  const cacheDir = await mkdtemp(join(tmpdir(), "fenghuo-node-list-test-"));
  const oldStorage = globalThis.localStorage;
  globalThis.localStorage = { getItem: () => "en" };
  const server = await createServer({
    configFile: false,
    root,
    cacheDir,
    logLevel: "silent",
    resolve: { alias: { "@": join(root, "src") } },
    server: { middlewareMode: true },
    appType: "custom",
  });

  try {
    const [{ NodeList }, { I18nProvider }, { TooltipProvider }, { demoNodes }] = await Promise.all([
      server.ssrLoadModule("/src/components/NodeList.tsx"),
      server.ssrLoadModule("/src/lib/i18n.tsx"),
      server.ssrLoadModule("/src/components/ui/tooltip.tsx"),
      server.ssrLoadModule("/src/lib/demo.ts"),
    ]);
    const now = 1_800_000_000;
    const nodes = demoNodes(now).filter((node) =>
      ["tokyo-edge", "frankfurt-backup", "sydney-pending"].includes(node.uuid),
    );
    assert.equal(nodes.length, 3);
    const html = renderToStaticMarkup(
      createElement(
        I18nProvider,
        null,
        createElement(
          TooltipProvider,
          null,
          createElement(MemoryRouter, null, createElement(NodeList, { nodes, now })),
        ),
      ),
    );

    const links = [...html.matchAll(/<a\b[^>]*>[\s\S]*?<\/a>/g)].map(([link]) => link);
    assert.equal(links.length, 3);
    for (const link of links) assert.doesNotMatch(link, /<button\b/);
    assert.match(html, /href="\/node\/tokyo-edge"/);
    assert.match(html, /aria-label="Usage details for Tokyo Edge · 01"/);
    assert.equal((html.match(/aria-label="Usage details for /g) ?? []).length, 1);
  } finally {
    await server.close();
    globalThis.localStorage = oldStorage;
    await rm(cacheDir, { recursive: true, force: true });
  }
});
