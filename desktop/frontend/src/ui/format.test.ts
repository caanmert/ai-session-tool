import { expect, test } from "vitest";
import type { Session } from "../api";
import { plural, sessionCost, sessionTokens, when } from "./format";

const session = (patch: Partial<Session>) =>
  ({
    usage: { input: 1200, output: 300, cacheRead: 46000, cacheWrite: 700 },
    ...patch,
  }) as Session;

test("tokens include cached ones", () => {
  expect(sessionTokens(session({}))).toBe("48.2K");
});

test("cost is shown only when known, and marked when partial", () => {
  expect(sessionCost(session({}))).toBe("");
  expect(sessionCost(session({ cost: 0.842 }))).toBe("$0.84");
  expect(sessionCost(session({ cost: 0.004 }))).toBe("<$0.01");
  expect(sessionCost(session({ cost: 0 }))).toBe("$0.00");
  expect(sessionCost(session({ cost: 12.4, costPartial: true }))).toBe(
    "$12.40+",
  );
});

test("counts read as one or many", () => {
  expect(plural(1, "prompt")).toBe("1 prompt");
  expect(plural(0, "prompt")).toBe("0 prompts");
  expect(plural(2, "reply", "replies")).toBe("2 replies");
});

test("every date reads one way, relative when recent", () => {
  // Wednesday 18 November 2026, 15:00 local time.
  const now = new Date(2026, 10, 18, 15, 0);
  const at = (day: number, hour: number, minute = 0, year = 2026) =>
    new Date(year, 10, day, hour, minute);
  expect(when(at(18, 14, 59, 2026), { now })).toBe("1m ago");
  expect(when(new Date(now.getTime() - 20_000), { now })).toBe("Just now");
  expect(when(at(18, 12), { now })).toBe("3h ago");
  expect(when(at(18, 2, 30), { now })).toBe("Today 02:30");
  expect(when(at(17, 18), { now })).toBe("Yesterday 18:00");
  expect(when(at(19, 9), { now })).toBe("Tomorrow 09:00");
  expect(when(at(16, 9), { now })).toBe("Mon 09:00");
  expect(when(at(23, 9), { now })).toBe("Mon 09:00");
  expect(when(at(4, 9), { now })).toBe("Nov 4");
  expect(when(at(4, 9, 0, 2025), { now })).toBe("Nov 4, 2025");
  expect(when(at(17, 18), { now, inline: true })).toBe("yesterday 18:00");
  expect(when(at(16, 9), { now, inline: true })).toBe("Mon 09:00");
  expect(when("not a date", { now })).toBe("");
});
