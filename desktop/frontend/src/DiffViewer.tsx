import { useEffect, useRef, useState } from "react";
import type { ReviewSnapshot } from "./api";

export default function DiffViewer({
  snapshot,
  selectedFile,
  selectedLine,
  onFile,
  hideFiles = false,
}: {
  snapshot: Pick<ReviewSnapshot, "files">;
  selectedFile: string;
  selectedLine: number;
  onFile: (file: string) => void;
  hideFiles?: boolean;
}) {
  const selected =
    snapshot.files.find((file) => file.path === selectedFile) ||
    snapshot.files[0];
  const [wrap, setWrap] = useState(false);
  const [width, setWidth] = useState(180);
  const highlight = useRef<HTMLDivElement>(null);
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
  let oldLine = 0,
    newLine = 0;
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
          {lines.map((text, index) => {
            const hunk = text.match(/^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/);
            let old: number | undefined,
              next: number | undefined,
              kind = "meta";
            if (hunk) {
              oldLine = Number(hunk[1]);
              newLine = Number(hunk[2]);
            } else if (text.startsWith("+") && !text.startsWith("+++")) {
              kind = "added";
              next = newLine++;
            } else if (text.startsWith("-") && !text.startsWith("---")) {
              kind = "removed";
              old = oldLine++;
            } else if (text.startsWith(" ")) {
              kind = "context";
              old = oldLine++;
              next = newLine++;
            }
            const focused = !!selectedLine && selectedLine === next;
            return (
              <div
                key={index}
                ref={focused ? highlight : undefined}
                className={`diff-line ${kind} ${focused ? "focused" : ""}`}
              >
                <span>{old ?? ""}</span>
                <span>{next ?? ""}</span>
                <code>{text || " "}</code>
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
}
