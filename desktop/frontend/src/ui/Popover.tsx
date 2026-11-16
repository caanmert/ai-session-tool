import { useEffect, useRef, useState, type ReactNode } from "react";

/**
 * A button that opens a small panel above it: a menu or a few settings.
 * Escape and a click outside close it, and focus goes back to the button.
 * In a menu, the arrow keys move between items.
 */
export default function Popover({
  label,
  title,
  className = "",
  buttonClassName = "pill-button",
  panelLabel,
  role = "dialog",
  disabled = false,
  button,
  children,
}: {
  /** The button's accessible name. */
  label: string;
  title?: string;
  className?: string;
  buttonClassName?: string;
  panelLabel: string;
  role?: "dialog" | "menu";
  disabled?: boolean;
  button: ReactNode;
  children: (close: () => void) => ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const close = () => {
    setOpen(false);
    trigger.current?.focus();
  };
  useEffect(() => {
    if (!open) return;
    const el = panel.current;
    (
      el?.querySelector<HTMLElement>('[aria-checked="true"]') ||
      el?.querySelector<HTMLElement>(
        "button:not(:disabled), select:not(:disabled), input:not(:disabled)",
      )
    )?.focus();
    const away = (e: MouseEvent) => {
      const target = e.target as Node;
      if (!el?.contains(target) && !trigger.current?.contains(target))
        setOpen(false);
    };
    document.addEventListener("mousedown", away);
    return () => document.removeEventListener("mousedown", away);
  }, [open]);
  return (
    <span className={`popover ${className}`}>
      <button
        ref={trigger}
        type="button"
        className={buttonClassName}
        aria-label={label}
        aria-haspopup={role}
        aria-expanded={open}
        title={title}
        disabled={disabled}
        onClick={() => setOpen((value) => !value)}
      >
        {button}
      </button>
      {open && (
        <div
          ref={panel}
          className="popover-panel"
          role={role}
          aria-label={panelLabel}
          tabIndex={-1}
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              e.preventDefault();
              e.stopPropagation();
              close();
              return;
            }
            if (role !== "menu") return;
            if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
            e.preventDefault();
            const items = [
              ...(panel.current?.querySelectorAll<HTMLElement>(
                '[role^="menuitem"]:not(:disabled)',
              ) || []),
            ];
            const at = items.indexOf(document.activeElement as HTMLElement);
            const step = e.key === "ArrowDown" ? 1 : -1;
            items[(at + step + items.length) % items.length]?.focus();
          }}
        >
          {children(close)}
        </div>
      )}
    </span>
  );
}
