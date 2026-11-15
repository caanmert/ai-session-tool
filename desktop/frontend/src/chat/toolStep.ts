import type { IconName } from "../Icon";
import type { ChatMessage } from "../api";

export type DiffLine = { kind: "add" | "del"; text: string };

export type ToolStepView = {
  icon: IconName;
  /** What the agent did, in a word: Read, Edit, Run… */
  verb: string;
  /** The file, command or pattern it was done to. */
  target: string;
  /** Changed lines for an edit, when the agent reported them. */
  diff?: DiffLine[];
  /** Lines left out of `diff` to keep the view short. */
  diffOmitted: number;
  /** Tool output, or the raw text when the step is not understood. */
  output: string;
};

const MAX_DIFF_LINES = 400;

type Input = Record<string, unknown>;
const text = (value: unknown) => (typeof value === "string" ? value : "");

/**
 * Claude Code steps carry their input as indented JSON, followed by a blank
 * line and the result once it arrives. Split the two. While the input is
 * still streaming it is not valid JSON yet, and the raw text is kept.
 */
export function splitToolText(raw: string): { input?: Input; output: string } {
  if (!raw.startsWith("{")) return { output: raw };
  const end =
    raw === "{}" || raw.startsWith("{}\n") ? 2 : raw.indexOf("\n}") + 2;
  if (end < 2) return { output: raw };
  try {
    const input: unknown = JSON.parse(raw.slice(0, end));
    if (input && typeof input === "object" && !Array.isArray(input))
      return {
        input: input as Input,
        output: raw.slice(end).replace(/^\n+/, ""),
      };
  } catch {
    // Not complete JSON: fall through to the raw text.
  }
  return { output: raw };
}

/** Show a path relative to the project when it lies inside it. */
export function shortPath(path: string, project: string) {
  const root = project.replace(/\/+$/, "");
  return root && path.startsWith(root + "/")
    ? path.slice(root.length + 1)
    : path;
}

const lines = (value: string, kind: DiffLine["kind"]): DiffLine[] =>
  value === ""
    ? []
    : value
        .replace(/\n$/, "")
        .split("\n")
        .map((line) => ({ kind, text: line }));

function editDiff(input: Input): DiffLine[] | undefined {
  if (Array.isArray(input.edits))
    return input.edits.flatMap((edit) => {
      const item = (edit || {}) as Input;
      return [
        ...lines(text(item.old_string), "del"),
        ...lines(text(item.new_string), "add"),
      ];
    });
  if ("old_string" in input || "new_string" in input)
    return [
      ...lines(text(input.old_string), "del"),
      ...lines(text(input.new_string), "add"),
    ];
  if (typeof input.content === "string") return lines(input.content, "add");
  return undefined;
}

/**
 * Turn a recorded tool step into what the conversation shows. Claude Code
 * reports a tool name with JSON input; Codex reports the command line itself
 * as the name. Anything not recognized is shown as it was recorded.
 */
export function describeToolStep(
  message: ChatMessage,
  project = "",
): ToolStepView {
  const name = (message.name || "Tool activity").trim();
  const key = name.toLowerCase();
  const { input, output } = splitToolText(message.text || "");
  const view = (
    icon: IconName,
    verb: string,
    target: string,
    extra: Partial<ToolStepView> = {},
  ): ToolStepView => ({
    icon,
    verb,
    target,
    diffOmitted: 0,
    output,
    ...extra,
  });
  const file = shortPath(
    text(input?.file_path) || text(input?.notebook_path),
    project,
  );

  if (/^(read|notebookread)$/.test(key)) return view("file", "Read", file);
  if (/^(edit|multiedit|write|notebookedit)$/.test(key)) {
    const all = input ? editDiff(input) : undefined;
    const diff = all?.slice(0, MAX_DIFF_LINES);
    return view("pencil", key === "write" ? "Write" : "Edit", file, {
      diff,
      diffOmitted: all && diff ? all.length - diff.length : 0,
    });
  }
  if (key === "bash")
    return view(
      "terminal",
      "Run",
      text(input?.command) || text(input?.description),
    );
  if (/^(grep|glob)$/.test(key))
    return view(
      "search",
      "Search",
      [text(input?.pattern), shortPath(text(input?.path), project)]
        .filter(Boolean)
        .join("  in  "),
    );
  if (/^(websearch|webfetch)$/.test(key))
    return view("search", "Web", text(input?.query) || text(input?.url));
  if (key === "file changes") return view("pencil", "Edit", "files");
  // A single word is a tool name; anything else is a command line (Codex).
  if (/^[a-z][\w.-]*$/i.test(name))
    return view("code", name, text(input?.description));
  return view("terminal", "Run", name);
}
