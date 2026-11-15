import {
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type RefObject,
} from "react";
import Icon from "../Icon";
import { api } from "../api";

/**
 * The file mention being typed at the caret, if any: an "@" at the start of
 * the text or after whitespace, followed by characters without spaces.
 */
export function mentionAt(text: string, caret: number) {
  const before = text.slice(0, caret);
  const start = before.lastIndexOf("@");
  if (start < 0) return null;
  if (start > 0 && !/\s/.test(before[start - 1])) return null;
  const query = before.slice(start + 1);
  if (/\s/.test(query)) return null;
  return { start, query };
}

/** Replace the mention being typed with a path, quoted when it has spaces. */
export function insertMention(
  text: string,
  caret: number,
  start: number,
  path: string,
) {
  const mention = /\s/.test(path) ? '"' + path + '"' : path;
  const head = text.slice(0, start) + mention + " ";
  return {
    text: head + text.slice(caret).replace(/^ /, ""),
    caret: head.length,
  };
}

/**
 * Type "@" in the message box to find a file in the project and insert its
 * path. Only file names are listed; the agent opens the file itself.
 */
export function useFileMentions({
  chatId,
  prompt,
  updatePrompt,
  input,
}: {
  chatId: string;
  prompt: string;
  updatePrompt: (text: string) => void;
  input: RefObject<HTMLTextAreaElement | null>;
}) {
  const [caret, setCaret] = useState(0);
  const [files, setFiles] = useState<string[]>([]);
  const [index, setIndex] = useState(0);
  const [dismissed, setDismissed] = useState(-1);
  const pendingCaret = useRef<number | null>(null);
  const mention = mentionAt(prompt, caret);
  const open = !!mention && mention.start !== dismissed;
  const query = open ? mention.query : null;

  useEffect(() => {
    if (query === null) return;
    let active = true;
    // Wait for a pause in typing before asking for matches.
    const timer = setTimeout(() => {
      api
        .ProjectFiles(chatId, query)
        .then((found) => {
          if (!active) return;
          setFiles(found);
          setIndex(0);
        })
        .catch(() => {
          if (active) setFiles([]);
        });
    }, 120);
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [chatId, query]);

  // Put the caret after an inserted path once the new text is in the box.
  useEffect(() => {
    if (pendingCaret.current === null || !input.current) return;
    input.current.setSelectionRange(pendingCaret.current, pendingCaret.current);
    setCaret(pendingCaret.current);
    pendingCaret.current = null;
  }, [prompt, input]);

  const shown = open ? files : [];
  const active = Math.min(index, Math.max(0, shown.length - 1));

  function choose(path: string) {
    if (!mention) return;
    const next = insertMention(prompt, caret, mention.start, path);
    pendingCaret.current = next.caret;
    updatePrompt(next.text);
    setFiles([]);
    input.current?.focus();
  }

  /** Returns true when the key was used by the file list. */
  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (!open) return false;
    if (e.key === "Escape") {
      e.preventDefault();
      setDismissed(mention.start);
      return true;
    }
    if (!shown.length) return false;
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      const step = e.key === "ArrowDown" ? 1 : -1;
      setIndex((active + step + shown.length) % shown.length);
      return true;
    }
    if ((e.key === "Enter" || e.key === "Tab") && !e.nativeEvent.isComposing) {
      e.preventDefault();
      choose(shown[active]);
      return true;
    }
    return false;
  }

  const listId = `file-mentions-${chatId}`;
  const popup = open ? (
    <div className="file-mentions">
      {shown.length ? (
        <ul id={listId} role="listbox" aria-label="Files in this project">
          {shown.map((path, i) => {
            const slash = path.lastIndexOf("/") + 1;
            return (
              // Chosen with the pointer here, or with Enter in the message
              // box, which keeps keyboard focus.
              // eslint-disable-next-line jsx-a11y/click-events-have-key-events
              <li
                key={path}
                id={`${listId}-${i}`}
                role="option"
                aria-selected={i === active}
                onMouseMove={() => setIndex(i)}
                // Keep focus in the message box while choosing.
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => choose(path)}
              >
                <Icon name="file" size={13} />
                <span>{path.slice(slash)}</span>
                <small>{path.slice(0, slash)}</small>
              </li>
            );
          })}
        </ul>
      ) : (
        <p role="status">
          {mention.query
            ? `No file matches “${mention.query}”.`
            : "Type to find a file in this project."}
        </p>
      )}
    </div>
  ) : null;

  return {
    popup,
    onKeyDown,
    trackCaret: () => {
      const position = input.current?.selectionStart ?? 0;
      setCaret(position);
      // Moving off a dismissed mention lets the next "@" open the list again.
      if (dismissed >= 0 && mentionAt(prompt, position)?.start !== dismissed)
        setDismissed(-1);
    },
    activeOption: open && shown.length ? `${listId}-${active}` : undefined,
    listId: open ? listId : undefined,
  };
}
