import { describe, expect, it } from "vitest";
import {
  commentsMessage,
  isStale,
  patchLines,
  quoteLines,
  type LineComment,
} from "./lineComments";

const patch = [
  "diff --git a/src/a.ts b/src/a.ts",
  "@@ -10,4 +10,5 @@",
  " keep()",
  "-retry(all)",
  "+const key = id()",
  "+retry(transient)",
  " done()",
].join("\n");
const comment: LineComment = {
  id: "c1",
  path: "src/a.ts",
  side: "new",
  start: 11,
  end: 12,
  quote: ["const key = id()", "retry(transient)"],
  note: "Only retry timeouts.",
};

describe("line comments", () => {
  it("numbers patch lines on their side", () => {
    const lines = patchLines(patch);
    expect(lines.map((l) => [l.kind, l.old ?? null, l.next ?? null])).toEqual([
      ["meta", null, null],
      ["meta", null, null],
      ["context", 10, 10],
      ["removed", 11, null],
      ["added", null, 11],
      ["added", null, 12],
      ["context", 12, 13],
    ]);
    expect(quoteLines(patch, "new", 11, 12)).toEqual(comment.quote);
    expect(quoteLines(patch, "old", 11, 11)).toEqual(["retry(all)"]);
    expect(quoteLines(patch, "new", 12, 20)).toBeNull();
  });

  it("is out of date when its lines read differently or the file is gone", () => {
    const files = (p: string) => ({ files: [{ path: "src/a.ts", patch: p }] });
    expect(isStale(comment, files(patch))).toBe(false);
    expect(
      isStale(comment, files(patch.replace("transient", "timeouts"))),
    ).toBe(true);
    expect(isStale(comment, { files: [] })).toBe(true);
    // A partial view may have left the file out.
    expect(isStale(comment, { files: [], partial: true })).toBe(false);
    // Unknown until the changes are read.
    expect(isStale(comment, undefined)).toBe(false);
  });

  it("drafts one message with places, quoted code and notes", () => {
    const removed: LineComment = {
      ...comment,
      id: "c2",
      side: "old",
      start: 11,
      end: 11,
      quote: ["retry(all)"],
      note: "Why was this removed?\nIt handled 429s.",
    };
    expect(commentsMessage([comment, removed], new Set(["c2"]))).toBe(
      [
        "Please address these comments on the changes:",
        "",
        "1. src/a.ts:11-12",
        "   ```",
        "   const key = id()",
        "   retry(transient)",
        "   ```",
        "   Only retry timeouts.",
        "",
        "2. src/a.ts:11 (removed lines)",
        "   ```",
        "   retry(all)",
        "   ```",
        "   Why was this removed?",
        "   It handled 429s.",
        "   (These lines have changed since this comment.)",
      ].join("\n"),
    );
  });
});
