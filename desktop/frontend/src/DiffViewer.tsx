import { Fragment, useEffect, useRef, useState } from "react";
import type { ReviewSnapshot } from "./api";
import {
  commentPlace,
  isStale,
  lineNumber,
  patchLines,
  type LineComment,
} from "./chat/lineComments";

type Selection = { side: "new" | "old"; start: number; end: number };

export default function DiffViewer({
  snapshot,
  selectedFile,
  selectedLine,
  onFile,
  hideFiles = false,
  comments,
  onComment,
  onRemoveComment,
}: {
  snapshot: Pick<ReviewSnapshot, "files">;
  selectedFile: string;
  selectedLine: number;
  onFile: (file: string) => void;
  hideFiles?: boolean;
  /** Line comments to show under their lines. */
  comments?: LineComment[];
  /** Given, line numbers can be clicked to comment (shift-click for a range). */
  onComment?: (comment: Omit<LineComment, "id">) => void;
  onRemoveComment?: (id: string) => void;
}) {
  const selected =
    snapshot.files.find((file) => file.path === selectedFile) ||
    snapshot.files[0];
  const [wrap, setWrap] = useState(false);
  const [width, setWidth] = useState(180);
  const highlight = useRef<HTMLDivElement>(null);
  const [selection, setSelection] = useState<Selection | null>(null);
  const [note, setNote] = useState("");
  useEffect(() => {
    setSelection(null);
    setNote("");
  }, [selected?.path]);
  const parsed = patchLines(selected?.patch || "");
  const shownComments = (comments || []).filter(
    (c) => c.path === selected?.path && !isStale(c, snapshot),
  );
  function pick(side: "new" | "old", n: number, extend: boolean) {
    setSelection((current) =>
      extend && current && current.side === side
        ? {
            side,
            start: Math.min(current.start, n),
            end: Math.max(current.end, n),
          }
        : { side, start: n, end: n },
    );
  }
  function addComment() {
    if (!selection || !selected || !note.trim() || !onComment) return;
    const quote = parsed
      .filter((line) => {
        const n = lineNumber(line, selection.side);
        return n !== undefined && n >= selection.start && n <= selection.end;
      })
      .map((line) => line.text.slice(1));
    onComment({
      path: selected.path,
      side: selection.side,
      start: selection.start,
      end: selection.end,
      quote,
      note: note.trim(),
    });
    setSelection(null);
    setNote("");
  }
  useEffect(() => {
    const row = highlight.current,
      container = row?.parentElement;
    if (selectedLine && row && container)
      container.scrollTop +=
        row.getBoundingClientRect().top -
        container.getBoundingClientRect().top -
        container.clientHeight / 2;
  }, [selected?.path, selectedLine]);
  const lines = selected?.patch.split("\n") || [];
  const added = lines.filter(
    (line) => line.startsWith("+") && !line.startsWith("+++"),
  ).length;
  const removed = lines.filter(
    (line) => line.startsWith("-") && !line.startsWith("---"),
  ).length;
  return (
    <section className="diff-viewer" aria-label="Diff preview">
      <div className="diff-toolbar">
        <strong title={selected?.path}>
          {selected?.path || "No changed files"}
        </strong>
        <span className="diff-count added">+{added}</span>
        <span className="diff-count removed">−{removed}</span>
        <label className="diff-width" hidden={hideFiles}>
          Files{" "}
          <input
            type="range"
            min="140"
            max="300"
            step="10"
            value={width}
            aria-label="File panel width"
            onChange={(e) => setWidth(Number(e.target.value))}
          />
        </label>
        <label>
          <input
            type="checkbox"
            checked={wrap}
            onChange={(e) => setWrap(e.target.checked)}
          />
          Wrap lines
        </label>
      </div>
      <div
        className={`review-diff ${wrap ? "wrap-lines" : ""}`}
        style={{
          gridTemplateColumns: hideFiles
            ? "minmax(0, 1fr)"
            : `${width}px minmax(0, 1fr)`,
        }}
      >
        <aside
          hidden={hideFiles}
          aria-label="Changed files"
          onKeyDown={(e) => {
            if (!(e.target instanceof HTMLButtonElement)) return;
            let index = snapshot.files.findIndex(
              (file) => file.path === selected?.path,
            );
            if (e.key === "ArrowDown")
              index = Math.min(index + 1, snapshot.files.length - 1);
            else if (e.key === "ArrowUp") index = Math.max(0, index - 1);
            else if (e.key === "Home") index = 0;
            else if (e.key === "End") index = snapshot.files.length - 1;
            else return;
            e.preventDefault();
            if (snapshot.files[index]) {
              onFile(snapshot.files[index].path);
              e.currentTarget.querySelectorAll("button")[index]?.focus();
            }
          }}
        >
          {snapshot.files.map((file) => (
            <button
              key={file.path}
              className={file.path === selected?.path ? "active" : ""}
              tabIndex={file.path === selected?.path ? 0 : -1}
              aria-current={file.path === selected?.path ? "true" : undefined}
              onClick={() => onFile(file.path)}
            >
              {file.path}
            </button>
          ))}
        </aside>
        {/* A scrollable region must be focusable to scroll it from the keyboard. */}
        {/* eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex */}
        <div className="diff-code" aria-label="Reviewed diff" tabIndex={0}>
          {parsed.map((line, index) => {
            const { kind, text, old, next } = line;
            const side = kind === "removed" ? "old" : "new";
            const n = lineNumber(line, side);
            const focused = !!selectedLine && selectedLine === next;
            const chosen =
              !!selection &&
              n !== undefined &&
              selection.side === side &&
              n >= selection.start &&
              n <= selection.end;
            const commented = shownComments.some(
              (c) =>
                c.side === side &&
                n !== undefined &&
                n >= c.start &&
                n <= c.end,
            );
            const number = (value: number | undefined, own: boolean) =>
              onComment && own && value !== undefined ? (
                <button
                  className="diff-number"
                  aria-label={`Comment on ${side === "old" ? "removed " : ""}line ${value}`}
                  title="Comment on this line; shift-click to include more lines"
                  onClick={(e) => pick(side, value, e.shiftKey)}
                >
                  {value}
                </button>
              ) : (
                <span>{value ?? ""}</span>
              );
            const endsHere =
              n !== undefined &&
              selection?.side === side &&
              selection.end === n;
            return (
              <Fragment key={index}>
                <div
                  ref={focused ? highlight : undefined}
                  className={`diff-line ${kind} ${focused ? "focused" : ""} ${chosen ? "selected" : ""} ${commented ? "commented" : ""}`}
                >
                  {number(old, side === "old")}
                  {number(next, side === "new")}
                  <code>{text || " "}</code>
                </div>
                {shownComments
                  .filter((c) => c.side === side && c.end === n)
                  .map((c) => (
                    <div className="diff-comment" key={c.id}>
                      <p>{c.note}</p>
                      <small>{commentPlace(c)}</small>
                      {onRemoveComment && (
                        <button
                          className="text-button"
                          onClick={() => onRemoveComment(c.id)}
                        >
                          Remove
                        </button>
                      )}
                    </div>
                  ))}
                {endsHere && selection && (
                  <div className="diff-comment-form">
                    <textarea
                      aria-label={`Comment on ${commentPlace({ path: selected!.path, side: selection.side, start: selection.start, end: selection.end, quote: [], note: "", id: "" })}`}
                      placeholder="What should change here?"
                      value={note}
                      // The form opens where the user just clicked.
                      // eslint-disable-next-line jsx-a11y/no-autofocus
                      autoFocus
                      onChange={(e) => setNote(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Escape") {
                          e.preventDefault();
                          e.stopPropagation();
                          setSelection(null);
                          setNote("");
                        } else if (
                          e.key === "Enter" &&
                          (e.metaKey || e.ctrlKey)
                        ) {
                          e.preventDefault();
                          addComment();
                        }
                      }}
                    />
                    <div>
                      <button
                        className="button secondary"
                        onClick={() => {
                          setSelection(null);
                          setNote("");
                        }}
                      >
                        Cancel
                      </button>
                      <button
                        className="button primary"
                        disabled={!note.trim()}
                        onClick={addComment}
                      >
                        Add comment
                      </button>
                    </div>
                  </div>
                )}
              </Fragment>
            );
          })}
        </div>
      </div>
    </section>
  );
}
