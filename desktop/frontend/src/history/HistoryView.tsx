import type { RefObject } from "react";
import AgentMark from "../AgentMark";
import Icon from "../Icon";
import {
  toolLabel,
  type AnnotationChange,
  type Preview,
  type Session,
  type Snapshot,
} from "../api";
import AgentsMissing from "../shell/AgentsMissing";
import { age, basename, sessionCost, sessionTokens } from "../ui/format";
import { SessionTags, SessionTitle } from "./SessionAnnotations";
import MessageList from "../chat/MessageList";
import { previewMessages } from "./previewMessages";

/**
 * Everything the History view shows and can do. The state lives in App,
 * because New task, the command palette and Continue use the same selection
 * and filters.
 */
export type HistoryProps = {
  /** True while a conversation or another page is in front. */
  hidden: boolean;
  snapshot?: Snapshot;
  project: string;
  loading: boolean;
  busy: boolean;
  /** Sessions in the current project and agent filter, before status filters. */
  scopedSessions: Session[];
  /** Sessions after every filter and sort, as listed. */
  sessions: Session[];
  selected?: Session;
  setSelectedKey: (key: string) => void;
  preview?: Preview;
  previewError: string;
  canResume: boolean;
  query: string;
  setQuery: (query: string) => void;
  searching: boolean;
  tool: string;
  setTool: (tool: string) => void;
  historyView: string;
  setHistoryView: (view: string) => void;
  historySort: string;
  setHistorySort: (sort: string) => void;
  hasFilters: boolean;
  clearFilters: () => void;
  searchInput: RefObject<HTMLInputElement | null>;
  sessionRows: RefObject<HTMLDivElement | null>;
  refresh: () => Promise<void> | void;
  /** Continue the selected conversation (false) or start a new one (true). */
  launch: (fresh: boolean) => Promise<void>;
  annotate: (change: AnnotationChange) => Promise<void>;
  pickProject: () => Promise<void>;
  openNew: () => void;
};

/** Filter field, agent filter, refresh and the status filters above the list. */
export function HistoryFilters(props: HistoryProps) {
  const {
    sessions,
    loading,
    hasFilters,
    historyView,
    busy,
    setQuery,
    sessionRows,
    scopedSessions,
    hidden,
    tool,
    searchInput,
    refresh,
    query,
    clearFilters,
    setTool,
    setHistoryView,
  } = props;
  return (
    <>
      <div className="toolbar" hidden={hidden}>
        <label className="history-filter">
          <Icon name="filter" size={15} />
          <input
            ref={searchInput}
            aria-label="Search sessions"
            placeholder="Filter by title, project or tag…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") {
                e.preventDefault();
                setQuery("");
              }
              if (e.key === "ArrowDown" && sessions.length) {
                e.preventDefault();
                sessionRows.current
                  ?.querySelector<HTMLButtonElement>('[tabindex="0"]')
                  ?.focus();
              }
            }}
          />
          {query && (
            <button
              className="icon-button"
              aria-label="Clear search"
              onClick={() => {
                setQuery("");
                searchInput.current?.focus();
              }}
            >
              <Icon name="close" size={14} />
            </button>
          )}
        </label>
        <div className="tool-filter" aria-label="Filter by agent">
          {["all", "codex", "claude"].map((value) => (
            <button
              key={value}
              aria-pressed={tool === value}
              className={tool === value ? "selected" : ""}
              onClick={() => setTool(value)}
            >
              {value === "all"
                ? "All agents"
                : value === "claude"
                  ? "Claude"
                  : "Codex"}
            </button>
          ))}
        </div>
        <button
          className={`icon-button refresh ${loading ? "spinning" : ""}`}
          aria-label="Refresh sessions"
          onClick={() => void refresh()}
          disabled={loading || busy}
        >
          <Icon name="refresh" />
        </button>
      </div>
      <div className="history-controls" hidden={hidden}>
        <div
          className="history-views"
          role="group"
          aria-label="Filter by session status"
        >
          {[
            {
              id: "all",
              label: "All conversations",
              count: scopedSessions.length,
            },
            {
              id: "pinned",
              label: "Pinned",
              count: scopedSessions.filter((s) => s.pinned).length,
            },
            {
              id: "live",
              label: "Live",
              count: scopedSessions.filter((s) => s.live).length,
            },
          ].map((view) => (
            <button
              key={view.id}
              aria-pressed={historyView === view.id}
              onClick={() => setHistoryView(view.id)}
            >
              {view.id === "pinned" && <Icon name="pin" size={13} />}
              {view.id === "live" && <span className="status-dot online" />}
              {view.label}
              <span className="filter-count">{view.count}</span>
            </button>
          ))}
        </div>
        {hasFilters && (
          <button className="history-reset" onClick={clearFilters}>
            Clear filters
          </button>
        )}
      </div>
    </>
  );
}

