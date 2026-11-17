import { useCallback, useState } from "react";
import type { ReviewSnapshot } from "../api";

/** A note on lines of the uncommitted changes, kept until it is sent. */
export type LineComment = {
  id: string;
  path: string;
  /** "new" numbers lines in the changed file; "old" in the removed lines. */
  side: "new" | "old";
  start: number;
  end: number;
  /** The commented lines as they were, without the diff's +/-/space mark. */
  quote: string[];
  note: string;
};

/** A line of a patch, numbered on the side it belongs to. */
export type PatchLine = {
  kind: "added" | "removed" | "context" | "meta";
  text: string;
  old?: number;
  next?: number;
};

export function patchLines(patch: string): PatchLine[] {
  let oldLine = 0,
    newLine = 0;
  return patch.split("\n").map((text): PatchLine => {
    const hunk = text.match(/^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/);
    if (hunk) {
      oldLine = Number(hunk[1]);
      newLine = Number(hunk[2]);
      return { kind: "meta", text };
    }
    if (text.startsWith("+") && !text.startsWith("+++"))
      return { kind: "added", text, next: newLine++ };
    if (text.startsWith("-") && !text.startsWith("---"))
      return { kind: "removed", text, old: oldLine++ };
    if (text.startsWith(" "))
      return { kind: "context", text, old: oldLine++, next: newLine++ };
    return { kind: "meta", text };
  });
}

/** The line's number on a side, if it has one there. */
export const lineNumber = (line: PatchLine, side: "new" | "old") =>
  side === "new"
    ? line.kind === "removed"
      ? undefined
      : line.next
    : line.kind === "removed"
      ? line.old
      : undefined;

/** The text of lines start..end on a side of a patch, or null if any is missing. */
export function quoteLines(
  patch: string,
  side: "new" | "old",
  start: number,
  end: number,
): string[] | null {
  const byNumber = new Map<number, string>();
  for (const line of patchLines(patch)) {
    const n = lineNumber(line, side);
    if (n !== undefined) byNumber.set(n, line.text.slice(1));
  }
  const quote: string[] = [];
  for (let n = start; n <= end; n++) {
    const text = byNumber.get(n);
    if (text === undefined) return null;
    quote.push(text);
  }
  return quote;
}

/** True when a comment's lines no longer read the same in the changes. */
export function isStale(
  comment: LineComment,
  snapshot: Pick<ReviewSnapshot, "files" | "partial"> | undefined,
) {
  if (!snapshot) return false;
  const file = snapshot.files.find((f) => f.path === comment.path);
  // A partial view may have left the file out; it is not known to be stale.
  if (!file) return !snapshot.partial;
  const now = quoteLines(file.patch, comment.side, comment.start, comment.end);
  return !now || now.join("\n") !== comment.quote.join("\n");
}

export const commentPlace = (c: LineComment) =>
  `${c.path}:${c.start === c.end ? c.start : `${c.start}-${c.end}`}${c.side === "old" ? " (removed lines)" : ""}`;

/** The message that asks the agent to address the comments. */
export function commentsMessage(
  comments: LineComment[],
  stale: Set<string> = new Set(),
) {
  const parts = comments.map((c, i) => {
    const fence = c.quote.some((line) => line.includes("```")) ? "````" : "```";
    const quoted = [fence, ...c.quote, fence].map((l) => `   ${l}`).join("\n");
    const outdated = stale.has(c.id)
      ? "\n   (These lines have changed since this comment.)"
      : "";
    const note = c.note
      .trim()
      .split("\n")
      .map((l) => `   ${l}`)
      .join("\n");
    return `${i + 1}. ${commentPlace(c)}\n${quoted}\n${note}${outdated}`;
  });
  return `Please address these comments on the changes:\n\n${parts.join("\n\n")}`;
}

const storageKey = (chatId: string) => `ais:line-comments:v1:${chatId}`;

function load(chatId: string): LineComment[] {
  try {
    const saved = JSON.parse(
      window.localStorage.getItem(storageKey(chatId)) || "[]",
    ) as unknown;
    return Array.isArray(saved) ? (saved as LineComment[]) : [];
  } catch {
    return [];
  }
}

/**
 * A conversation's line comments, kept on this Mac until they are sent, like
 * a draft. When storage is unavailable they last until the app quits.
 */
export function useLineComments(chatId: string) {
  const [comments, setComments] = useState<LineComment[]>(() => load(chatId));
  const change = useCallback(
    (next: (current: LineComment[]) => LineComment[]) =>
      setComments((current) => {
        const value = next(current);
        try {
          if (value.length)
            window.localStorage.setItem(
              storageKey(chatId),
              JSON.stringify(value),
            );
          else window.localStorage.removeItem(storageKey(chatId));
        } catch {
          // Kept for this session only.
        }
        return value;
      }),
    [chatId],
  );
  return {
    comments,
    add: (comment: Omit<LineComment, "id">) =>
      change((current) => [
        ...current,
        { ...comment, id: `${Date.now()}-${current.length}` },
      ]),
    update: (id: string, note: string) =>
      change((current) =>
        current.map((c) => (c.id === id ? { ...c, note } : c)),
      ),
    remove: (id: string) =>
      change((current) => current.filter((c) => c.id !== id)),
    clear: () => change(() => []),
  };
}
