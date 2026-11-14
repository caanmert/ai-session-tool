import { useState } from "react";
import AgentMark from "../AgentMark";
import Icon, { type IconName } from "../Icon";
import {
  api,
  errorText,
  toolLabel,
  type ChatApproval,
  type ChatState,
  type Tool,
} from "../api";

const projectName = (path: string) =>
  path.split("/").filter(Boolean).at(-1) || path;

type Column = {
  id: string;
  title: string;
  icon: IconName;
  empty: string;
  chats: ChatState[];
};

/** Approve or deny in place. Requests with questions are answered in the
 * conversation, where the full context is visible. */
function InlineApproval({
  chat,
  approval,
  onOpen,
}: {
  chat: ChatState;
  approval: ChatApproval;
  onOpen: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function answer(allow: boolean) {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await api.AnswerChat(chat.id, approval.id, allow, {});
    } catch (e) {
      setError(errorText(e));
      setBusy(false);
    }
  }
  return (
    <div className="board-approval">
      <p className="attention">
        <Icon name="alert" size={13} />
        {approval.questions.length ? "Has a question" : "Needs approval"}
      </p>
      <p className="board-approval-title">{approval.title}</p>
      {approval.detail && !approval.questions.length && (
        <code title={approval.detail}>{approval.detail}</code>
      )}
      {error && <p role="alert">{error}</p>}
      {approval.questions.length ? (
        <button className="button secondary" onClick={onOpen}>
          Reply in conversation
        </button>
      ) : (
        <div className="board-actions">
          <button
            className="button primary"
            disabled={busy}
            onClick={() => void answer(true)}
          >
            Allow once
          </button>
          <button
            className="button secondary"
            disabled={busy}
            onClick={() => void answer(false)}
          >
            Deny
          </button>
        </div>
      )}
    </div>
  );
}

function TaskCard({
  chat,
  onOpen,
}: {
  chat: ChatState;
  onOpen: (id: string) => void;
}) {
  const title = chat.title || "New conversation";
  const open = () => onOpen(chat.id);
  return (
    <article
      className={`task-card ${chat.status}`}
      aria-label={`${title}, ${toolLabel(chat.tool)}`}
    >
      <header>
        <span className={`task-agent ${chat.tool}`}>
          <AgentMark tool={chat.tool} dot />
          {toolLabel(chat.tool)}
        </span>
        <span title={chat.project}>{projectName(chat.project)}</span>
      </header>
      <h3>
        <button onClick={open}>{title}</button>
      </h3>
      {chat.review && <p className="task-note">Independent review</p>}
      {chat.status === "waiting" &&
        (chat.approvals[0] ? (
          <InlineApproval
            // A new request replaces the card's state, including its error.
            key={chat.approvals[0].id}
            chat={chat}
            approval={chat.approvals[0]}
            onOpen={open}
          />
        ) : (
          <p className="attention">
            <Icon name="alert" size={13} /> Needs your input
          </p>
        ))}
      {(chat.status === "running" || chat.status === "stopping") && (
        <p className="task-note">
          {chat.status === "stopping" ? "Stopping…" : "Working…"}
          {chat.activeModel && <code>{chat.activeModel}</code>}
        </p>
      )}
      {chat.status === "error" && (
        <p className="task-problem">
          <Icon name="alert" size={13} />
          {chat.error || "The last response failed."}
        </p>
      )}
      {chat.status === "stopped" && (
        <p className="task-note">
          Stopped by you. Edits and the conversation are kept.
        </p>
      )}
      {chat.status === "idle" && (
        <p className="task-note">Ready for your next message.</p>
      )}
    </article>
  );
}

/**
 * Every open conversation across agents, grouped by what it needs. "Needs you"
 * comes first on purpose. Opening a card only shows the conversation.
 */
export default function Board({
  chats,
  tools,
  onOpen,
}: {
  chats: ChatState[];
  tools: Tool[];
  onOpen: (id: string) => void;
}) {
  const columns: Column[] = [
    {
      id: "needs",
      title: "Needs you",
      icon: "alert",
      empty: "Nothing is waiting for you.",
      chats: chats.filter((c) => c.status === "waiting"),
    },
    {
      id: "working",
      title: "Working",
      icon: "refresh",
      empty: "No agent is working.",
      chats: chats.filter(
        (c) => c.status === "running" || c.status === "stopping",
      ),
    },
    {
      id: "ready",
      title: "Ready",
      icon: "check",
      empty: "Finished and stopped conversations appear here.",
      chats: chats.filter((c) =>
        ["idle", "stopped", "error"].includes(c.status),
      ),
    },
  ];
  return (
    <div className="board" role="region" aria-label="Agents board">
      <div className="capacity-row">
        {tools.map((tool) => {
          const mine = chats.filter((chat) => chat.tool === tool.name);
          const waiting = mine.filter((c) => c.status === "waiting").length;
          const working = mine.filter(
            (c) => c.status === "running" || c.status === "stopping",
          ).length;
          return (
            <div className="capacity-card" key={tool.name}>
              <AgentMark tool={tool.name} />
              <div>
                <strong>{toolLabel(tool.name)}</strong>
                <small>
                  {working} working · {waiting} waiting · {mine.length} open
                </small>
              </div>
              {tool.available ? (
                <span className="capacity-state found">
                  <Icon name="check" size={12} /> Found
                </span>
              ) : (
                <span className="capacity-state missing">
                  <Icon name="alert" size={12} /> Not found
                </span>
              )}
            </div>
          );
        })}
      </div>
      {/* Columns keep their width and scroll sideways in a narrow window, so
          the region must be focusable to scroll it from the keyboard. */}
      <div
        className="board-columns"
        // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
        tabIndex={0}
        aria-label="Conversations by state"
        role="group"
      >
        {columns.map((column) => (
          <section key={column.id} aria-labelledby={`board-${column.id}`}>
            <h2 id={`board-${column.id}`} className={column.id}>
              <Icon name={column.icon} size={15} />
              {column.title}
              <span>{column.chats.length}</span>
            </h2>
            {column.chats.map((chat) => (
              <TaskCard key={chat.id} chat={chat} onOpen={onOpen} />
            ))}
            {!column.chats.length && (
              <p className="board-empty">{column.empty}</p>
            )}
          </section>
        ))}
      </div>
    </div>
  );
}
