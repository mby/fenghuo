import assert from "node:assert/strict";
import test from "node:test";
import { probeConfigureCommand, probeInstallFlags } from "./probes.ts";

test("probe template stays optional and generates local-only commands", () => {
  const empty = { probe_targets: "", probe_interval: 30 };
  assert.equal(probeInstallFlags(empty), "");
  assert.match(probeConfigureCommand(empty), /--disable$/);

  const targets = "CT=203.0.113.10:443,CU=198.51.100.2:80";
  const configured = { probe_targets: targets, probe_interval: 30 };
  assert.match(
    probeInstallFlags(configured),
    /--ping 'CT=203\.0\.113\.10:443,CU=198\.51\.100\.2:80' --ping-interval 30s$/,
  );
  assert.match(probeConfigureCommand(configured), /configure-probes\.sh \| sudo bash -s -- --ping /);
});
