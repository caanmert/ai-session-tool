import type { ReactNode, Ref } from "react";
import AgentMark from "../AgentMark";
import Icon, { type IconName } from "../Icon";
import {
  toolLabel,
  type AgentUsage,
  type ChatState,
  type Snapshot,
} from "../api";
import { resetClock, tightest } from "../usage/limits";
import OpenSessions, { busyChat } from "./OpenSessions";
import SectionToggle from "./SectionToggle";

/** The key a folded "Agents" section is remembered under. */
export const AGENTS_SECTION = "agents";

function NavItem({
  icon,
  label,
  current,
  count,
  attention,
  compact,
  onClick,
}: {
  icon: IconName;
  label: string;
  current?: boolean;
  count?: ReactNode;
  /** The count is something waiting for the user. */
  attention?: boolean;
  /** Icon only, for the narrow sidebar. Only counts that need you show. */
  compact?: boolean;
  onClick: () => void;
}) {
  if (compact)
    return (
      <button
        className="nav-item compact"
        aria-current={current ? "page" : undefined}
        aria-label={attention ? `${label}, ${count} need you` : label}
        title={label}
        onClick={onClick}
      >
        <Icon name={icon} size={18} />
        {attention && <b className="rail-badge">{count}</b>}
      </button>
    );
  return (
    <button
      className="nav-item"
      aria-current={current ? "page" : undefined}
      onClick={onClick}
    >
      <Icon name={icon} size={17} />
      <span>{label}</span>
      {count !== undefined && (
        <b className={attention ? "needs" : undefined}>{count}</b>
      )}
    </button>
  );
}

/** An agent's session and weekly use in a few words. */
function limitParts(agent: AgentUsage) {
  const worst = tightest(agent);
  if (!worst) return [];
  const parts: { text: string; level: string }[] = [];
  if (agent.blocked) {
    const reached =
      agent.limits.find((limit) => limit.level === "reached") || worst;
    const at = reached.resetsAt ? new Date(reached.resetsAt) : undefined;
    parts.push({
      text: at
        ? `Limit reached · resets ${resetClock(at, new Date())}`
        : "Limit reached",
      level: "reached",
    });
    return parts;
  }
  for (const [window, word] of [
    ["session", "Session"],
    ["weekly", "Week"],
  ]) {
    const limits = agent.limits.filter((limit) => limit.window === window);
    if (!limits.length) continue;
    const top = limits.reduce((a, b) => (b.percent > a.percent ? b : a));
    parts.push({
      text: `${word} ${Math.round(top.percent)}%`,
      level: top.level,
    });
  }
  if (!parts.length)
    parts.push({
      text: `${worst.label} ${Math.round(worst.percent)}%`,
      level: worst.level,
    });
  return parts;
}

/** The agent's session and weekly use, under its row; opens Usage limits. */
function LimitGlance({
  agent,
  onOpen,
}: {
  agent: AgentUsage;
  onOpen: () => void;
}) {
  const parts = limitParts(agent);
  if (!parts.length) return null;
  const summary = parts.map((part) => part.text).join(", ");
  return (
    <button
      className="agent-limit"
      aria-label={`${toolLabel(agent.tool)}: ${summary}. Open usage limits`}
      title="Usage"
      onClick={onOpen}
    >
      {parts.map((part, i) => (
        <span key={part.text} className={part.level}>
          {i > 0 && <span className="sep"> · </span>}
          {part.text}
        </span>
      ))}
    </button>
  );
}

