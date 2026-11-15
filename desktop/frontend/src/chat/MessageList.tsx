import { memo, useEffect, useState } from "react";
import { SafeMarkdown } from "../ui/SafeMarkdown";
import AgentMark from "../AgentMark";
import Icon from "../Icon";
import { describeToolStep } from "./toolStep";
import { toolLabel, type ChatMessage } from "../api";

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

const ToolStep = memo(function ToolStep({
  message,
  project,
}: {
  message: ChatMessage;
  project: string;
}) {
  const step = describeToolStep(message, project);
  const failed = message.status === "failed" || message.status === "declined";
  const running =
    message.status === "running" || message.status === "inProgress";
  const added = step.diff?.filter((line) => line.kind === "add").length || 0;
  const removed = step.diff?.filter((line) => line.kind === "del").length || 0;
  return (
    <details className="tool-step">
      <summary>
        <Icon name={step.icon} size={15} />
        <span className="tool-verb">{step.verb}</span>
        <strong title={step.target}>{step.target}</strong>
        {step.diff && (added > 0 || removed > 0) && (
          <span className="tool-counts">
            {added > 0 && <b className="added">+{added}</b>}
            {removed > 0 && <b className="removed">−{removed}</b>}
          </span>
        )}
        <small
          className={failed ? "failed" : running ? "running" : ""}
          data-status={message.status || ""}
        >
          {failed && <Icon name="close" size={12} />}
          {stepStatus(message.status)}
        </small>
      </summary>
      {step.diff && step.diff.length > 0 && (
        <div className="tool-diff" role="group" aria-label="Changed lines">
          {step.diff.map((line, index) => (
            <div key={index} className={line.kind}>
              <span aria-hidden="true">{line.kind === "add" ? "+" : "−"}</span>
              <code>{line.text || " "}</code>
            </div>
          ))}
          {step.diffOmitted > 0 && (
            <p>{step.diffOmitted} more lines are not shown.</p>
          )}
        </div>
      )}
      {(step.output || !step.diff?.length) && (
        <pre>{step.output || "Waiting for output…"}</pre>
      )}
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
        {!!message.images?.length && (
          <ul
            className="message-images"
            aria-label="Images sent with this message"
          >
            {message.images.map((name, index) => (
              <li key={index}>
                <Icon name="image" size={12} />
                {name}
              </li>
            ))}
          </ul>
        )}
      </div>
    </article>
  );
});

/** Messages in order, with consecutive tool calls gathered into one block. */
export default function MessageList({
  messages,
  tool,
  project = "",
}: {
  messages: ChatMessage[];
  tool: string;
  /** Used to show file paths relative to the project. */
  project?: string;
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
          <ToolStep key={message.id} message={message} project={project} />
        ))}
      </div>
    ) : (
      <TextMessage key={item.id} message={item} tool={tool} />
    ),
  );
}
