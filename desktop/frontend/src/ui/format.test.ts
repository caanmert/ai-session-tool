import { expect, test } from "vitest";
import type { Session } from "../api";
import { sessionCost, sessionTokens } from "./format";

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
