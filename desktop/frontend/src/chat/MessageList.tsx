import { memo, useEffect, useState } from "react";
import Markdown from "react-markdown";
import AgentMark from "../AgentMark";
import Icon, { type IconName } from "../Icon";
import { toolLabel, type ChatMessage } from "../api";

/** Provider output is rendered without active HTML, links or remote images. */
export const SafeMarkdown = memo(function SafeMarkdown({
  text,
}: {
  text: string;
}) {
  return (
    <Markdown
      components={{
        a: ({ children }) => (
          <span className="transcript-link">{children}</span>
        ),
        img: () => <span>[Image omitted]</span>,
      }}
    >
      {text}
    </Markdown>
  );
});

// Claude Code reports tool names (Read, Edit, Bash); Codex reports the command
// line itself. Pick an icon from the name and treat anything else as a command.
function stepKind(name: string): { icon: IconName; command: boolean } {
  const key = name.trim().toLowerCase();
  if (/^(read|notebookread)$/.test(key))
    return { icon: "file", command: false };
  if (/^(edit|multiedit|write|notebookedit|file changes)$/.test(key))
    return { icon: "pencil", command: false };
  if (/^(grep|glob|websearch|webfetch|toolsearch)$/.test(key))
    return { icon: "search", command: false };
  if (/^[a-z][a-z0-9_]*$/i.test(name.trim()) && key !== "bash")
    return { icon: "code", command: false };
  return { icon: "terminal", command: key !== "bash" };
}

const stepStatus = (status?: string) =>
  ({
    running: "Running",
    inProgress: "Running",
    completed: "Done",
    finished: "Done",
    failed: "Failed",
    declined: "Declined",
  })[status || ""] ||
  status ||
  "Saved";

const ToolStep = memo(function ToolStep({ message }: { message: ChatMessage }) {
  const name = message.name || "Tool activity";
  const kind = stepKind(name);
  const failed = message.status === "failed" || message.status === "declined";
  const running =
    message.status === "running" || message.status === "inProgress";
  return (
    <details className="tool-step">
      <summary>
        <Icon name={kind.icon} size={15} />
        <strong className={kind.command ? "command" : ""}>{name}</strong>
        <small
          className={failed ? "failed" : running ? "running" : ""}
          data-status={message.status || ""}
        >
          {failed && <Icon name="close" size={12} />}
          {stepStatus(message.status)}
        </small>
      </summary>
      <pre>{message.text || "Waiting for output…"}</pre>
    </details>
  );
});

function CopyButton({ text }: { text: string }) {
  const [state, setState] = useState<"" | "copied" | "failed">("");
  useEffect(() => {
    if (!state) return;
    const timer = setTimeout(() => setState(""), 1600);
    return () => clearTimeout(timer);
  }, [state]);
  return (
    <button
      className="message-copy"
      aria-label="Copy message"
      title="Copy message"
      onClick={() => {
        navigator.clipboard
          .writeText(text)
          .then(() => setState("copied"))
          .catch(() => setState("failed"));
      }}
    >
      <Icon name={state === "copied" ? "check" : "copy"} size={13} />
      <span role="status">
        {state === "copied"
          ? "Copied"
          : state === "failed"
            ? "Copy failed"
            : ""}
      </span>
    </button>
  );
}

// Rows are memoized: while a reply streams, only its own row renders again.
const TextMessage = memo(function TextMessage({
  message,
  tool,
}: {
  message: ChatMessage;
  tool: string;
}) {
  const user = message.role === "user";
  const system = message.role === "system";
  return (
    <article className={`chat-message ${message.role}`}>
      {user ? (
        <span className="avatar" aria-hidden="true">
          You
        </span>
      ) : system ? (
        <span className="avatar" aria-hidden="true">
          ais
        </span>
      ) : (
        <AgentMark tool={tool} small />
      )}
      <div className="chat-message-body">
        <div className="chat-message-author">
          {user ? "You" : system ? "ais" : toolLabel(tool)}
          {message.role === "assistant" && message.model && (
            <span className="message-model">{message.model}</span>
          )}
          {!system && message.text && <CopyButton text={message.text} />}
        </div>
        <div className="markdown">
          <SafeMarkdown text={message.text} />
        </div>
      </div>
    </article>
  );
});

/** Messages in order, with consecutive tool calls gathered into one block. */
export default function MessageList({
  messages,
  tool,
}: {
  messages: ChatMessage[];
  tool: string;
}) {
  const items: (ChatMessage | ChatMessage[])[] = [];
  for (const message of messages) {
    const last = items.at(-1);
    if (message.kind !== "tool") items.push(message);
    else if (Array.isArray(last)) last.push(message);
    else items.push([message]);
  }
  return items.map((item) =>
    Array.isArray(item) ? (
      <div
        className="tool-group"
        key={item[0].id}
        role="group"
        aria-label="Tool activity"
      >
        {item.map((message) => (
          <ToolStep key={message.id} message={message} />
        ))}
      </div>
    ) : (
      <TextMessage key={item.id} message={item} tool={tool} />
    ),
  );
}
