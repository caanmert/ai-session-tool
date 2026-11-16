import { expect, it } from "vitest";
import { fillPrompt, matchPrompts } from "./savedPrompts";

it("fills placeholders, with plain words when something is unknown", () => {
  expect(
    fillPrompt("{project} {branch}: {changed files} ({project})", {
      project: "/w/app",
      branch: "main",
      files: ["a.ts", "b.ts"],
    }),
  ).toBe("/w/app main: a.ts, b.ts (/w/app)");
  expect(fillPrompt("{branch} {changed files}", { project: "" })).toBe(
    "the current branch (no uncommitted changes)",
  );
});

it("matches names before text", () => {
  const prompts = [
    { id: "1", name: "Explain", text: "Explain the tests" },
    { id: "2", name: "Tests", text: "Write them" },
  ];
  expect(matchPrompts(prompts, "test").map((p) => p.id)).toEqual(["2", "1"]);
  expect(matchPrompts(prompts, "")).toHaveLength(2);
});
