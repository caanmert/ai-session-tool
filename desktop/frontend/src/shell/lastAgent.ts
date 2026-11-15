import type { ToolName } from "../api";

const key = "ais:last-agent:v1";

function read(): Record<string, ToolName> {
  try {
    const saved: unknown = JSON.parse(window.localStorage.getItem(key) || "{}");
    return saved && typeof saved === "object" && !Array.isArray(saved)
      ? (saved as Record<string, ToolName>)
      : {};
  } catch {
    return {};
  }
}

/** The agent last started in a project from this app, if any. */
export function lastAgent(project: string): ToolName | undefined {
  const tool = read()[project];
  return tool === "codex" || tool === "claude" ? tool : undefined;
}

/** Remember which agent a task was started with. A convenience only: it is
 * fine for this to fail when storage is unavailable. */
export function rememberAgent(project: string, tool: ToolName) {
  try {
    window.localStorage.setItem(
      key,
      JSON.stringify({ ...read(), [project]: tool }),
    );
  } catch {
    // Not remembered; the dialog falls back to its usual default.
  }
}
