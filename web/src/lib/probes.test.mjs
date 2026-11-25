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

test("protocol presets generate independently selectable local probes", () => {
  const settings = {
    probe_targets: "CT=192.0.2.10:80,CU=198.51.100.10:80,CM=203.0.113.10:80",
    probe_interval: 30,
    probe_protocols: "CT=both,CU=icmp,CM=tcp",
  };
  const flags = probeInstallFlags(settings);
  assert.match(flags, /--ping 'CT=192\.0\.2\.10:80,CM=203\.0\.113\.10:80'/);
  assert.match(flags, /--icmp 'CT=192\.0\.2\.10,CU=198\.51\.100\.10' --icmp-interval 30s/);
  assert.doesNotMatch(flags, /CU=198\.51\.100\.10:80/);
});

test("TCP and ICMP share the selected interval for install and reconfiguration", () => {
  for (const interval of [15, 30, 60]) {
    for (const kind of ["tcp", "icmp", "both"]) {
      const settings = {
        probe_targets: "CT=192.0.2.10:80",
        probe_protocols: `CT=${kind}`,
        probe_interval: interval,
      };
      for (const command of [probeInstallFlags(settings), probeConfigureCommand(settings)]) {
        if (kind !== "icmp") assert.match(command, new RegExp(`--ping-interval ${interval}s(?: |$)`));
        else assert.doesNotMatch(command, /--ping-interval/);
        if (kind !== "tcp") assert.match(command, new RegExp(`--icmp-interval ${interval}s(?: |$)`));
        else assert.doesNotMatch(command, /--icmp-interval/);
      }
    }
  }
});
