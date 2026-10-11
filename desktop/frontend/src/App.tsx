import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import Markdown from "react-markdown";
import ChatWorkspace, { chatBusy } from "./ChatWorkspace";
import {
  api,
  errorText,
  toolLabel,
  type Preview,
  type Session,
  type Snapshot,
  type ToolName,
  type ChatState,
} from "./api";

function Icon({ name, size = 18 }: { name: string; size?: number }) {
  const paths: Record<string, ReactNode> = {
    folder: (
      <>
        <path d="M3 7V5a1 1 0 0 1 1-1h5l2 3h9a1 1 0 0 1 1 1v11a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V7Z" />
      </>
    ),
    search: (
      <>
        <circle cx="10.5" cy="10.5" r="6.5" />
        <path d="m16 16 5 5" />
      </>
    ),
    plus: <path d="M12 5v14M5 12h14" />,
    grid: (
      <>
        <rect x="3" y="3" width="7" height="7" rx="1" />
        <rect x="14" y="3" width="7" height="7" rx="1" />
        <rect x="3" y="14" width="7" height="7" rx="1" />
        <rect x="14" y="14" width="7" height="7" rx="1" />
      </>
    ),
    terminal: (
      <>
        <rect x="3" y="4" width="18" height="16" rx="3" />
        <path d="m7 9 3 3-3 3m6 0h4" />
      </>
    ),
    refresh: (
      <>
        <path d="M20 7v5h-5M4 17v-5h5" />
        <path d="M6 7a7 7 0 0 1 12-1l2 3M4 15l2 3a7 7 0 0 0 12-1" />
      </>
    ),
    branch: (
      <>
        <circle cx="6" cy="5" r="2" />
        <circle cx="6" cy="19" r="2" />
        <circle cx="18" cy="5" r="2" />
        <path d="M6 7v10m12-10v2a6 6 0 0 1-6 6H6" />
      </>
    ),
    arrow: <path d="M5 12h14m-5-5 5 5-5 5" />,
    close: <path d="m6 6 12 12M6 18 18 6" />,
    message: <path d="M21 11a8 8 0 0 1-8 8H5l-3 3V11a9 9 0 0 1 19 0Z" />,
    check: <path d="m5 12 4 4L19 6" />,
    pin: <path d="m9 3 6 0-1 6 4 4v2H6v-2l4-4-1-6Zm3 12v6" />,
  };
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {paths[name] || paths.message}
    </svg>
  );
}

function AgentMark({ tool, small = false }: { tool: string; small?: boolean }) {
  return (
    <span
      className={`agent-mark ${tool} ${small ? "small" : ""}`}
      aria-hidden="true"
    >
      {tool === "claude" ? (
        "✳"
      ) : (
        <Icon name="terminal" size={small ? 14 : 19} />
      )}
    </span>
  );
}

