import { useEffect, useRef, useState } from "react";
import Icon from "../Icon";
import { api, errorText } from "../api";

export const webSearchChoices = [
  { id: "", label: "Codex's own setting", detail: "As set in Codex" },
  { id: "disabled", label: "Off", detail: "No web search" },
  { id: "cached", label: "Cached", detail: "Searches an index of pages" },
  { id: "live", label: "Live", detail: "Fetches pages as they are now" },
];

/**
 * Codex's web search for this conversation, from the next message: a globe
 * button with a short menu. It is marked when search is on.
 */
export default function WebSearchPicker({
  chatId,
  mode,
  disabled,
  onSaved,
}: {
  chatId: string;
  mode: string;
  disabled: boolean;
  onSaved: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const button = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const current =
    webSearchChoices.find((choice) => choice.id === mode) ||
    webSearchChoices[0];
  const on = mode === "cached" || mode === "live";
  useEffect(() => {
    if (!open) return;
    menu.current
      ?.querySelector<HTMLButtonElement>('[aria-checked="true"]')
      ?.focus();
    const away = (e: MouseEvent) => {
      if (
        !menu.current?.contains(e.target as Node) &&
        e.target !== button.current
      )
        setOpen(false);
    };
    document.addEventListener("mousedown", away);
    return () => document.removeEventListener("mousedown", away);
  }, [open]);
  function close() {
    setOpen(false);
    button.current?.focus();
  }
  async function choose(next: string) {
    close();
    if (next === mode) return;
    setSaving(true);
    setError("");
    try {
      await api.SetChatWebSearch(chatId, next);
      onSaved();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setSaving(false);
    }
  }
  return (
    <span className="web-search-picker">
      <button
        ref={button}
        type="button"
        className={`icon-button ${on ? "on" : ""}`}
        aria-label={`Web search: ${current.label}`}
        aria-haspopup="menu"
        aria-expanded={open}
        title={`Web search: ${current.label}. ${current.detail}.`}
        disabled={disabled || saving}
        onClick={() => setOpen((value) => !value)}
      >
        <Icon name="globe" size={16} />
      </button>
      {open && (
        <div
          ref={menu}
          className="web-search-menu"
          role="menu"
          aria-label="Web search"
          tabIndex={-1}
          onKeyDown={(e) => {
            const items = [
              ...(menu.current?.querySelectorAll<HTMLButtonElement>(
                '[role="menuitemradio"]',
              ) || []),
            ];
            const at = items.indexOf(
              document.activeElement as HTMLButtonElement,
            );
            if (e.key === "Escape") {
              e.preventDefault();
              close();
            } else if (e.key === "ArrowDown" || e.key === "ArrowUp") {
              e.preventDefault();
              const step = e.key === "ArrowDown" ? 1 : -1;
              items[(at + step + items.length) % items.length]?.focus();
            }
          }}
        >
          <p>Web search · from the next message</p>
          {webSearchChoices.map((choice) => (
            <button
              key={choice.id}
              type="button"
              role="menuitemradio"
              aria-checked={choice.id === mode}
              onClick={() => void choose(choice.id)}
            >
              <span className="radio-dot" aria-hidden="true" />
              <strong>{choice.label}</strong>
              <small>{choice.detail}</small>
            </button>
          ))}
        </div>
      )}
      {error && (
        <span className="mode-error" role="alert">
          {error}
        </span>
      )}
    </span>
  );
}
