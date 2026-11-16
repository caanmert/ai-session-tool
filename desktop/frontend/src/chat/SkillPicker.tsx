import {
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type RefObject,
} from "react";
import Icon from "../Icon";
import { api, errorText, type AgentSkill } from "../api";

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

/**
 * Type "/" in the message box to find one of the agent's skills and add it
 * to the message. Nothing is sent until you send; the list starts no turn.
 */
export function useSkillPicker({
  chatId,
  tool,
  project,
  prompt,
  updatePrompt,
  input,
}: {
  chatId: string;
  tool: string;
  project: string;
  prompt: string;
  updatePrompt: (text: string) => void;
  input: RefObject<HTMLTextAreaElement | null>;
}) {
  const [caret, setCaret] = useState(0);
  const [skills, setSkills] = useState<AgentSkill[]>();
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

  useEffect(() => {
    if (pendingCaret.current === null || !input.current) return;
    input.current.setSelectionRange(pendingCaret.current, pendingCaret.current);
    setCaret(pendingCaret.current);
    pendingCaret.current = null;
  }, [prompt, input]);

  const shown = open && skills ? matchSkills(skills, typed.query) : [];
  const active = Math.min(index, Math.max(0, shown.length - 1));

  function choose(skill: AgentSkill) {
    if (!typed) return;
    const head =
      prompt.slice(0, typed.start) + skillToken(tool, skill.name) + " ";
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
      {shown.length ? (
        <ul id={listId} role="listbox" aria-label="Skills">
          {shown.map((skill, i) => (
            // Chosen with the pointer here, or with Enter in the message box,
            // which keeps keyboard focus.
            // eslint-disable-next-line jsx-a11y/click-events-have-key-events
            <li
              key={skill.name}
              id={`${listId}-${i}`}
              role="option"
              aria-selected={i === active}
              title={skill.description}
              onMouseMove={() => setIndex(i)}
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => choose(skill)}
            >
              <Icon name="code" size={13} />
              <span>
                {skillToken(tool, skill.name)}
                {skill.hint && <em> {skill.hint}</em>}
              </span>
              <small>{skill.description}</small>
            </li>
          ))}
        </ul>
      ) : (
        <p role="status">
          {skills === undefined
            ? "Reading skills…"
            : problem ||
              (typed.query
                ? `No skill matches “${typed.query}”.`
                : "This agent has no skills in this project.")}
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
