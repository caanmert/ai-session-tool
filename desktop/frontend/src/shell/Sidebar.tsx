import type { ReactNode, Ref } from "react";
import AgentMark from "../AgentMark";
import Icon, { type IconName } from "../Icon";
import { toolLabel, type ChatState, type Snapshot } from "../api";
import OpenSessions, { busyChat } from "./OpenSessions";

function NavItem({
  icon,
  label,
  current,
  count,
  onClick,
}: {
  icon: IconName;
  label: string;
  current?: boolean;
  count?: ReactNode;
  onClick: () => void;
}) {
  return (
    <button
      className="nav-item"
      aria-current={current ? "page" : undefined}
      onClick={onClick}
    >
      <Icon name={icon} size={17} />
      <span>{label}</span>
      {count !== undefined && <b>{count}</b>}
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
  footer,
  onNew,
  onHistory,
  onRuns,
  onSaved,
  onSelectChat,
  onCloseChat,
}: {
  view: "history" | "runs" | "chat";
  snapshot?: Snapshot;
  chats: ChatState[];
  activeChat: string;
  closingChat: string;
  newButton: Ref<HTMLButtonElement>;
  canStart: boolean;
  footer: ReactNode;
  onNew: () => void;
  onHistory: () => void;
  onRuns: () => void;
  onSaved: () => void;
  onSelectChat: (id: string) => void;
  onCloseChat: (id: string) => void;
}) {
  const needsYou = chats.filter((chat) => chat.status === "waiting").length;
  return (
    <aside className="sidebar" aria-label="Workspace">
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
        <NavItem
          icon="message"
          label="History"
          current={view === "history"}
          count={snapshot?.sessions.length ?? "—"}
          onClick={onHistory}
        />
        <NavItem
          icon="branch"
          label="Parallel runs"
          current={view === "runs"}
          onClick={onRuns}
        />
        <NavItem icon="folder" label="Saved workspace" onClick={onSaved} />
      </div>
      <div className="sidebar-scroll">
        <h2 className="section-label">Agents</h2>
        {snapshot?.tools.map((tool) => {
          const mine = chats.filter((chat) => chat.tool === tool.name);
          const waiting = mine.filter((c) => c.status === "waiting").length;
          const running =
            mine.filter((c) => busyChat(c.status)).length - waiting;
          return (
            <div
              className="agent-row"
              key={tool.name}
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
              closing={closingChat}
            />
          </>
        )}
      </div>
      <div className="sidebar-footer">{footer}</div>
    </aside>
  );
}
