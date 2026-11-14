import { useEffect, useMemo, useRef, useState } from "react";
import Icon, { type IconName } from "../Icon";

export type Command = {
  id: string;
  group: string;
  label: string;
  /** Secondary text, also searched. */
  hint?: string;
  icon: IconName;
  shortcut?: string;
  run: () => void;
};

const LIMIT = 40;

/** Jump to a conversation, project or action from the keyboard (Cmd+K). */
export default function CommandPalette({
  commands,
  onClose,
}: {
  commands: Command[];
  onClose: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const list = useRef<HTMLUListElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState("");
  const [index, setIndex] = useState(0);
  useEffect(() => {
    const el = dialog.current;
    if (el?.showModal) el.showModal();
    input.current?.focus();
    return () => el?.close?.();
  }, []);
  const matches = useMemo(() => {
    const terms = query.toLowerCase().split(/\s+/).filter(Boolean);
    return commands
      .filter((command) => {
        const text =
          `${command.label} ${command.hint || ""} ${command.group}`.toLowerCase();
        return terms.every((term) => text.includes(term));
      })
      .slice(0, LIMIT);
  }, [commands, query]);
  const active = Math.min(index, Math.max(0, matches.length - 1));
  useEffect(() => {
    list.current
      ?.querySelector('[aria-selected="true"]')
      ?.scrollIntoView?.({ block: "nearest" });
  }, [active, matches]);
  function choose(command?: Command) {
    if (!command) return;
    onClose();
    command.run();
  }
  return (
    // A click on the backdrop closes the palette, as Escape does.
    // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
    <dialog
      ref={dialog}
      className="palette"
      aria-label="Search and commands"
      onClose={onClose}
      onCancel={onClose}
      onMouseDown={(e) => {
        // A click on the backdrop lands on the dialog element itself.
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <label className="palette-input">
        <Icon name="search" size={16} />
        <input
          ref={input}
          role="combobox"
          aria-label="Search conversations, projects and actions"
          aria-expanded="true"
          aria-controls="palette-results"
          aria-activedescendant={
            matches[active] ? `palette-${matches[active].id}` : undefined
          }
          placeholder="Search conversations, projects and actions…"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setIndex(0);
          }}
          onKeyDown={(e) => {
            if (e.key === "ArrowDown") {
              e.preventDefault();
              setIndex(active >= matches.length - 1 ? 0 : active + 1);
            } else if (e.key === "ArrowUp") {
              e.preventDefault();
              setIndex(active <= 0 ? matches.length - 1 : active - 1);
            } else if (e.key === "Enter" && !e.nativeEvent.isComposing) {
              e.preventDefault();
              choose(matches[active]);
            }
          }}
        />
        <kbd>Esc</kbd>
      </label>
      <ul id="palette-results" role="listbox" ref={list} aria-label="Results">
        {matches.map((command, i) => (
          // Options are chosen with the pointer here and with Enter in the
          // input above, which owns keyboard focus.
          // eslint-disable-next-line jsx-a11y/click-events-have-key-events
          <li
            key={command.id}
            id={`palette-${command.id}`}
            role="option"
            aria-selected={i === active}
            onMouseMove={() => setIndex(i)}
            onClick={() => choose(command)}
          >
            {(i === 0 || matches[i - 1].group !== command.group) && (
              <span className="palette-group">{command.group}</span>
            )}
            <span className="palette-row">
              <Icon name={command.icon} size={15} />
              <span className="palette-label">{command.label}</span>
              {command.hint && (
                <span className="palette-hint">{command.hint}</span>
              )}
              {command.shortcut && <kbd>{command.shortcut}</kbd>}
            </span>
          </li>
        ))}
      </ul>
      {!matches.length && (
        <p className="palette-empty" role="status">
          Nothing matches “{query}”.
        </p>
      )}
    </dialog>
  );
}
