import { useEffect, useRef, useState, type RefObject } from "react";
import Icon from "../Icon";

type HighlightRegistry = {
  set(name: string, highlight: unknown): void;
  delete(name: string): void;
};
// The CSS Custom Highlight API marks matches without changing the page. It is
// missing in older WebKit; search and stepping still work without it.
const registry = () =>
  (globalThis.CSS as unknown as { highlights?: HighlightRegistry } | undefined)
    ?.highlights;
const makeHighlight = (ranges: Range[]) => {
  const Ctor = (
    globalThis as unknown as { Highlight?: new (...r: Range[]) => unknown }
  ).Highlight;
  return Ctor ? new Ctor(...ranges) : undefined;
};

/** Ranges of every visible occurrence of query under root, in reading order. */
export function findRanges(root: HTMLElement, query: string) {
  const needle = query.toLowerCase();
  const ranges: Range[] = [];
  if (!needle) return ranges;
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  let node: Node | null;
  while ((node = walker.nextNode())) {
    const parent = node.parentElement;
    // Skip output of collapsed tool steps (their summary row stays searchable).
    const collapsed = parent?.closest("details:not([open])");
    if (!parent || (collapsed && !parent.closest("summary"))) continue;
    const text = (node.textContent || "").toLowerCase();
    for (
      let at = text.indexOf(needle);
      at >= 0;
      at = text.indexOf(needle, at + needle.length)
    ) {
      const range = document.createRange();
      range.setStart(node, at);
      range.setEnd(node, at + needle.length);
      ranges.push(range);
    }
  }
  return ranges;
}

/** Find text in the conversation. Enter steps forward, Shift+Enter back. */
export default function FindBar({
  container,
  version,
  onClose,
}: {
  container: RefObject<HTMLElement | null>;
  /** Changes when the conversation does, so matches are found again. */
  version: number;
  onClose: () => void;
}) {
  const [query, setQuery] = useState("");
  const [count, setCount] = useState(0);
  const [current, setCurrent] = useState(0);
  const ranges = useRef<Range[]>([]);
  const input = useRef<HTMLInputElement>(null);

  useEffect(() => input.current?.focus(), []);
  useEffect(() => {
    ranges.current = container.current
      ? findRanges(container.current, query.trim())
      : [];
    setCount(ranges.current.length);
    setCurrent((index) =>
      Math.min(index, Math.max(0, ranges.current.length - 1)),
    );
    const all = makeHighlight(ranges.current);
    if (all) registry()?.set("find-match", all);
    return () => registry()?.delete("find-match");
  }, [container, query, version]);
  useEffect(() => {
    const range = ranges.current[current];
    if (!range) return;
    const here = makeHighlight([range]);
    if (here) registry()?.set("find-current", here);
    range.startContainer.parentElement?.scrollIntoView?.({
      block: "center",
      behavior: "auto",
    });
    return () => registry()?.delete("find-current");
  }, [current, count, query, version]);

  const step = (by: number) =>
    count && setCurrent((index) => (index + by + count) % count);
  return (
    <div className="find-bar" role="search">
      <Icon name="search" size={14} />
      <input
        ref={input}
        aria-label="Find in conversation"
        placeholder="Find in conversation"
        value={query}
        onChange={(e) => {
          setQuery(e.target.value);
          setCurrent(0);
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            step(e.shiftKey ? -1 : 1);
          } else if (e.key === "Escape") {
            e.preventDefault();
            onClose();
          }
        }}
      />
      <span role="status">
        {query.trim()
          ? count
            ? `${current + 1} of ${count}`
            : "No matches"
          : ""}
      </span>
      <button
        className="icon-button"
        aria-label="Previous match"
        disabled={!count}
        onClick={() => step(-1)}
      >
        <Icon name="arrow-up" size={14} />
      </button>
      <button
        className="icon-button next"
        aria-label="Next match"
        disabled={!count}
        onClick={() => step(1)}
      >
        <Icon name="arrow-up" size={14} />
      </button>
      <button className="icon-button" aria-label="Close find" onClick={onClose}>
        <Icon name="close" size={14} />
      </button>
    </div>
  );
}
