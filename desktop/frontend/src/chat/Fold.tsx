import { useState, type ReactNode } from "react";
import Icon from "../Icon";

const KEY = "ais:details-fold:v1";

function stored(): Record<string, boolean> {
  try {
    const value = JSON.parse(window.localStorage.getItem(KEY) || "{}");
    return value && typeof value === "object" ? value : {};
  } catch {
    return {};
  }
}

/**
 * Whether a section of the details panel is open, remembered on this Mac for
 * every conversation. Storage is a convenience: without it, sections start
 * as they would the first time.
 */
export function useFold(name: string, initial: boolean) {
  const [open, setOpen] = useState(() => stored()[name] ?? initial);
  function toggle() {
    setOpen((value) => {
      try {
        window.localStorage.setItem(
          KEY,
          JSON.stringify({ ...stored(), [name]: !value }),
        );
      } catch {
        // Not remembered; the section still opens and closes.
      }
      return !value;
    });
  }
  return [open, toggle] as const;
}

/** A section heading that opens and closes the section below it. */
export function FoldHeading({
  title,
  open,
  onToggle,
  children,
}: {
  title: string;
  open: boolean;
  onToggle: () => void;
  /** Controls beside the title, such as a refresh button. */
  children?: ReactNode;
}) {
  return (
    <h3 className="details-section fold-heading">
      <button
        type="button"
        className="fold-toggle"
        aria-expanded={open}
        // Named apart from the section's own buttons, such as Run.
        aria-label={`${title} section`}
        onClick={onToggle}
      >
        <Icon name="chevron" size={12} />
        {title}
      </button>
      {children}
    </h3>
  );
}
