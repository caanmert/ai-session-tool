import AgentMark from "../AgentMark";
import Icon from "../Icon";
import { toolLabel, type ChatState } from "../api";
import { conversationStatus, sidebarOrder } from "../chat/conversation";
import SectionToggle from "./SectionToggle";

export const busyChat = (status: string) =>
  ["running", "waiting", "stopping"].includes(status);
// The dot already identifies the agent, so rows use the short name.
const shortName = (tool: string) =>
  tool === "claude" ? "Claude" : toolLabel(tool);
const projectName = (path: string) =>
  path.split("/").filter(Boolean).at(-1) || path;
/** The key a folded project group is remembered under. */
export const projectSection = (path: string) => `project:${path}`;
const needsYou = (chat: ChatState) =>
  chat.status === "waiting" || chat.status === "error";

/**
 * Open conversations in the sidebar, grouped by project. A folded group
 * still shows the conversation that is open, so you can see where you are.
 */
export default function OpenSessions({
  chats,
  active,
  onSelect,
  onClose,
  closing,
  folded = [],
  onFold,
  compact = false,
}: {
  chats: ChatState[];
  active: string;
  onSelect: (id: string) => void;
  onClose: (id: string) => void;
  closing: string;
  /** Sections folded away, by key. */
  folded?: string[];
  onFold?: (key: string) => void;
  /** One small button per conversation, for the narrow sidebar. */
  compact?: boolean;
}) {
  const { groups } = sidebarOrder(chats);
  const shown = [...groups].map(([project, items]) => {
    const isFolded = !compact && folded.includes(projectSection(project));
    return {
      project,
      items,
      isFolded,
      rows: isFolded ? items.filter((chat) => chat.id === active) : items,
    };
  });
  const visible = shown.flatMap((group) => group.rows);
  return (
    <nav
      className={`open-sessions ${compact ? "compact" : ""}`}
      aria-label="Open conversations"
      onKeyDown={(e) => {
        if (
          !(e.target instanceof HTMLButtonElement) ||
          !e.target.hasAttribute("data-open-session") ||
          !visible.length
        )
          return;
        let index = visible.findIndex((chat) => chat.id === active);
        const last = visible.length - 1;
        if (e.key === "ArrowDown") index = index >= last ? 0 : index + 1;
        else if (e.key === "ArrowUp") index = index <= 0 ? last : index - 1;
        else if (e.key === "Home") index = 0;
        else if (e.key === "End") index = last;
        else return;
        e.preventDefault();
        onSelect(visible[index].id);
        const rows = e.currentTarget.querySelectorAll<HTMLButtonElement>(
          "[data-open-session]",
        );
        rows[index]?.focus();
      }}
    >
      {shown.map(({ project, items, isFolded, rows }) => (
        <section key={project} aria-label={projectName(project)}>
          {compact ? (
            <hr className="rail-divider" />
          ) : (
            <SectionToggle
              label={projectName(project)}
              title={project}
              folded={isFolded}
              count={items.length}
              needs={items.filter(needsYou).length}
              onToggle={() => onFold?.(projectSection(project))}
            />
          )}
          {rows.map((chat) => {
            const title = chat.title || "New conversation";
            const attention = needsYou(chat);
            if (compact)
              return (
                <button
                  key={chat.id}
                  data-open-session
                  className={`rail-session ${active === chat.id ? "active" : ""}`}
                  aria-pressed={active === chat.id}
                  aria-label={`${title}, ${shortName(chat.tool)}, ${conversationStatus(chat)}`}
                  title={`${title}\n${shortName(chat.tool)} · ${conversationStatus(chat)}`}
                  onClick={() => onSelect(chat.id)}
                >
                  <span aria-hidden="true">
                    {title.trim().charAt(0).toUpperCase() || "?"}
                  </span>
                  <AgentMark tool={chat.tool} dot />
                  {(attention || busyChat(chat.status)) && (
                    <i className={`rail-state ${chat.status}`} />
                  )}
                </button>
              );
            return (
              <div
                key={chat.id}
                className={`session-item ${active === chat.id ? "active" : ""}`}
              >
                <button
                  data-open-session
                  aria-pressed={active === chat.id}
                  onClick={() => onSelect(chat.id)}
                  title={`${title}\n${chat.project}`}
                >
                  <strong>{title}</strong>
                  <small>
                    <AgentMark tool={chat.tool} dot />
                    <span>
                      {shortName(chat.tool)}
                      {chat.review ? " · Review" : ""} ·
                    </span>
                    <span className="visually-hidden"> </span>
                    <span className={`session-state ${chat.status}`}>
                      {attention && <Icon name="alert" size={12} />}
                      {conversationStatus(chat)}
                    </span>
                  </small>
                </button>
                <button
                  className="session-close"
                  aria-label={`Close ${title}`}
                  disabled={busyChat(chat.status) || !!closing}
                  title={
                    busyChat(chat.status)
                      ? "Stop the response before closing"
                      : "Close; saved work is kept"
                  }
                  onClick={() => onClose(chat.id)}
                >
                  <Icon name="close" size={13} />
                </button>
              </div>
            );
          })}
        </section>
      ))}
    </nav>
  );
}