/** The conversation list and the preview of the selected conversation. */
export function HistoryBrowser(props: HistoryProps) {
  const {
    selected,
    sessions,
    loading,
    hasFilters,
    historyView,
    preview,
    busy,
    snapshot,
    sessionRows,
    hidden,
    canResume,
    setSelectedKey,
    searching,
    refresh,
    previewError,
    launch,
    clearFilters,
    annotate,
    setHistorySort,
    project,
    pickProject,
    openNew,
    historySort,
  } = props;
  return (
    <div className="session-workspace" hidden={hidden}>
      <section className="session-list" aria-label="Conversations">
        <div className="list-heading">
          <span>
            CONVERSATIONS <b>{sessions.length}</b>
          </span>
          <label className="history-sort">
            <span className="sr-only">Sort conversations</span>
            <select
              value={historySort}
              onChange={(e) => setHistorySort(e.target.value)}
            >
              <option value="recent">Pinned · Recent</option>
              <option value="oldest">Oldest first</option>
              <option value="title">Title A–Z</option>
            </select>
          </label>
        </div>
        <div className="history-list-status" role="status">
          {loading
            ? "Refreshing conversations…"
            : searching
              ? "Searching conversations…"
              : `${sessions.length} ${sessions.length === 1 ? "conversation" : "conversations"}${hasFilters ? " matching your filters" : project ? " in this project" : " across your projects"}`}
        </div>
        <div
          className="session-rows"
          ref={sessionRows}
          aria-busy={loading || searching}
          onKeyDown={(e) => {
            if (
              !(e.target instanceof HTMLButtonElement) ||
              !e.target.classList.contains("session-row") ||
              e.altKey ||
              e.ctrlKey ||
              e.metaKey
            )
              return;
            const index = sessions.findIndex((s) => s.key === selected?.key);
            let next = index;
            if (e.key === "ArrowDown")
              next = Math.min(index + 1, sessions.length - 1);
            else if (e.key === "ArrowUp") next = Math.max(0, index - 1);
            else if (e.key === "Home") next = 0;
            else if (e.key === "End") next = sessions.length - 1;
            else if (e.key === "Enter") {
              e.preventDefault();
              if (canResume) void launch(false);
              return;
            } else return;
            e.preventDefault();
            if (sessions[next]) {
              setSelectedKey(sessions[next].key);
              const rows =
                sessionRows.current?.querySelectorAll<HTMLButtonElement>(
                  ".session-row",
                );
              rows?.[next]?.focus();
            }
          }}
        >
          {sessions.map((s) => (
            <button
              className={`session-row ${selected?.key === s.key ? "selected" : ""}`}
              key={s.key}
              onClick={() => setSelectedKey(s.key)}
              tabIndex={selected?.key === s.key ? 0 : -1}
              aria-current={selected?.key === s.key ? "true" : undefined}
            >
              <div className="row-top">
                <AgentMark tool={s.tool} small />
                <span>{toolLabel(s.tool)}</span>
                <time title={new Date(s.updatedAt).toLocaleString()}>
                  {age(s.updatedAt)}
                </time>
              </div>
              <strong className="session-title">
                {s.pinned && <Icon name="pin" size={13} />}{" "}
                {s.title || "Untitled conversation"}
              </strong>
              <div className="row-project">
                <Icon name="folder" size={13} />
                {basename(s.project || s.cwd) || "Unknown project"}
                {s.gitBranch && (
                  <span className="branch-label" title={s.gitBranch}>
                    {s.gitBranch}
                  </span>
                )}
              </div>
              <div className="row-bottom">
                <span>
                  {s.userTurns + s.assistantTurns} messages <span>·</span>{" "}
                  {sessionTokens(s)} tokens
                </span>
                {s.live && (
                  <span className="live-label">
                    <i />
                    Live
                  </span>
                )}
              </div>
            </button>
          ))}
        </div>
        {!sessions.length && (
          <div className="list-empty">
            <Icon name="message" size={30} />
            <h3>
              {loading
                ? "Finding your conversations…"
                : hasFilters
                  ? historyView === "pinned"
                    ? "No pinned conversations"
                    : historyView === "live"
                      ? "No live conversations"
                      : "No matching conversations"
                  : "A fresh start"}
            </h3>
            <p>
              {loading
                ? "Reading your local session history."
                : hasFilters
                  ? historyView === "pinned"
                    ? "Pin conversations with ais pin in the CLI, or try another filter."
                    : historyView === "live"
                      ? "Running agent sessions appear here after refreshing."
                      : "Try a different search or clear your filters."
                  : "Start a conversation in this project and it will appear here."}
            </p>
            {!loading && (
              <button
                className="text-button"
                onClick={hasFilters ? clearFilters : openNew}
              >
                {hasFilters ? "Show all conversations" : "Start a session"}{" "}
                <Icon name="arrow" size={15} />
              </button>
            )}
          </div>
        )}
      </section>
      <section className="conversation" aria-label="Conversation preview">
        {selected ? (
          <>
            <div className="conversation-header">
              <div className="conversation-kicker">
                <AgentMark tool={selected.tool} small />
                <span>{toolLabel(selected.tool)}</span>
                {selected.gitBranch && (
                  <span className="conversation-branch">
                    <Icon name="branch" size={13} />
                    {selected.gitBranch}
                  </span>
                )}
              </div>
              <SessionTitle session={selected} onChange={annotate} />
              <div className="conversation-actions">
                <span className="conversation-path" title={selected.cwd}>
                  <Icon name="folder" size={14} />
                  {selected.cwd}
                </span>
                <button
                  className="button accent"
                  disabled={busy || !canResume}
                  title={
                    canResume
                      ? "Continue this conversation in the app"
                      : "The agent or project folder is unavailable"
                  }
                  onClick={() => void launch(false)}
                >
                  <Icon name="terminal" size={16} />
                  {busy ? "Opening…" : "Continue in app"}
                  <Icon name="arrow" size={15} />
                </button>
              </div>
              <div className="session-facts">
                <span>{selected.userTurns} prompts</span>
                <span>{selected.assistantTurns} replies</span>
                <span>{sessionTokens(selected)} tokens</span>
                {sessionCost(selected) && (
                  <span title="What this usage would cost at API list prices. A subscription is not billed this way.">
                    {sessionCost(selected)} at API prices
                  </span>
                )}
                {selected.model && <span>{selected.model}</span>}
                <SessionTags session={selected} onChange={annotate} />
              </div>
            </div>
            <div className="messages" key={selected.key}>
              {previewError ? (
                <div className="empty-panel" role="alert">
                  <h3>Preview unavailable</h3>
                  <p>{previewError}</p>
                </div>
              ) : !preview ? (
                <div className="loading-preview" role="status">
                  <span className="loader" />
                  Loading conversation…
                </div>
              ) : (
                <>
                  {preview.truncated && (
                    <p className="preview-note">
                      Showing a shortened preview of the latest messages. Open
                      the agent for the full conversation.
                    </p>
                  )}
                  {!preview.messages.length && (
                    <p className="preview-note">
                      This session has no previewable messages yet.
                    </p>
                  )}
                  {/* Drawn like an open conversation, from the saved transcript. */}
                  <MessageList
                    tool={selected.tool}
                    project={selected.cwd || selected.project}
                    messages={previewMessages(preview.messages)}
                    imageNote="[Image omitted from preview]"
                  />
                </>
              )}
            </div>
            <div className="conversation-footer">
              <span className="status-dot online" />
              Saved on your machine{" "}
              <span>Continue to interact live in the app</span>
            </div>
          </>
        ) : snapshot && !snapshot.tools.some((t) => t.available) ? (
          <AgentsMissing
            tools={snapshot.tools}
            checking={loading}
            onCheck={() => void refresh()}
          />
        ) : (
          <div className="empty-panel welcome">
            <div className="welcome-mark">
              <Icon name="terminal" size={32} />
            </div>
            <div className="eyebrow">A LITTLE LESS CONTEXT SWITCHING</div>
            <h2>
              Your projects.
              <br />
              Your conversations.
            </h2>
            <p>
              Select a conversation to see where you left off,
              <br />
              or open a project and start something new.
            </p>
            <button
              className="button primary"
              onClick={() => void pickProject()}
              disabled={busy}
            >
              <Icon name="folder" />
              Choose a project
            </button>
          </div>
        )}
      </section>
    </div>
  );
}
