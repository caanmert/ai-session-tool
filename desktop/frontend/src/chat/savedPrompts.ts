import type { SavedPrompt } from "../api";

/** What a saved prompt's placeholders are filled with. */
export type PromptContext = {
  project: string;
  branch?: string;
  files?: string[];
};

/** Fills {project}, {branch} and {changed files} in a saved prompt. */
export function fillPrompt(text: string, context: PromptContext) {
  const files = context.files?.length
    ? context.files.join(", ")
    : "(no uncommitted changes)";
  return text
    .replaceAll("{project}", context.project || "this project")
    .replaceAll("{branch}", context.branch || "the current branch")
    .replaceAll("{changed files}", files);
}

/** Saved prompts matching what was typed after "/": names first, then text. */
export function matchPrompts(prompts: SavedPrompt[], query: string) {
  const q = query.toLowerCase();
  if (!q) return prompts;
  const byName = prompts.filter((p) => p.name.toLowerCase().includes(q));
  const byText = prompts.filter(
    (p) =>
      !p.name.toLowerCase().includes(q) && p.text.toLowerCase().includes(q),
  );
  return [...byName, ...byText];
}
