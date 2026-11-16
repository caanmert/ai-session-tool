import type { ChatMessage, Message } from "../api";

// Saved transcripts keep a one-line summary of each tool call: the command,
// file or pattern. It goes back under the key the step view reads it from.
const inputKey: Record<string, string> = {
  read: "file_path",
  edit: "file_path",
  multiedit: "file_path",
  write: "file_path",
  notebookread: "notebook_path",
  notebookedit: "notebook_path",
  bash: "command",
  grep: "pattern",
  glob: "pattern",
  webfetch: "url",
  websearch: "query",
};

/**
 * A saved transcript in the shape open conversations use, so History shows it
 * the same way. Results are paired with calls in the order they were made; a
 * result without a call and reasoning are kept as steps of their own.
 */
export function previewMessages(messages: Message[]): ChatMessage[] {
  const out: ChatMessage[] = [];
  // Calls still waiting for their result, oldest first.
  const waiting: ChatMessage[] = [];
  messages.forEach((message, index) => {
    const id = `saved-${index}`;
    const text = message.text || "";
    switch (message.kind) {
      case "text":
        out.push({
          id,
          role: message.role,
          kind: "text",
          text,
          at: message.time,
        });
        return;
      case "tool_use": {
        const name = message.toolName || "Tool";
        const key = inputKey[name.toLowerCase()] || "description";
        const step: ChatMessage = {
          id,
          role: "assistant",
          kind: "tool",
          name,
          // Indented JSON is what the step view reads tool input from.
          text: JSON.stringify({ [key]: text }, null, 2),
        };
        out.push(step);
        waiting.push(step);
        return;
      }
      case "tool_result": {
        const call = waiting.shift();
        if (call) {
          call.text = `${call.text}\n\n${text}`;
          call.status = "completed";
        } else
          out.push({
            id,
            role: "assistant",
            kind: "tool",
            name: "Output",
            text,
            status: "completed",
          });
        return;
      }
      case "thinking":
        out.push({
          id,
          role: "assistant",
          kind: "tool",
          name: "Reasoning",
          text,
          status: "completed",
        });
        return;
      default:
        // A kind this app doesn't know yet is still shown, as a plain step.
        out.push({
          id,
          role: "assistant",
          kind: "tool",
          name: message.toolName || "Output",
          text,
        });
    }
  });
  return out;
}
