import { toolLabel, type ChatState } from "./api";

export const busyChat = (status: string) =>
  ["running", "waiting", "stopping"].includes(status);
export const chatStatus = (status: string) =>
  ({
    idle: "Ready",
    running: "Working",
    waiting: "Needs your input",
    stopping: "Stopping",
    stopped: "Stopped",
    error: "Needs attention",
  })[status] || status;

export default function WorkspaceTabs({
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
  return (
    <nav
      className="live-tabs"
      aria-label="Workspace tabs"
      onKeyDown={(e) => {
        if (
          !(e.target instanceof HTMLButtonElement) ||
          !e.target.hasAttribute("data-workspace-tab")
        )
          return;
        const ids = ["", ...chats.map((chat) => chat.id)];
        let index = ids.indexOf(active);
        if (e.key === "ArrowRight") index = (index + 1) % ids.length;
        else if (e.key === "ArrowLeft")
          index = (index - 1 + ids.length) % ids.length;
        else if (e.key === "Home") index = 0;
        else if (e.key === "End") index = ids.length - 1;
        else return;
        e.preventDefault();
        onSelect(ids[index]);
        e.currentTarget
          .querySelectorAll<HTMLButtonElement>("[data-workspace-tab]")
          [index]?.focus();
      }}
    >
      <button
        data-workspace-tab
        className={!active ? "active" : ""}
        aria-pressed={!active}
        onClick={() => onSelect("")}
      >
        History
      </button>
      {chats.map((chat) => (
        <div
          key={chat.id}
          className={`workspace-tab ${active === chat.id ? "active" : ""}`}
        >
          <button
            data-workspace-tab
            aria-pressed={active === chat.id}
            onClick={() => onSelect(chat.id)}
            title={`${chat.title}\n${chat.project}\n${chatStatus(chat.status)}`}
          >
            <span
              className={`tab-state ${chat.status}`}
              role="img"
              aria-label={chatStatus(chat.status)}
            />
            <span className="tab-copy">
              <strong>{chat.title || "New conversation"}</strong>
              <small>
                {toolLabel(chat.tool)} ·{" "}
                {chat.project.split("/").filter(Boolean).at(-1)}
                {chat.review ? " · Review" : ""}
                {busyChat(chat.status) || chat.status === "error"
                  ? ` · ${chatStatus(chat.status)}`
                  : ""}
              </small>
            </span>
          </button>
          <button
            className="tab-close"
            aria-label={`Close ${chat.title || "New conversation"}`}
            disabled={busyChat(chat.status) || !!closing}
            title={
              busyChat(chat.status)
                ? "Stop the response before closing"
                : "Close tab; keep saved work"
            }
            onClick={() => onClose(chat.id)}
          >
            ×
          </button>
        </div>
      ))}
    </nav>
  );
}
