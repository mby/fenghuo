import assert from "node:assert/strict";
import test from "node:test";
import { trafficResetTime } from "./traffic-reset.ts";

test("billing reset tooltip uses the server's calendar and offset, not the browser timezone", () => {
  const reset = trafficResetTime(
    { next_reset_at: Date.parse("2026-10-21T16:00:00Z") / 1000, reset_utc_offset: 28800 },
    "zh",
  );
  assert.equal(reset.zone, "UTC+08:00");
  assert.match(reset.date, /2026.*10.*22.*00:00/);
  assert.equal(reset.iso, "2026-10-21T16:00:00.000Z");
  const fraction = trafficResetTime(
    { next_reset_at: Date.parse("2026-10-21T18:15:00Z") / 1000, reset_utc_offset: 20700 },
    "en",
  );
  assert.equal(fraction.zone, "UTC+05:45");
  assert.match(fraction.date, /00:00/);
  assert.equal(trafficResetTime({ next_reset_at: 1234567890, reset_utc_offset: -14400 }, "en").zone, "UTC−04:00");
});

test("unknown, hidden and invalid reset times do not invent a date", () => {
  for (const tr of [
    {},
    { next_reset_at: 0, reset_utc_offset: 0 },
    { next_reset_at: NaN, reset_utc_offset: 0 },
    { next_reset_at: 1234567890, reset_utc_offset: 999999 },
    { next_reset_at: 1e20, reset_utc_offset: 0 },
  ]) {
    assert.equal(trafficResetTime(tr, "zh"), null);
  }
});