function age(date: string) {
  const minutes = Math.max(
    0,
    Math.floor((Date.now() - Date.parse(date)) / 60000),
  );
  if (!Number.isFinite(minutes)) return "";
  if (minutes < 1) return "Just now";
  if (minutes < 60) return `${minutes}m ago`;
  if (minutes < 1440) return `${Math.floor(minutes / 60)}h ago`;
  return `${Math.floor(minutes / 1440)}d ago`;
}
function tokens(session: Session) {
  const u = session.usage;
  return new Intl.NumberFormat("en", {
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(u.input + u.output + u.cacheRead + u.cacheWrite);
}
function basename(path: string) {
  return path.split("/").filter(Boolean).at(-1) || path;
}

export default function App() {
  const [snapshot, setSnapshot] = useState<Snapshot>();
  const [chats, setChats] = useState<ChatState[]>([]);
  const [activeChat, setActiveChat] = useState("");
  const [project, setProject] = useState("");
  const [tool, setTool] = useState("all");
  const [query, setQuery] = useState("");
  const [searchKeys, setSearchKeys] = useState<Set<string> | null>(null);
  const [searching, setSearching] = useState(false);
  const [selectedKey, setSelectedKey] = useState("");
  const [preview, setPreview] = useState<Preview>();
  const [previewError, setPreviewError] = useState("");
  const [error, setError] = useState("");
  const [toast, setToast] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [showNew, setShowNew] = useState(false);
  const [newProject, setNewProject] = useState("");
  const [newTool, setNewTool] = useState<ToolName>("codex");
  const searchInput = useRef<HTMLInputElement>(null);
  const modal = useRef<HTMLDivElement>(null);
  const newButton = useRef<HTMLButtonElement>(null);
  const refreshSeq = useRef(0);
  const searchSeq = useRef(0);
  const operation = useRef(false);

  const refresh = useCallback(async () => {
    const seq = ++refreshSeq.current;
    setLoading(true);
    setError("");
    try {
      const next = await api.Refresh();
      if (seq === refreshSeq.current) setSnapshot(next);
    } catch (e) {
      if (seq === refreshSeq.current) setError(errorText(e));
    } finally {
      if (seq === refreshSeq.current) setLoading(false);
    }
  }, []);
  useEffect(() => {
    void refresh();
    return () => {
      refreshSeq.current++;
    };
  }, [refresh]);
  useEffect(() => {
    let active = true;
    api
      .ListChats()
      .then((items) => {
        if (active)
          setChats((current) => [
            ...current,
            ...items.filter((item) => !current.some((s) => s.id === item.id)),
          ]);
      })
      .catch((e) => {
        if (active) setError(errorText(e));
      });
    return () => {
      active = false;
    };
  }, []);
  const chatChanged = useCallback(
    (next: ChatState) => {
      setChats((items) =>
        items.map((item) => (item.id === next.id ? next : item)),
      );
      if (!chatBusy(next.status)) void refresh();
    },
    [refresh],
  );
  function chatClosed(id: string) {
    setChats((items) => items.filter((item) => item.id !== id));
    setActiveChat((current) => (current === id ? "" : current));
  }
  function openChat(chat: ChatState) {
    setChats((items) =>
      items.some((item) => item.id === chat.id) ? items : [...items, chat],
    );
    setActiveChat(chat.id);
  }
  useEffect(() => {
    const seq = ++searchSeq.current;
    setSearchKeys(null);
    if (!query.trim() || !snapshot) {
      setSearching(false);
      return;
    }
    setSearching(true);
    const timer = setTimeout(() => {
      api
        .Search(query)
        .then((keys) => {
          if (seq === searchSeq.current) setSearchKeys(new Set(keys));
        })
        .catch((e) => {
          if (seq === searchSeq.current) {
            setError(errorText(e));
            setSearchKeys(new Set());
          }
        })
        .finally(() => {
          if (seq === searchSeq.current) setSearching(false);
        });
    }, 180);
    return () => {
      clearTimeout(timer);
      searchSeq.current++;
    };
  }, [query, snapshot]);

  const sessions = useMemo(
    () =>
      (snapshot?.sessions || []).filter(
        (s) =>
          (!project || s.project === project) &&
          (tool === "all" || s.tool === tool) &&
          (!searchKeys || searchKeys.has(s.key)),
      ),
    [snapshot, project, tool, searchKeys],
  );
  const selected = sessions.find((s) => s.key === selectedKey) || sessions[0];
  useEffect(() => {
    let active = true;
    setPreview(undefined);
    setPreviewError("");
    if (selected)
      api
        .Transcript(selected.key)
        .then((value) => {
          if (active) setPreview(value);
        })
        .catch((e) => {
          if (active) setPreviewError(errorText(e));
        });
    return () => {
      active = false;
    };
  }, [selected?.key, selected?.updatedAt]);
  const currentProject = snapshot?.projects.find((p) => p.path === project);

  const openNew = useCallback(() => {
    setNewProject(
      project ||
        selected?.project ||
        snapshot?.projects.find((p) => p.available)?.path ||
        "",
    );
    setNewTool(
      snapshot?.tools.find((t) => t.available && t.name === selected?.tool)
        ?.name ||
        snapshot?.tools.find((t) => t.available)?.name ||
        "codex",
    );
    setError("");
    setShowNew(true);
  }, [project, selected, snapshot]);
  useEffect(() => {
    function key(event: KeyboardEvent) {
      if (
        (event.metaKey || event.ctrlKey) &&
        event.key.toLowerCase() === "k" &&
        !showNew
      ) {
        event.preventDefault();
        setActiveChat("");
        searchInput.current?.focus();
      }
      if (
        (event.metaKey || event.ctrlKey) &&
        event.key.toLowerCase() === "n" &&
        !showNew &&
        snapshot
      ) {
        event.preventDefault();
        openNew();
      }
      if (event.key === "Escape" && showNew && !busy) {
        setShowNew(false);
        newButton.current?.focus();
      }
      if (event.key === "Tab" && showNew) {
        const items = modal.current?.querySelectorAll<HTMLElement>(
          'button:not(:disabled), select, input:not(:disabled), [tabindex="0"]',
        );
        if (!items?.length) return;
        const first = items[0],
          last = items[items.length - 1];
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last.focus();
        }
        if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first.focus();
        }
      }
    }
    document.addEventListener("keydown", key);
    return () => document.removeEventListener("keydown", key);
  }, [showNew, busy, openNew, snapshot]);
  useEffect(() => {
    if (showNew) modal.current?.querySelector<HTMLElement>("button")?.focus();
  }, [showNew]);

  async function pickProject() {
    if (operation.current) return;
    operation.current = true;
    setBusy(true);
    setError("");
    try {
      const path = await api.PickProject();
      if (path) {
        setProject(path);
        setNewProject(path);
        setQuery("");
        await refresh();
      }
    } catch (e) {
      setError(errorText(e));
    } finally {
      operation.current = false;
      setBusy(false);
    }
  }
  async function launch(fresh: boolean) {
    if (operation.current) return;
    if (!fresh && !selected) return;
    operation.current = true;
    setBusy(true);
    setError("");
    try {
      const live = fresh
        ? await api.NewChat(newProject, newTool)
        : await api.ResumeChat(selected!.key);
      setChats((items) =>
        items.some((item) => item.id === live.id) ? items : [...items, live],
      );
      setActiveChat(live.id);
      setShowNew(false);
      setToast("");
    } catch (e) {
      setError(errorText(e));
    } finally {
      operation.current = false;
      setBusy(false);
    }
  }
  const canResume =
    !!selected &&
    !!snapshot?.tools.find((t) => t.name === selected.tool && t.available) &&
    !!snapshot?.projects.find(
      (p) => p.path === selected.project && p.available,
    );
  const canStart =
    !!newProject &&
    !!snapshot?.projects.find((p) => p.path === newProject && p.available) &&
    !!snapshot?.tools.find((t) => t.name === newTool && t.available);

  return (
    <div className="app-shell">
      <aside className="sidebar" aria-label="Projects">
        <div className="brand">
          <span className="brand-symbol">
            a<span>✳</span>
          </span>
          <div>
            <strong>ais</strong>
            <span>YOUR CODING WORKSPACE</span>
          </div>
        </div>
        <div className="sidebar-main">
          <button
            className={`nav-all ${!project ? "active" : ""}`}
            onClick={() => {
              setActiveChat("");
              setProject("");
              setQuery("");
            }}
          >
            <Icon name="grid" />
            <span>All sessions</span>
            <b>{snapshot?.sessions.length ?? "—"}</b>
          </button>
          <div className="section-heading">
            <span>PROJECTS</span>
            <button
              aria-label="Add project"
              className="icon-button"
              onClick={() => void pickProject()}
              disabled={busy}
            >
              <Icon name="plus" size={16} />
            </button>
          </div>
          <div className="project-list">
            {snapshot?.projects.map((p) => (
              <button
                key={p.path}
                className={`project-item ${project === p.path ? "active" : ""}`}
                title={p.path}
                onClick={() => {
                  setActiveChat("");
                  setProject(p.path);
                  setQuery("");
                }}
              >
                <Icon name="folder" />
                <span>
                  <strong>{p.name || p.path}</strong>
                  <small>
                    {p.available
                      ? `${p.sessions} ${p.sessions === 1 ? "session" : "sessions"}`
                      : "Folder unavailable"}
                  </small>
                </span>
                {p.recent && (
                  <i className="recent-dot" title="Recently added" />
                )}
              </button>
            ))}
          </div>
          {snapshot && !snapshot.projects.length && (
            <p className="sidebar-hint">
              Add a project folder to start your first conversation.
            </p>
          )}
          <button
            className="add-project"
            onClick={() => void pickProject()}
            disabled={busy}
          >
            <Icon name="plus" size={16} /> Add project folder
          </button>
        </div>
        <div className="sidebar-bottom">
          <div className="section-heading">YOUR AGENTS</div>
          {snapshot?.tools.map((t) => (
            <div className="agent-status" key={t.name}>
              <AgentMark tool={t.name} small />
              <span>{toolLabel(t.name)}</span>
              <span
                className={`status-dot ${t.available ? "online" : ""}`}
                title={t.available ? t.path : "Not installed"}
              />
              <small>{t.available ? "Ready" : "Missing"}</small>
            </div>
          ))}
          <div className="local-note">
            <span className="status-dot online" />
            Local sessions. Your machine.
          </div>
        </div>
      </aside>
      <main className={`workspace ${activeChat ? "has-chat" : ""}`}>
        <header className="workspace-header">
          <div>
            <div className="eyebrow">
              WORKSPACE <span>/</span> {project ? "PROJECT" : "ALL PROJECTS"}
            </div>
            <h1>
              {activeChat
                ? "Your coding workspace"
                : project
                  ? currentProject?.name || basename(project)
                  : "Your next step starts here."}
            </h1>
            <p title={project}>
              {project ||
                "Every conversation. Every project. One place to pick up where you left off."}
            </p>
          </div>
          <button
            ref={newButton}
            className="button primary"
            onClick={openNew}
            disabled={!snapshot || busy}
          >
            <Icon name="plus" size={17} /> New session
          </button>
        </header>
        <div className="toolbar" hidden={!!activeChat}>
          <label className="search">
            <Icon name="search" />
            <input
              ref={searchInput}
              aria-label="Search sessions"
              placeholder="Search conversations, projects, or tags…"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
            <kbd>⌘ K</kbd>
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
        {error && !showNew && (
          <div className="notice error" role="alert">
            <span>{error}</span>
            <button
              className="icon-button"
              aria-label="Dismiss error"
              onClick={() => setError("")}
            >
              <Icon name="close" size={16} />
            </button>
          </div>
        )}
        {toast && (
          <div className="notice success" role="status">
            <Icon name="check" />
            <span>{toast}</span>
            <button
              className="icon-button"
              aria-label="Dismiss notification"
              onClick={() => setToast("")}
            >
              <Icon name="close" size={16} />
            </button>
          </div>
        )}
        {!!snapshot?.warnings.length && (
          <details className="warnings">
            <summary>
              {snapshot.warnings.length}{" "}
              {snapshot.warnings.length === 1 ? "notice" : "notices"} about your
              session data
            </summary>
            {snapshot.warnings.map((w, i) => (
              <p key={i}>{w}</p>
            ))}
          </details>
        )}
        {chats.length > 0 && (
          <nav className="live-tabs" aria-label="Workspace tabs">
            <button
              className={!activeChat ? "active" : ""}
              aria-pressed={!activeChat}
              onClick={() => setActiveChat("")}
            >
              History
            </button>
            {chats.map((live, i) => (
              <button
                key={live.id}
                className={activeChat === live.id ? "active" : ""}
                aria-pressed={activeChat === live.id}
                onClick={() => setActiveChat(live.id)}
                title={live.project}
              >
                <span
                  className={`status-dot ${chatBusy(live.status) ? "online" : ""}`}
                />
                {toolLabel(live.tool)} · {basename(live.project)}{" "}
                <small>
                  #{i + 1}
                  {live.review ? " · Review" : ""}
                </small>
              </button>
            ))}
          </nav>
        )}
        {chats.map((live) => (
          <ChatWorkspace
            key={live.id}
            chat={live}
            active={activeChat === live.id}
            reviewerAvailable={
              !!snapshot?.tools.some((t) => t.name !== live.tool && t.available)
            }
            onOpen={openChat}
            onSelect={(id) => {
              void api
                .ReadChat(id, -1)
                .then((update) => {
                  if (update.state) openChat(update.state);
                })
                .catch((e) => setError(errorText(e)));
            }}
            onState={chatChanged}
            onClose={chatClosed}
          />
        ))}
        <div className="session-workspace" hidden={!!activeChat}>
          <section className="session-list" aria-label="Conversations">
            <div className="list-heading">
              <span>
                CONVERSATIONS <b>{sessions.length}</b>
              </span>
              <small>
                {loading
                  ? "Refreshing…"
                  : searching
                    ? "Searching…"
                    : "Pinned · Recent"}
              </small>
            </div>
            <div className="session-rows" aria-busy={loading || searching}>
              {sessions.map((s) => (
                <button
                  className={`session-row ${selected?.key === s.key ? "selected" : ""}`}
                  key={s.key}
                  onClick={() => setSelectedKey(s.key)}
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
                      {tokens(s)} tokens
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
                    : query || tool !== "all"
                      ? "No matches yet"
                      : "A fresh start"}
                </h3>
                <p>
                  {loading
                    ? "Reading your local session history."
                    : query || tool !== "all"
                      ? "Try another search or agent filter."
                      : "Start a conversation in this project and it will appear here."}
                </p>
                {!loading && (
                  <button
                    className="text-button"
                    onClick={
                      query || tool !== "all"
                        ? () => {
                            setQuery("");
                            setTool("all");
                          }
                        : openNew
                    }
                  >
                    {query || tool !== "all"
                      ? "Clear filters"
                      : "Start a session"}{" "}
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
                  <h2>{selected.title || "Untitled conversation"}</h2>
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
                    <span>{tokens(selected)} tokens</span>
                    {selected.model && <span>{selected.model}</span>}
                    {selected.tags?.length ? (
                      <div className="tags">
                        {selected.tags.map((tag) => (
                          <span key={tag}>#{tag}</span>
                        ))}
                      </div>
                    ) : null}
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
                          Showing a shortened preview of the latest messages.
                          Open the agent for the full conversation.
                        </p>
                      )}
                      {!preview.messages.length && (
                        <p className="preview-note">
                          This session has no previewable messages yet.
                        </p>
                      )}
                      {preview.messages.map((message, i) =>
                        message.kind === "text" ? (
                          <article
                            key={i}
                            className={`message ${message.role}`}
                          >
                            <div className="message-heading">
                              <span className={`avatar ${message.role}`}>
                                {message.role === "user" ? (
                                  "Y"
                                ) : (
                                  <AgentMark tool={selected.tool} small />
                                )}
                              </span>
                              <strong>
                                {message.role === "user"
                                  ? "You"
                                  : toolLabel(selected.tool)}
                              </strong>
                              {message.time && (
                                <time>
                                  {new Date(message.time).toLocaleTimeString(
                                    [],
                                    { hour: "2-digit", minute: "2-digit" },
                                  )}
                                </time>
                              )}
                            </div>
                            <div className="markdown">
                              <Markdown
                                components={{
                                  a: ({ children }) => (
                                    <span className="transcript-link">
                                      {children}
                                    </span>
                                  ),
                                  img: () => (
                                    <span className="preview-note">
                                      [Image omitted from preview]
                                    </span>
                                  ),
                                }}
                              >
                                {message.text || ""}
                              </Markdown>
                            </div>
                          </article>
                        ) : (
                          <details className="tool-message" key={i}>
                            <summary>
                              <Icon name="terminal" size={13} />
                              {message.toolName ||
                                (message.kind === "thinking"
                                  ? "Reasoning"
                                  : "Tool output")}
                            </summary>
                            <pre>{message.text}</pre>
                          </details>
                        ),
                      )}
                    </>
                  )}
                </div>
                <div className="conversation-footer">
                  <span className="status-dot online" />
                  Saved on your machine{" "}
                  <span>Continue to interact live in the app</span>
                </div>
              </>
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
      </main>
      {showNew && (
        <div
          className="modal-overlay"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget && !busy) {
              setShowNew(false);
              newButton.current?.focus();
            }
          }}
        >
          <div
            className="modal"
            ref={modal}
            role="dialog"
            aria-modal="true"
            aria-labelledby="new-session-title"
          >
            <button
              className="icon-button modal-close"
              aria-label="Close new session"
              disabled={busy}
              onClick={() => {
                setShowNew(false);
                newButton.current?.focus();
              }}
            >
              <Icon name="close" />
            </button>
            <div className="modal-icon">
              <Icon name="terminal" size={24} />
            </div>
            <div className="eyebrow">MAKE YOUR NEXT MOVE</div>
            <h2 id="new-session-title">Start a new session</h2>
            <p>A fresh conversation, in the project you choose.</p>
            <label className="field-label" htmlFor="new-project">
              Project
            </label>
            <div className="project-select">
              <select
                id="new-project"
                value={newProject}
                onChange={(e) => setNewProject(e.target.value)}
              >
                <option value="">Choose a project…</option>
                {snapshot?.projects.map((p) => (
                  <option key={p.path} value={p.path} disabled={!p.available}>
                    {p.name} — {p.path}
                  </option>
                ))}
              </select>
              <button
                className="button subtle"
                onClick={() => void pickProject()}
                disabled={busy}
              >
                Browse…
              </button>
            </div>
            <span className="field-label">Agent</span>
            <div className="agent-options">
              {(["codex", "claude"] as ToolName[]).map((name) => {
                const available = snapshot?.tools.some(
                  (t) => t.name === name && t.available,
                );
                return (
                  <button
                    key={name}
                    className={`agent-option ${newTool === name ? "selected" : ""}`}
                    disabled={!available || busy}
                    aria-pressed={newTool === name}
                    onClick={() => setNewTool(name)}
                  >
                    <AgentMark tool={name} />
                    <span>
                      <strong>{toolLabel(name)}</strong>
                      <small>
                        {available ? "Ready on this machine" : "Not installed"}
                      </small>
                    </span>
                    <span className="radio-dot" />
                  </button>
                );
              })}
            </div>
            {snapshot?.tools.every((t) => !t.available) && (
              <p className="preview-note">
                Install Codex or Claude Code, then reopen ais. Run{" "}
                <code>ais setup</code> in your terminal for installation
                instructions.
              </p>
            )}
            {error && (
              <div className="notice error" role="alert">
                {error}
              </div>
            )}
            <div className="terminal-note">
              <Icon name="terminal" size={16} />
              <p>
                Start a graphical conversation with live replies, tool activity
                and approval controls. The agent starts when you send a message.
              </p>
            </div>
            <button
              className="button accent start-session"
              disabled={!canStart || busy}
              onClick={() => void launch(true)}
            >
              {busy ? "Starting agent…" : "Start in app"}
              <Icon name="arrow" size={17} />
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
