import { useState } from "react";
import AgentMark from "../AgentMark";
import Icon, { type IconName } from "../Icon";
import { requestSummary } from "../chat/conversation";
import {
  runTaskColumn,
  runTaskState,
  useRunTasks,
  type BoardRunTask,
} from "./runTasks";
import {
  api,
  errorText,
  toolLabel,
  type AgentUsage,
  type ChatApproval,
  type ChatState,
  type Tool,
} from "../api";
import { resetClock, tightest } from "../usage/limits";

const projectName = (path: string) =>
  path.split("/").filter(Boolean).at(-1) || path;

/**
 * The agent's limit closest to running out, as a bar with its reset time. It
 * opens the Usage limits page. Nothing is shown when no limits were read, so
 * a missing figure never looks like 0%.
 */
function CapacityLimit({
  agent,
  onOpen,
}: {
  agent: AgentUsage;
  onOpen: () => void;
}) {
  const worst = tightest(agent);
  if (!worst) return null;
  const limit = agent.blocked
    ? agent.limits.find((item) => item.level === "reached") || worst
    : worst;
  const level = agent.blocked ? "reached" : limit.level;
  const percent = Math.round(limit.percent);
  const at = limit.resetsAt ? new Date(limit.resetsAt) : undefined;
  const resets = at ? ` · resets ${resetClock(at, new Date())}` : "";
  const what =
    limit.window === "session"
      ? "5-hour session"
      : limit.window === "weekly"
        ? "week"
        : limit.label;
  const text = agent.blocked
    ? `Limit reached${resets}`
    : `${percent}% of ${what}${resets}`;
  return (
    <button
      className={`capacity-limit ${level}`}
      aria-label={`${toolLabel(agent.tool)}: ${text}. Open usage limits`}
      title={`${limit.label}: ${percent}% used`}
      onClick={onOpen}
    >
      <span className={`usage-meter ${level}`} aria-hidden="true">
        <i style={{ width: `${Math.min(100, Math.max(0, limit.percent))}%` }} />
      </span>
      <span className="capacity-limit-text">{text}</span>
    </button>
  );
}

type Column = {
  id: string;
  title: string;
  icon: IconName;
  empty: string;
  chats: ChatState[];
  runs: BoardRunTask[];
};

/** A task of a parallel run. Its process state and whether it wants attention
 * are shown as two separate chips, never merged into one label. */
function RunTaskCard({
  task,
  onOpen,
}: {
  task: BoardRunTask;
  onOpen: (run: string, window: string) => void;
}) {
  const { run, status } = task;
  const files = status.files.length;
  return (
    <article
      className={`task-card run ${status.error ? "error" : ""}`}
      aria-label={`${status.task.prompt}, parallel run task`}
    >
      <header>
        <span className={`task-agent ${run.tool}`}>
          <AgentMark tool={run.tool} dot />
          {toolLabel(run.tool)} · run
        </span>
        <span title={run.repository}>{projectName(run.repository)}</span>
      </header>
      <h3>
        <button onClick={() => onOpen(run.id, status.task.window)}>
          {status.task.prompt}
        </button>
      </h3>
      <p className="task-chips">
        <span className="chip outline">{runTaskState(status)}</span>
        {status.attention && (
          <span className="chip status waiting">
            <Icon name="alert" size={12} /> Agent event
          </span>
        )}
        {status.quiet && (
          <span className="chip status waiting">
            <Icon name="alert" size={12} /> Gone quiet
          </span>
        )}
      </p>
      {status.error ? (
        <p className="task-problem">
          <Icon name="alert" size={13} />
          {status.error}
        </p>
      ) : (
        <p className="task-note">
          {files
            ? `${files} changed ${files === 1 ? "file" : "files"} to review`
            : "No changed files yet"}
        </p>
      )}
    </article>
  );
}

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
        {approval.questions.length
          ? "Has a question"
          : approval.plan
            ? "Plan to approve"
            : "Needs approval"}
      </p>
      <p className="board-approval-title">{approval.title}</p>
      {approval.plan && <p className="task-note">A plan is ready to review.</p>}
      {approval.detail && !approval.questions.length && !approval.plan && (
        <code title={requestSummary(approval.detail).text}>
          {requestSummary(approval.detail).text}
        </code>
      )}
      {error && <p role="alert">{error}</p>}
      {approval.questions.length || approval.plan ? (
        <button className="button secondary" onClick={onOpen}>
          {approval.plan ? "Review in conversation" : "Reply in conversation"}
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
  onOpenRun,
  usage,
  onUsage = () => {},
}: {
  chats: ChatState[];
  tools: Tool[];
  onOpen: (id: string) => void;
  /** Open a task of a parallel run on the Parallel runs page. */
  onOpenRun: (run: string, window: string) => void;
  /** Each agent's plan limits, once read. */
  usage?: AgentUsage[];
  onUsage?: () => void;
}) {
  const runs = useRunTasks(true);
  const inColumn = (column: string) =>
    runs.tasks.filter((task) => runTaskColumn(task.status) === column);
  const columns: Column[] = [
    {
      id: "needs",
      title: "Needs you",
      icon: "alert",
      empty: "Nothing is waiting for you.",
      chats: chats.filter((c) => c.status === "waiting"),
      runs: inColumn("needs"),
    },
    {
      id: "working",
      title: "Working",
      icon: "refresh",
      empty: "No agent is working.",
      chats: chats.filter(
        (c) => c.status === "running" || c.status === "stopping",
      ),
      runs: inColumn("working"),
    },
    {
      id: "ready",
      title: "Ready",
      icon: "check",
      empty: "Finished and stopped conversations appear here.",
      chats: chats.filter((c) =>
        ["idle", "stopped", "error"].includes(c.status),
      ),
      runs: inColumn("ready"),
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
          const limits = tool.available
            ? usage?.find((agent) => agent.tool === tool.name)
            : undefined;
          return (
            <div className="capacity-card" key={tool.name}>
              <AgentMark tool={tool.name} />
              <div>
                <strong>{toolLabel(tool.name)}</strong>
                <small>
                  {working} working · {waiting} waiting · {mine.length} open
                </small>
                {limits && <CapacityLimit agent={limits} onOpen={onUsage} />}
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
      {runs.error && (
        <p className="board-note" role="status">
          Some parallel runs could not be read: {runs.error}
        </p>
      )}
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
              <span>{column.chats.length + column.runs.length}</span>
            </h2>
            {column.chats.map((chat) => (
              <TaskCard key={chat.id} chat={chat} onOpen={onOpen} />
            ))}
            {column.runs.map((task) => (
              <RunTaskCard
                key={`${task.run.id}/${task.status.task.window}`}
                task={task}
                onOpen={onOpenRun}
              />
            ))}
            {!column.chats.length && !column.runs.length && (
              <p className="board-empty">{column.empty}</p>
            )}
          </section>
        ))}
      </div>
    </div>
  );
}
