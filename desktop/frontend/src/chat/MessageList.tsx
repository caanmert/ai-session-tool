import { memo, useCallback, useEffect, useRef, useState } from "react";
import { SafeMarkdown } from "../ui/SafeMarkdown";
import AgentMark from "../AgentMark";
import Icon from "../Icon";
import Foldable, { useReveal } from "../ui/Foldable";
import { describeToolStep, type ToolStepView } from "./toolStep";
import { toolLabel, type ChatMessage, type Checkpoint } from "../api";

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

const stepFailed = (message: ChatMessage) =>
  message.status === "failed" || message.status === "declined";

/** A group with more steps than this folds to its summary once it is done. */
export const GROUP_FOLD = 3;

const ToolStep = memo(function ToolStep({
  message,
  step,
  latestEdit,
}: {
  message: ChatMessage;
  step: ToolStepView;
  /** The latest edit in its group shows its diff until you close it. */
  latestEdit: boolean;
}) {
  const failed = stepFailed(message);
  const running =
    message.status === "running" || message.status === "inProgress";
  const added = step.diff?.filter((line) => line.kind === "add").length || 0;
  const removed = step.diff?.filter((line) => line.kind === "del").length || 0;
  // Your own choice wins; until you make one, only the latest edit is open.
  const [chosen, setChosen] = useState<boolean>();
  const open = chosen ?? latestEdit;
  return (
    <details
      className="tool-step"
      data-message-id={message.id}
      open={open}
      onToggle={(e) => {
        if (e.currentTarget.open !== open) setChosen(e.currentTarget.open);
      }}
    >
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

const plural = (count: number, one: string, many: string) =>
  `${count} ${count === 1 ? one : many}`;

/** "7 steps · 2 edits · 3 commands", for a folded group. */
export function groupSummary(steps: ToolStepView[]) {
  const count = (...verbs: string[]) =>
    steps.filter((step) => verbs.includes(step.verb)).length;
  const parts = [plural(steps.length, "step", "steps")];
  const edits = count("Edit", "Write");
  const commands = count("Run");
  const reads = count("Read");
  const searches = count("Search", "Web");
  if (edits) parts.push(plural(edits, "edit", "edits"));
  if (commands) parts.push(plural(commands, "command", "commands"));
  if (reads) parts.push(plural(reads, "read", "reads"));
  if (searches) parts.push(plural(searches, "search", "searches"));
  return parts.join(" · ");
}

type GroupProps = {
  messages: ChatMessage[];
  project: string;
  /** The group of a turn that is still running stays open. */
  live: boolean;
};

// A group renders again only when one of its own steps changes.
const ToolGroup = memo(function ToolGroup({
  messages,
  project,
  live,
}: GroupProps) {
  const ref = useRef<HTMLDivElement>(null);
  const steps = messages.map((message) => describeToolStep(message, project));
  const long = messages.length > GROUP_FOLD;
  const [chosen, setChosen] = useState<boolean>();
  const open = !long || (chosen ?? live);
  const failed = messages.filter(stepFailed).length;
  let latestEdit = -1;
  steps.forEach((step, index) => {
    if (step.diff?.length) latestEdit = index;
  });
  const reveal = useCallback(() => setChosen(true), []);
  useReveal(ref, !open, reveal);
  return (
    <div
      ref={ref}
      className={`tool-group ${long ? "foldable-group" : ""}`}
      role="group"
      aria-label="Tool activity"
      data-folded={open ? undefined : ""}
    >
      {long && (
        <button
          className="tool-group-summary"
          aria-expanded={open}
          onClick={() => setChosen(!open)}
        >
          <Icon name="chevron" size={12} />
          <span>{groupSummary(steps)}</span>
          {failed > 0 && <b className="failed">{failed} failed</b>}
        </button>
      )}
      <div className="tool-group-steps" hidden={!open}>
        {messages.map((message, index) => (
          <ToolStep
            key={message.id}
            message={message}
            step={steps[index]}
            latestEdit={index === latestEdit}
          />
        ))}
      </div>
    </div>
  );
}, sameGroup);

function sameGroup(a: GroupProps, b: GroupProps) {
  return (
    a.live === b.live &&
    a.project === b.project &&
    a.messages.length === b.messages.length &&
    a.messages.every((message, index) => message === b.messages[index])
  );
}

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

/** "14:05" today, "Mon 14:05" this week, "12 Nov 14:05" before that. */
export function messageTime(at: string, now = new Date()) {
  const date = new Date(at);
  if (Number.isNaN(date.getTime())) return "";
  const time = date.toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
  });
  if (date.toDateString() === now.toDateString()) return time;
  const days = (now.getTime() - date.getTime()) / 86_400_000;
  const day = date.toLocaleDateString(
    [],
    days < 6 && days > 0
      ? { weekday: "short" }
      : {
          day: "numeric",
          month: "short",
          ...(date.getFullYear() === now.getFullYear()
            ? {}
            : { year: "numeric" }),
        },
  );
  return `${day} ${time}`;
}

/** Lines a message takes, counting long lines as wrapping. */
const lineCount = (text: string) =>
  text
    .split("\n")
    .reduce((sum, line) => sum + Math.max(1, Math.ceil(line.length / 110)), 0);

// Rows are memoized: while a reply streams, only its own row renders again.
const TextMessage = memo(function TextMessage({
  message,
  tool,
  onReuse,
  imageNote,
  checkpoint,
  onRestore,
  onTurnDiff,
  restoreDisabled,
}: {
  message: ChatMessage;
  tool: string;
  onReuse?: (text: string) => void;
  imageNote?: string;
  /** The files as they were at this message, to go back to. */
  checkpoint?: Checkpoint;
  onRestore?: (point: Checkpoint) => void;
  /** Shows what changed since this message's turn began. */
  onTurnDiff?: (point: Checkpoint) => void;
  restoreDisabled?: boolean;
}) {
  const user = message.role === "user";
  const system = message.role === "system";
  const when = message.at ? messageTime(message.at) : "";
  const body = (
    <div className="markdown">
      <SafeMarkdown text={message.text} imageNote={imageNote} />
    </div>
  );
  return (
    <article
      className={`chat-message ${message.role}`}
      data-message-id={message.id}
    >
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
          {when && (
            <time
              className="message-time"
              dateTime={message.at}
              title={new Date(message.at!).toLocaleString()}
            >
              {when}
            </time>
          )}
          <span className="message-actions">
            {checkpoint && onRestore && (
              <button
                className="message-copy"
                aria-label={
                  checkpoint.kind === "turn"
                    ? "Restore files to before this turn"
                    : "Undo this"
                }
                title={
                  restoreDisabled
                    ? "Wait for the response to finish to restore files"
                    : checkpoint.kind === "turn"
                      ? "Put the project's files back as they were before this turn. You see the list first."
                      : "Put the files back as they were just before this. You see the list first."
                }
                disabled={restoreDisabled}
                onClick={() => onRestore(checkpoint)}
              >
                <Icon name="undo" size={13} />
              </button>
            )}
            {checkpoint?.kind === "turn" && onTurnDiff && (
              <button
                className="message-copy"
                aria-label="Changes since this message"
                title="See what changed in the project since this message. Changes nothing."
                onClick={() => onTurnDiff(checkpoint)}
              >
                <Icon name="code" size={13} />
              </button>
            )}
            {user && onReuse && message.text && (
              <button
                className="message-copy"
                aria-label="Reuse this message"
                title="Put this message in the message box to edit and send again. Nothing is sent."
                onClick={() => onReuse(message.text)}
              >
                <Icon name="pencil" size={13} />
              </button>
            )}
            {!system && message.text && <CopyButton text={message.text} />}
          </span>
        </div>
        {user ? (
          <Foldable lines={lineCount(message.text)} limit={12} keep={8}>
            {body}
          </Foldable>
        ) : (
          body
        )}
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
  busy = false,
  onReuse,
  imageNote,
  checkpoints,
  onRestore,
  onTurnDiff,
}: {
  messages: ChatMessage[];
  tool: string;
  /** Used to show file paths relative to the project. */
  project?: string;
  /** A turn is running: its tool activity stays open. */
  busy?: boolean;
  /** Puts one of your messages back in the message box. */
  onReuse?: (text: string) => void;
  /** What stands in for an image, which is never loaded. */
  imageNote?: string;
  /** Checkpoints by the message they belong to. */
  checkpoints?: Record<string, Checkpoint>;
  onRestore?: (point: Checkpoint) => void;
  onTurnDiff?: (point: Checkpoint) => void;
}) {
  const items: (ChatMessage | ChatMessage[])[] = [];
  for (const message of messages) {
    const last = items.at(-1);
    if (message.kind !== "tool") items.push(message);
    else if (Array.isArray(last)) last.push(message);
    else items.push([message]);
  }
  // Only the activity after your latest message belongs to the running turn.
  let lastUser = -1;
  items.forEach((item, index) => {
    if (!Array.isArray(item) && item.role === "user") lastUser = index;
  });
  return items.map((item, index) =>
    Array.isArray(item) ? (
      <ToolGroup
        key={item[0].id}
        messages={item}
        project={project}
        live={busy && index > lastUser}
      />
    ) : (
      <TextMessage
        key={item.id}
        message={item}
        tool={tool}
        onReuse={onReuse}
        imageNote={imageNote}
        checkpoint={checkpoints?.[item.id]}
        onRestore={onRestore}
        onTurnDiff={onTurnDiff}
        restoreDisabled={busy}
      />
    ),
  );
}
