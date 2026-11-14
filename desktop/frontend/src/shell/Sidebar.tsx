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
  attention,
  onClick,
}: {
  icon: IconName;
  label: string;
  current?: boolean;
  count?: ReactNode;
  /** The count is something waiting for the user. */
  attention?: boolean;
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
      {count !== undefined && (
        <b className={attention ? "needs" : undefined}>{count}</b>
      )}
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
  onNew,
  onHistory,
  onRuns,
  onSaved,
  onSettings,
  onBoard,
  onSelectChat,
  onCloseChat,
}: {
  view: "history" | "runs" | "settings" | "board" | "chat";
  snapshot?: Snapshot;
  chats: ChatState[];
  activeChat: string;
  closingChat: string;
  newButton: Ref<HTMLButtonElement>;
  canStart: boolean;
  onNew: () => void;
  onHistory: () => void;
  onRuns: () => void;
  onSaved: () => void;
  onSettings: () => void;
  onBoard: () => void;
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
          icon="grid"
          label="Agents board"
          current={view === "board"}
          count={needsYou || undefined}
          attention={needsYou > 0}
          onClick={onBoard}
        />
        <NavItem
          icon="branch"
          label="Parallel runs"
          current={view === "runs"}
          onClick={onRuns}
        />
        <NavItem icon="folder" label="Saved workspace" onClick={onSaved} />
        <NavItem
          icon="settings"
          label="Settings"
          current={view === "settings"}
          onClick={onSettings}
        />
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
    </aside>
  );
}
