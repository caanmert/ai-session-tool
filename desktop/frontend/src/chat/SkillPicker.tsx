import {
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type RefObject,
} from "react";
import Icon from "../Icon";
import { api, errorText, type AgentSkill, type SavedPrompt } from "../api";
import { matchPrompts } from "./savedPrompts";

const LIMIT = 60;

/**
 * The skill being typed at the caret, if any: "/" (or "$" for Codex) at the
 * start of the text or after whitespace, then characters without spaces.
 */
export function skillAt(text: string, caret: number, tool: string) {
  const before = text.slice(0, caret);
  const marks = tool === "codex" ? ["/", "$"] : ["/"];
  const start = Math.max(...marks.map((mark) => before.lastIndexOf(mark)));
  if (start < 0) return null;
  if (start > 0 && !/\s/.test(before[start - 1])) return null;
  const query = before.slice(start + 1);
  if (/\s/.test(query)) return null;
  return { start, query };
}

/** How a skill is named in a message: "/name" for Claude Code, "$name" for Codex. */
export const skillToken = (tool: string, name: string) =>
  (tool === "codex" ? "$" : "/") + name;

/** Skills matching what was typed: names first, then descriptions. */
export function matchSkills(skills: AgentSkill[], query: string) {
  const q = query.toLowerCase();
  if (!q) return skills.slice(0, LIMIT);
  const byName = skills.filter((s) => s.name.toLowerCase().includes(q));
  const byText = skills.filter(
    (s) =>
      !s.name.toLowerCase().includes(q) &&
      s.description.toLowerCase().includes(q),
  );
  return [...byName, ...byText].slice(0, LIMIT);
}

type Item =
  | { kind: "prompt"; prompt: SavedPrompt }
  | { kind: "skill"; skill: AgentSkill };

/**
 * Type "/" in the message box to find a saved prompt or one of the agent's
 * skills. A prompt replaces what was typed with its text; a skill is added as
 * "/name". Nothing is sent until you send; the list starts no turn.
 */
export function useSkillPicker({
  chatId,
  tool,
  project,
  prompt,
  updatePrompt,
  input,
  fill = (text) => text,
}: {
  chatId: string;
  tool: string;
  project: string;
  prompt: string;
  updatePrompt: (text: string) => void;
  input: RefObject<HTMLTextAreaElement | null>;
  /** Fills a saved prompt's placeholders when it is chosen. */
  fill?: (text: string) => string;
}) {
  const [caret, setCaret] = useState(0);
  const [skills, setSkills] = useState<AgentSkill[]>();
  const [saved, setSaved] = useState<SavedPrompt[]>();
  const [problem, setProblem] = useState("");
  const [index, setIndex] = useState(0);
  const [dismissed, setDismissed] = useState(-1);
  const pendingCaret = useRef<number | null>(null);
  const typed = skillAt(prompt, caret, tool);
  const open = !!typed && typed.start !== dismissed;

  // Read the agent's skills the first time the list opens.
  const wanted = open && skills === undefined;
  useEffect(() => {
    if (!wanted) return;
    let active = true;
    api
      .AgentSkills(tool, project, false)
      .then((list) => {
        if (!active) return;
        setSkills(list.skills);
        setProblem(list.reason || "");
      })
      .catch((e) => {
        if (!active) return;
        setSkills([]);
        setProblem(errorText(e));
      });
    return () => {
      active = false;
    };
  }, [wanted, tool, project]);
  // Saved prompts are read each time the list opens, so edits in Settings show.
  useEffect(() => {
    if (!open) return;
    let active = true;
    api
      .SavedPrompts()
      .then((list) => active && setSaved(list || []))
      .catch(() => active && setSaved([]));
    return () => {
      active = false;
    };
    // Once per opening of the list.
  }, [open]);

  useEffect(() => {
    if (pendingCaret.current === null || !input.current) return;
    input.current.setSelectionRange(pendingCaret.current, pendingCaret.current);
    setCaret(pendingCaret.current);
    pendingCaret.current = null;
  }, [prompt, input]);

  const shown: Item[] = open
    ? [
        ...matchPrompts(saved || [], typed.query).map((p): Item => ({
          kind: "prompt",
          prompt: p,
        })),
        ...(skills ? matchSkills(skills, typed.query) : []).map((s): Item => ({
          kind: "skill",
          skill: s,
        })),
      ]
    : [];
  const active = Math.min(index, Math.max(0, shown.length - 1));

  function choose(item: Item) {
    if (!typed) return;
    const head =
      prompt.slice(0, typed.start) +
      (item.kind === "prompt"
        ? fill(item.prompt.text)
        : skillToken(tool, item.skill.name) + " ");
    pendingCaret.current = head.length;
    updatePrompt(head + prompt.slice(caret).replace(/^ /, ""));
    setIndex(0);
    input.current?.focus();
  }

  /** Returns true when the key was used by the skill list. */
  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (!open) return false;
    if (e.key === "Escape") {
      e.preventDefault();
      setDismissed(typed.start);
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

  const listId = `skills-${chatId}`;
  const popup = open ? (
    <div className="file-mentions skill-picker">
      {shown.length > 0 && (
        <ul id={listId} role="listbox" aria-label="Saved prompts and skills">
          {shown.map((item, i) => (
            // Chosen with the pointer here, or with Enter in the message box,
            // which keeps keyboard focus.
            // eslint-disable-next-line jsx-a11y/click-events-have-key-events
            <li
              key={
                item.kind === "prompt"
                  ? `p-${item.prompt.id}`
                  : `s-${item.skill.name}`
              }
              id={`${listId}-${i}`}
              role="option"
              aria-selected={i === active}
              title={
                item.kind === "prompt"
                  ? item.prompt.text
                  : item.skill.description
              }
              onMouseMove={() => setIndex(i)}
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => choose(item)}
            >
              {item.kind === "prompt" ? (
                <>
                  <Icon name="message" size={13} />
                  <span className="prompt-name">{item.prompt.name}</span>
                  <small>Saved prompt · {item.prompt.text}</small>
                </>
              ) : (
                <>
                  <Icon name="code" size={13} />
                  <span>
                    {skillToken(tool, item.skill.name)}
                    {item.skill.hint && <em> {item.skill.hint}</em>}
                  </span>
                  <small>{item.skill.description}</small>
                </>
              )}
            </li>
          ))}
        </ul>
      )}
      {(skills === undefined || !shown.length) && (
        <p role="status">
          {skills === undefined
            ? "Reading skills…"
            : problem ||
              (typed.query
                ? `No saved prompt or skill matches “${typed.query}”.`
                : "No saved prompts, and this agent has no skills in this project.")}
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
      if (
        dismissed >= 0 &&
        skillAt(prompt, position, tool)?.start !== dismissed
      )
        setDismissed(-1);
    },
    activeOption: open && shown.length ? `${listId}-${active}` : undefined,
    listId: open ? listId : undefined,
  };
}
