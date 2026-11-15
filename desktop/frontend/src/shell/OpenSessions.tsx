import AgentMark from "../AgentMark";
import Icon from "../Icon";
import { toolLabel, type ChatState } from "../api";
import { conversationStatus, sidebarOrder } from "../chat/conversation";

export const busyChat = (status: string) =>
  ["running", "waiting", "stopping"].includes(status);
// The dot already identifies the agent, so rows use the short name.
const shortName = (tool: string) =>
  tool === "claude" ? "Claude" : toolLabel(tool);
const projectName = (path: string) =>
  path.split("/").filter(Boolean).at(-1) || path;

/** Open conversations in the sidebar, grouped by project. */
export default function OpenSessions({
  chats,
  active,
  onSelect,
  onClose,
  closing,
}: {
  chats: ChatState[];
  active: string;
  onSelect: (id: string) => void;
  onClose: (id: string) => void;
  closing: string;
}) {
  const { groups, ordered } = sidebarOrder(chats);
  return (
    <nav
      className="open-sessions"
      aria-label="Open conversations"
      onKeyDown={(e) => {
        if (
          !(e.target instanceof HTMLButtonElement) ||
          !e.target.hasAttribute("data-open-session")
        )
          return;
        let index = ordered.findIndex((chat) => chat.id === active);
        const last = ordered.length - 1;
        if (e.key === "ArrowDown") index = index >= last ? 0 : index + 1;
        else if (e.key === "ArrowUp") index = index <= 0 ? last : index - 1;
        else if (e.key === "Home") index = 0;
        else if (e.key === "End") index = last;
        else return;
        e.preventDefault();
        onSelect(ordered[index].id);
        const rows = e.currentTarget.querySelectorAll<HTMLButtonElement>(
          "[data-open-session]",
        );
        rows[index]?.focus();
      }}
    >
      {[...groups].map(([project, items]) => (
        <section key={project} aria-label={projectName(project)}>
          <h2 className="section-label" title={project}>
            {projectName(project)}
          </h2>
          {items.map((chat) => {
            const title = chat.title || "New conversation";
            const attention =
              chat.status === "waiting" || chat.status === "error";
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
