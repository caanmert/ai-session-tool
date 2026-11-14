import { useEffect, useRef, useState } from "react";
import {
  api,
  errorText,
  toolLabel,
  type ChatState,
  type ChatSummary,
} from "./api";

type Props = {
  project: string;
  onClose: () => void;
  onOpen: (chat: ChatState) => void;
};
const count = (n: number, noun: string) => `${n} ${noun}${n === 1 ? "" : "s"}`;

export default function SavedWorkspace({ project, onClose, onOpen }: Props) {
  const [items, setItems] = useState<ChatSummary[]>([]);
  const [scope, setScope] = useState(project);
  const [kind, setKind] = useState("all");
  const [query, setQuery] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [opening, setOpening] = useState("");
  const [revision, setRevision] = useState(0);
  const dialog = useRef<HTMLDialogElement>(null);
  const search = useRef<HTMLInputElement>(null);
  const operation = useRef(false);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    const previous = document.activeElement as HTMLElement | null;
    dialog.current?.showModal();
    search.current?.focus();
    return () => {
      mounted.current = false;
      dialog.current?.close();
      previous?.focus();
    };
  }, []);
  useEffect(() => {
    let active = true;
    setLoading(true);
    setError("");
    api
      .ListSavedChats()
      .then((result) => {
        if (active) setItems(result);
      })
      .catch((e) => {
        if (active) setError(errorText(e));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [revision]);

  async function reopen(id: string) {
    if (operation.current) return;
    operation.current = true;
    setOpening(id);
    setError("");
    try {
      const chat = await api.ReopenChat(id);
      if (mounted.current) {
        onOpen(chat);
        onClose();
      }
    } catch (e) {
      if (mounted.current) setError(errorText(e));
    } finally {
      operation.current = false;
      if (mounted.current) setOpening("");
    }
  }
  const projects = [
    ...new Set([
      ...items.map((item) => item.project),
      ...(project ? [project] : []),
    ]),
  ].sort();
  const terms = query.toLocaleLowerCase().trim().split(/\s+/).filter(Boolean);
  const visible = items.filter(
    (item) =>
      (!scope || item.project === scope) &&
      (kind === "all" ||
        (kind === "reviews" ? item.isReview : !item.isReview)) &&
      terms.every((term) =>
        `${item.title} ${item.project} ${toolLabel(item.tool)}`
          .toLocaleLowerCase()
          .includes(term),
      ),
  );
  return (
    <dialog
      ref={dialog}
      className="saved-workspace"
      aria-labelledby="saved-workspace-title"
      onCancel={(e) => {
        if (operation.current) e.preventDefault();
        else onClose();
      }}
    >
      <header className="saved-header">
        <div>
          <div className="eyebrow">SAVED ON THIS MAC</div>
          <h2 id="saved-workspace-title">Return to your work</h2>
          <p>
            Conversations and independent reviews, including ones you closed.
          </p>
        </div>
        <button
          className="icon-button"
          aria-label="Close saved workspace"
          disabled={!!opening}
          onClick={onClose}
        >
          ×
        </button>
      </header>
      <div className="saved-filters">
        <input
          ref={search}
          type="search"
          aria-label="Search saved work"
          placeholder="Search titles, projects, or agents…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <select
          aria-label="Saved work project"
          value={scope}
          onChange={(e) => setScope(e.target.value)}
        >
          <option value="">All projects</option>
          {projects.map((path) => (
            <option key={path} value={path}>
              {path}
            </option>
          ))}
        </select>
        <select
          aria-label="Saved work type"
          value={kind}
          onChange={(e) => setKind(e.target.value)}
        >
          <option value="all">All saved work</option>
          <option value="reviews">Reviews</option>
          <option value="conversations">Conversations</option>
        </select>
      </div>
      {error && (
        <div className="notice error" role="alert">
          <span>{error}</span>
          <button
            className="text-button"
            disabled={loading || !!opening}
            onClick={() => setRevision((n) => n + 1)}
          >
            Retry
          </button>
        </div>
      )}
      <div className="saved-list" aria-busy={loading}>
        {loading ? (
          <p className="saved-empty" role="status">
            Loading saved work…
          </p>
        ) : visible.length ? (
          visible.map((item) => (
            <article key={item.id} className="saved-card">
              <div className="saved-card-heading">
                <span className={`saved-kind ${item.isReview ? "review" : ""}`}>
                  {item.isReview ? "Review" : "Conversation"}
                </span>
                <span>{toolLabel(item.tool)}</span>
                <time dateTime={item.updatedAt}>
                  {new Date(item.updatedAt).toLocaleString([], {
                    dateStyle: "medium",
                    timeStyle: "short",
                  })}
                </time>
              </div>
              <h3>{item.title || "Untitled conversation"}</h3>
              <p className="saved-project" title={item.project}>
                {item.project}
              </p>
              <div className="saved-card-footer">
                <span>
                  {item.isReview
                    ? `${item.complete ? `${count(item.findingCount, "finding")} · ${item.sentCount} sent` : item.status === "error" ? "Review needs attention" : item.status === "stopped" ? "Review interrupted" : "Review in progress"} · ${count(item.fileCount, "file")}`
                    : item.status === "waiting"
                      ? "Needs your input"
                      : item.status === "running" || item.status === "stopping"
                        ? "Response in progress"
                        : item.closed
                          ? "Closed tab"
                          : "Open tab"}
                </span>
                <button
                  className="button subtle"
                  disabled={!!opening}
                  aria-label={`${item.closed ? "Reopen" : "View"} ${item.title || "Untitled conversation"}`}
                  onClick={() => void reopen(item.id)}
                >
                  {opening === item.id
                    ? "Opening…"
                    : item.closed
                      ? "Reopen"
                      : "View"}{" "}
                  ↗
                </button>
              </div>
            </article>
          ))
        ) : (
          <div className="saved-empty">
            <h3>
              {items.length
                ? "No matching saved work"
                : "Your saved work will be here"}
            </h3>
            <p>
              {items.length
                ? "Try another project, type, or search."
                : "Conversations and reviews stay here when you close their tabs."}
            </p>
            {!!items.length && (
              <button
                className="text-button"
                onClick={() => {
                  setScope("");
                  setKind("all");
                  setQuery("");
                }}
              >
                Clear saved-work filters
              </button>
            )}
          </div>
        )}
      </div>
      <footer className="saved-footer">
        <span>
          {loading
            ? "Reading local records…"
            : `${visible.length} saved ${visible.length === 1 ? "item" : "items"}`}
        </span>
        <span>Opening saved work does not start an agent.</span>
      </footer>
    </dialog>
  );
}