export default function Sidebar({
  view,
  snapshot,
  chats,
  activeChat,
  closingChat,
  newButton,
  canStart,
  rail = false,
  folded = [],
  onFold = () => {},
  onNew,
  onHistory,
  onRuns,
  onSaved,
  onSettings,
  onBoard,
  onIssues,
  usage,
  onUsage,
  onSelectChat,
  onCloseChat,
  onBesideChat,
  besideChat,
}: {
  view: "history" | "runs" | "settings" | "board" | "github" | "usage" | "chat";
  snapshot?: Snapshot;
  chats: ChatState[];
  activeChat: string;
  closingChat: string;
  newButton: Ref<HTMLButtonElement>;
  canStart: boolean;
  /** The narrow sidebar: icons only, with names in tooltips. */
  rail?: boolean;
  /** Sections folded away, by key. */
  folded?: string[];
  onFold?: (key: string) => void;
  onNew: () => void;
  onHistory: () => void;
  onRuns: () => void;
  onSaved: () => void;
  onSettings: () => void;
  onBoard: () => void;
  onIssues: () => void;
  /** Each agent's plan limits, once read. */
  usage?: AgentUsage[];
  onUsage: () => void;
  onSelectChat: (id: string) => void;
  onCloseChat: (id: string) => void;
  /** Opens a conversation beside the current one, on wide windows. */
  onBesideChat?: (id: string) => void;
  /** The conversation in the other pane. */
  besideChat?: string;
}) {
  const needsYou = chats.filter((chat) => chat.status === "waiting").length;
  const nav: Parameters<typeof NavItem>[0][] = [
    {
      icon: "message",
      label: "History",
      current: view === "history",
      count: rail ? undefined : (snapshot?.sessions.length ?? "—"),
      onClick: onHistory,
    },
    {
      icon: "grid",
      label: "Agents board",
      current: view === "board",
      count: needsYou || undefined,
      attention: needsYou > 0,
      onClick: onBoard,
    },
    {
      icon: "issue",
      label: "Issues & PRs",
      current: view === "github",
      onClick: onIssues,
    },
    {
      icon: "gauge",
      label: "Usage",
      current: view === "usage",
      onClick: onUsage,
    },
    {
      icon: "branch",
      label: "Parallel runs",
      current: view === "runs",
      onClick: onRuns,
    },
    { icon: "folder", label: "Saved work", onClick: onSaved },
    {
      icon: "settings",
      label: "Settings",
      current: view === "settings",
      onClick: onSettings,
    },
  ];
  const tools = snapshot?.tools || [];
  if (rail)
    return (
      <aside className="sidebar rail" id="app-sidebar" aria-label="Workspace">
        <button
          ref={newButton}
          className="button primary new-task compact"
          aria-label="New task"
          title="New task (⌘N)"
          onClick={onNew}
          disabled={!canStart}
        >
          <Icon name="plus" size={18} />
        </button>
        <div className="sidebar-nav">
          {nav.map((item) => (
            <NavItem key={item.label} {...item} compact />
          ))}
        </div>
        <div className="sidebar-scroll">
          <div className="rail-agents" role="group" aria-label="Agents">
            {tools.map((tool) => {
              const limits = tool.available
                ? usage?.find((agent) => agent.tool === tool.name)
                : undefined;
              const parts = limits ? limitParts(limits) : [];
              const level = parts.some((part) => part.level === "reached")
                ? "reached"
                : parts.some((part) => part.level === "warning")
                  ? "warning"
                  : "";
              const state = tool.available
                ? parts.map((part) => part.text).join(", ") || "Ready"
                : "Not found";
              return (
                <button
                  key={tool.name}
                  className={`rail-agent ${level} ${tool.available ? "" : "missing"}`}
                  aria-label={`${toolLabel(tool.name)}: ${state}. Open usage limits`}
                  title={`${toolLabel(tool.name)} · ${state}`}
                  onClick={onUsage}
                >
                  <AgentMark tool={tool.name} small />
                </button>
              );
            })}
          </div>
          {chats.length > 0 && (
            <OpenSessions
              chats={chats}
              active={activeChat}
              onSelect={onSelectChat}
              onClose={onCloseChat}
              closing={closingChat}
              compact
            />
          )}
        </div>
      </aside>
    );
  const agentsFolded = folded.includes(AGENTS_SECTION);
  return (
    <aside className="sidebar" id="app-sidebar" aria-label="Workspace">
      <button
        ref={newButton}
        className="button primary new-task"
        onClick={onNew}
        disabled={!canStart}
      >
        <Icon name="plus" size={16} />
        <span>New task</span>
        <kbd>⌘N</kbd>
      </button>
      <div className="sidebar-nav">
        {nav.map((item) => (
          <NavItem key={item.label} {...item} />
        ))}
      </div>
      <div className="sidebar-scroll">
        <SectionToggle
          label="Agents"
          folded={agentsFolded}
          count={tools.length}
          onToggle={() => onFold(AGENTS_SECTION)}
        />
        {!agentsFolded &&
          tools.map((tool) => {
            const mine = chats.filter((chat) => chat.tool === tool.name);
            const waiting = mine.filter((c) => c.status === "waiting").length;
            const running =
              mine.filter((c) => busyChat(c.status)).length - waiting;
            const limits = tool.available
              ? usage?.find((agent) => agent.tool === tool.name)
              : undefined;
            return (
              <div className="agent-block" key={tool.name}>
                <div
                  className="agent-row"
                  title={tool.available ? tool.path : undefined}
                >
                  <AgentMark tool={tool.name} dot />
                  <span>{toolLabel(tool.name)}</span>
                  {tool.available ? (
                    <small>
                      {running
                        ? `${running} running`
                        : waiting
                          ? `${waiting} waiting`
                          : "Ready"}
                    </small>
                  ) : (
                    <small className="missing">
                      <Icon name="alert" size={12} /> Not found
                    </small>
                  )}
                </div>
                {limits && <LimitGlance agent={limits} onOpen={onUsage} />}
              </div>
            );
          })}
        {chats.length > 0 && (
          <>
            {needsYou > 0 && (
              <p className="needs-you" role="status">
                <Icon name="alert" size={13} />
                {needsYou === 1
                  ? "1 conversation needs you"
                  : `${needsYou} conversations need you`}
              </p>
            )}
            <OpenSessions
              chats={chats}
              active={activeChat}
              onSelect={onSelectChat}
              onClose={onCloseChat}
              onBeside={onBesideChat}
              beside={besideChat}
              closing={closingChat}
              folded={folded}
              onFold={onFold}
            />
          </>
        )}
      </div>
    </aside>
  );
}
