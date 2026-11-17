import { describe, expect, it } from "vitest";
import { duration, tokenCount, turnSummary } from "./turnStats";

describe("turn figures", () => {
  it("reads durations and token counts briefly", () => {
    expect(duration(400)).toBe("0s");
    expect(duration(45_000)).toBe("45s");
    expect(duration(134_000)).toBe("2m 14s");
    expect(duration(3_900_000)).toBe("1h 05m");
    expect(tokenCount(820)).toBe("820");
    expect(tokenCount(4_000)).toBe("4k");
    expect(tokenCount(4_250)).toBe("4.3k");
    expect(tokenCount(48_100)).toBe("48k");
    expect(tokenCount(1_200_000)).toBe("1.2M");
  });

  it("shows only what the agent reported", () => {
    expect(turnSummary({ ms: 134_000 }, "Codex")).toEqual({
      text: "Worked 2m 14s",
      detail: "This turn ran for 2m 14s. Codex reported no token use for it.",
    });
    const claude = turnSummary(
      { ms: 9_000, tokens: 45_600, costUsd: 0.4234 },
      "Claude Code",
    );
    expect(claude.text).toBe("Worked 9s · 46k tokens");
    expect(claude.detail).toContain("45,600 tokens");
    expect(claude.detail).toContain(
      "estimates $0.42 at API prices; on a subscription it is not charged.",
    );
  });
});
