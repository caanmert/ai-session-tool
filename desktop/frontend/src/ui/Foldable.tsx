import {
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";

/** Sent to a folded element to open it, for example by find. */
export const REVEAL_EVENT = "ais:reveal";

/**
 * Opens everything folded around a node, up to (not including) stop: closed
 * details and anything marked as folded. Used to show a match from find.
 */
export function revealAround(node: Element | null, stop: Element | null) {
  for (let el = node; el && el !== stop; el = el.parentElement) {
    if (
      el instanceof HTMLDetailsElement &&
      !el.open &&
      !node?.closest("summary")
    )
      el.open = true;
    if (el.hasAttribute("data-folded"))
      el.dispatchEvent(new Event(REVEAL_EVENT));
  }
}

/** Calls reveal when find asks a folded element to open. */
export function useReveal(
  ref: React.RefObject<HTMLElement | null>,
  active: boolean,
  reveal: () => void,
) {
  useEffect(() => {
    const el = ref.current;
    if (!el || !active) return;
    el.addEventListener(REVEAL_EVENT, reveal);
    return () => el.removeEventListener(REVEAL_EVENT, reveal);
  }, [ref, active, reveal]);
}

/**
 * Long content shown as its first lines with a fade, and a button to show it
 * all. The full text stays in the page, so find can still reach it.
 */
export default function Foldable({
  lines,
  limit,
  keep,
  more = "Show more",
  less = "Show less",
  children,
}: {
  /** How many lines the content has. */
  lines: number;
  /** Fold when there are more lines than this. */
  limit: number;
  /** How many lines stay visible while folded. */
  keep: number;
  more?: string;
  less?: string;
  children: ReactNode;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const long = lines > limit;
  useReveal(ref, long && !open, () => setOpen(true));
  if (!long) return children;
  return (
    <div
      ref={ref}
      className={`foldable ${open ? "" : "folded"}`}
      data-folded={open ? undefined : ""}
      style={{ "--fold-lines": keep } as CSSProperties}
    >
      <div className="foldable-body">{children}</div>
      <button
        className="fold-toggle"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
      >
        {open ? less : more}
      </button>
    </div>
  );
}
