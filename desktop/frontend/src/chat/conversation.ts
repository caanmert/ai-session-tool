import { toolLabel, type ChatState } from "../api";

const labels: Record<string, string> = {
  idle: "Ready",
  running: "Working",
  stopping: "Stopping",
  stopped: "Stopped",
  error: "Needs attention",
};

/**
 * A conversation's state in words. While it waits for the user, say what it
 * waits for: an approval or an answer.
 */
export function conversationStatus(
  chat: Pick<ChatState, "status" | "approvals">,
) {
  if (chat.status === "waiting") {
    const request = chat.approvals?.[0];
    if (!request) return "Needs your input";
    return request.questions?.length ? "Has a question" : "Needs approval";
  }
  return labels[chat.status] || chat.status;
}

/** Open conversations in the order the sidebar shows them: grouped by project. */
export function sidebarOrder(chats: ChatState[]) {
  const groups = new Map<string, ChatState[]>();
  for (const chat of chats)
    groups.set(chat.project, [...(groups.get(chat.project) || []), chat]);
  return { groups, ordered: [...groups.values()].flat() };
}

/**
 * The conversation as Markdown, for pasting elsewhere. Tool steps are listed
 * by name and status; their output is left out to keep the copy readable.
 */
export function conversationMarkdown(chat: ChatState) {
  const agent = toolLabel(chat.tool);
  const parts = [
    `# ${chat.title || "Conversation"}`,
    `${agent} · ${chat.project}`,
  ];
  let steps: string[] = [];
  const flush = () => {
    if (steps.length) parts.push(steps.join("\n"));
    steps = [];
  };
  for (const message of chat.messages) {
    if (message.kind === "tool") {
      const status = message.status ? ` (${message.status})` : "";
      steps.push(`- \`${message.name || "Tool"}\`${status}`);
      continue;
    }
    flush();
    if (!message.text) continue;
    const author =
      message.role === "user"
        ? "You"
        : message.role === "system"
          ? "ais"
          : agent;
    parts.push(`**${author}**\n\n${message.text}`);
  }
  flush();
  return parts.join("\n\n") + "\n";
}

// What a request asks for, in a readable form. Agents send the details as
// JSON; a command is shown as the command, with the agent's own description
// of it above. Anything else is shown as it was sent.
export function requestSummary(detail: string): { text: string; note: string } {
  try {
    const parsed: unknown = JSON.parse(detail);
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      const input = parsed as Record<string, unknown>;
      if (typeof input.command === "string" && input.command)
        return {
          text: input.command,
          note:
            typeof input.description === "string"
              ? input.description
              : typeof input.reason === "string"
                ? input.reason
                : "",
        };
    }
  } catch {
    // Not JSON: show it as sent.
  }
  return { text: detail, note: "" };
}
