import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import RunDashboard from "./RunDashboard";
import ChatWorkspace, { chatBusy } from "./ChatWorkspace";
import SavedWorkspace from "./SavedWorkspace";
import { useAppearance } from "./Appearance";
import { storeDraft } from "./useChatDraft";
import { lastAgent, rememberAgent } from "./shell/lastAgent";
import Settings from "./settings/Settings";
import Board from "./board/Board";
import IssuesPage from "./github/IssuesPage";
import UsagePage from "./usage/UsagePage";
import { useUsageLimits } from "./usage/limits";
import { isBoolean, isStringList, useStored } from "./ui/stored";
import { basename } from "./ui/format";
import {
  HistoryBrowser,
  HistoryFilters,
  type HistoryProps,
} from "./history/HistoryView";
import { sidebarOrder } from "./chat/conversation";
import TopBar from "./shell/TopBar";
import Sidebar from "./shell/Sidebar";
import CommandPalette, { type Command } from "./shell/CommandPalette";
import {
  attentionChange,
  attentionNotices,
  notificationsEnabled,
} from "./shell/attention";
import Icon from "./Icon";
import AgentMark from "./AgentMark";
import {
  api,
  errorText,
  toolLabel,
  type Preview,
  type Snapshot,
  type ToolName,
  type ChatState,
  type AnnotationChange,
  type AgentHealth,
} from "./api";

function boardSummary(chats: ChatState[]) {
  if (!chats.length) return "Open conversations appear here, grouped by state.";
  const count = (...statuses: string[]) =>
    chats.filter((chat) => statuses.includes(chat.status)).length;
  return `${count("running", "stopping")} working · ${count("waiting")} need you · ${chats.length} open`;
}

export default function App() {
  const [snapshot, setSnapshot] = useState<Snapshot>();
  const [chats, setChats] = useState<ChatState[]>([]);
  const [activeChat, setActiveChat] = useState("");
  const [project, setProject] = useState("");
  // The page behind any open conversation: History, Parallel runs or Settings.
  const [page, setPage] = useState<
    "history" | "runs" | "settings" | "board" | "github" | "usage"
  >("history");
  // Layout choices, remembered on this Mac.
  const [rail, setRail] = useStored("ais:sidebar-rail:v1", false, isBoolean);
  const [folded, setFolded] = useStored<string[]>(
    "ais:sidebar-folded:v1",
    [],
    isStringList,
  );
  const fold = useCallback(
    (key: string) =>
      setFolded((keys) =>
        keys.includes(key) ? keys.filter((k) => k !== key) : [...keys, key],
      ),
    [setFolded],
  );
  const [detailsHidden, setDetailsHidden] = useStored(
    "ais:details-hidden:v1",
    false,
    isBoolean,
  );
  const usage = useUsageLimits(
    !!snapshot?.tools.some((tool) => tool.available),
  );
  const showRuns = page === "runs";
  const otherPage = page !== "history";
  const setShowRuns = useCallback(
    (on: boolean) => setPage(on ? "runs" : "history"),
    [],
  );
  // The run task to show when the Parallel runs page is opened from the board.
  const [runFocus, setRunFocus] = useState<{
    run: string;
    task: string;
    at: number;
  }>();
  const appearance = useAppearance();
  // Whether each installed agent still accepts what ais sends it. Checked at
  // startup and on request; it never contacts a model.
  const [health, setHealth] = useState<AgentHealth[]>([]);
  const checkHealth = useCallback(() => {
    api
      .AgentHealth()
      .then(setHealth)
      .catch(() => setHealth([]));
  }, []);
  useEffect(() => {
    checkHealth();
  }, [checkHealth]);
  const incompatible = health.filter((item) => item.problem);
  const [tool, setTool] = useState("all");
  const [historyView, setHistoryView] = useState("all");
  const [historySort, setHistorySort] = useState("recent");
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
  const [showLibrary, setShowLibrary] = useState(false);
  const [closingChat, setClosingChat] = useState("");
  const closingOperation = useRef(false);
  const [newProject, setNewProject] = useState("");
  const [newTool, setNewTool] = useState<ToolName>("codex");
  const [newMessage, setNewMessage] = useState("");
  // Give the task to both agents, each in its own worktree, and compare.
  const [newCompare, setNewCompare] = useState(false);
  const searchInput = useRef<HTMLInputElement>(null);
  const [showPalette, setShowPalette] = useState(false);
  const sessionRows = useRef<HTMLDivElement>(null);
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
  // Tell the Dock how many conversations are waiting for the user, and bounce
  // its icon when one starts waiting or a response ends in the background.
  const attention = useRef({ waiting: 0, busy: 0 });
  const seenStatuses = useRef(new Map<string, string>());
  useEffect(() => {
    const { now, update } = attentionChange(attention.current, chats);
    attention.current = now;
    // The Dock is a convenience; the sidebar shows the same state.
    if (update)
      api
        .SetAttention(update.count, update.alert && !document.hasFocus())
        .catch(() => {});
    // Banners only when nobody is looking at the app, and only if wanted.
    const { now: statuses, notices } = attentionNotices(
      seenStatuses.current,
      chats,
    );
    seenStatuses.current = statuses;
    if (!document.hasFocus() && notificationsEnabled())
      for (const notice of notices.slice(0, 3))
        api.Notify(notice.title, notice.body).catch(() => {});
  }, [chats]);
  function chatClosed(id: string) {
    setChats((items) => items.filter((item) => item.id !== id));
    setActiveChat((current) => (current === id ? "" : current));
  }
  async function closeChat(id: string) {
    if (closingOperation.current) return;
    closingOperation.current = true;
    setClosingChat(id);
    try {
      await api.CloseChat(id);
      chatClosed(id);
    } catch (e) {
      setError(errorText(e));
    } finally {
      closingOperation.current = false;
      setClosingChat("");
    }
  }
  function openChat(chat: ChatState) {
    setShowRuns(false);
    setChats((items) =>
      items.some((item) => item.id === chat.id)
        ? items.map((item) => (item.id === chat.id ? chat : item))
        : [...items, chat],
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

  const scopedSessions = useMemo(
    () =>
      (snapshot?.sessions || []).filter(
        (s) =>
          (!project || s.project === project) &&
          (tool === "all" || s.tool === tool) &&
          (!searchKeys || searchKeys.has(s.key)),
      ),
    [snapshot, project, tool, searchKeys],
  );
  const sessions = useMemo(() => {
    const items = scopedSessions.filter(
      (s) =>
        historyView === "all" || (historyView === "pinned" ? s.pinned : s.live),
    );
    return items.sort((a, b) => {
      if (historySort === "title")
        return a.title.localeCompare(b.title) || a.key.localeCompare(b.key);
      if (historySort === "recent" && !!a.pinned !== !!b.pinned)
        return a.pinned ? -1 : 1;
      const order = Date.parse(b.updatedAt) - Date.parse(a.updatedAt);
      return (
        (historySort === "oldest" ? -order : order) ||
        a.key.localeCompare(b.key)
      );
    });
  }, [scopedSessions, historyView, historySort]);
  const selected = sessions.find((s) => s.key === selectedKey) || sessions[0];
  const hasFilters = !!query || tool !== "all" || historyView !== "all";
  function clearFilters() {
    setQuery("");
    setTool("all");
    setHistoryView("all");
  }
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
  const showHistory = useCallback(() => {
    setActiveChat("");
    setPage("history");
  }, []);

  // The agent to preselect for a project: the one last started there, if it is
  // still installed.
  const rememberedAgent = useCallback(
    (path: string) => {
      const tool = lastAgent(path);
      return snapshot?.tools.find((t) => t.available && t.name === tool)?.name;
    },
    [snapshot],
  );
  const openNew = useCallback(() => {
    const target =
      project ||
      selected?.project ||
      snapshot?.projects.find((p) => p.available)?.path ||
      "";
    setNewProject(target);
    setNewTool(
      rememberedAgent(target) ||
        snapshot?.tools.find((t) => t.available && t.name === selected?.tool)
          ?.name ||
        snapshot?.tools.find((t) => t.available)?.name ||
        "codex",
    );
    setError("");
    setShowNew(true);
  }, [project, selected, snapshot, rememberedAgent]);
  // Start a task from outside the dialog, such as from a GitHub issue: New
  // task opens with the text filled in, and nothing is sent until started.
  const startTaskFrom = useCallback(
    (target: string, text: string) => {
      setNewProject(target);
      setNewTool(
        rememberedAgent(target) ||
          snapshot?.tools.find((t) => t.available)?.name ||
          "codex",
      );
      setNewMessage(text);
      setError("");
      setShowNew(true);
    },
    [snapshot, rememberedAgent],
  );
  // Everything the command palette can jump to. Choosing a saved conversation
  // only shows it in History; it never starts or resumes an agent.
  const commands = useMemo<Command[]>(() => {
    const folder = (path: string) => basename(path);
    return [
      {
        id: "new",
        group: "Actions",
        label: "New task",
        icon: "plus",
        shortcut: "⌘N",
        run: openNew,
      },
      {
        id: "history",
        group: "Actions",
        label: "History",
        icon: "message",
        run: showHistory,
      },
      {
        id: "runs",
        group: "Actions",
        label: "Parallel runs",
        icon: "branch",
        run: () => {
          setActiveChat("");
          setShowRuns(true);
        },
      },
      {
        id: "board",
        group: "Actions",
        label: "Agents board",
        icon: "grid",
        run: () => {
          setActiveChat("");
          setPage("board");
        },
      },
      {
        id: "github",
        group: "Actions",
        label: "Issues and pull requests",
        icon: "issue",
        run: () => {
          setActiveChat("");
          setPage("github");
        },
      },
      {
        id: "sidebar",
        group: "Actions",
        label: rail ? "Expand the sidebar" : "Narrow the sidebar",
        icon: "sidebar",
        run: () => setRail((on) => !on),
      },
      {
        id: "usage",
        group: "Actions",
        label: "Usage limits",
        icon: "gauge",
        run: () => {
          setActiveChat("");
          setPage("usage");
        },
      },
      {
        id: "settings",
        group: "Actions",
        label: "Settings",
        icon: "settings",
        run: () => {
          setActiveChat("");
          setPage("settings");
        },
      },
      {
        id: "saved",
        group: "Actions",
        label: "Saved workspace",
        icon: "folder",
        run: () => setShowLibrary(true),
      },
      ...chats.map((chat): Command => ({
        id: `open-${chat.id}`,
        group: "Open conversations",
        label: chat.title || "New conversation",
        hint: `${toolLabel(chat.tool)} · ${folder(chat.project)}`,
        icon: "message",
        run: () => {
          setShowRuns(false);
          setActiveChat(chat.id);
        },
      })),
      ...(snapshot?.projects || []).map((p): Command => ({
        id: `project-${p.path}`,
        group: "Projects",
        label: p.name || p.path,
        hint: p.path,
        icon: "folder",
        run: () => {
          showHistory();
          setProject(p.path);
          setQuery("");
        },
      })),
      ...(snapshot?.sessions || []).map((s): Command => ({
        id: `session-${s.key}`,
        group: "Conversations",
        label: s.title || "Untitled conversation",
        hint: `${toolLabel(s.tool)} · ${folder(s.project)}`,
        icon: "file",
        run: () => {
          showHistory();
          setProject("");
          setQuery("");
          setTool("all");
          setHistoryView("all");
          setSelectedKey(s.key);
        },
      })),
    ];
  }, [chats, snapshot, openNew, showHistory, setShowRuns, rail, setRail]);
  useEffect(() => {
    function key(event: KeyboardEvent) {
      if (showLibrary || document.querySelector("dialog[open]")) return;
      if (
        (event.metaKey || event.ctrlKey) &&
        event.key.toLowerCase() === "k" &&
        !showNew
      ) {
        event.preventDefault();
        setShowPalette(true);
      }
      // Cmd+1..9 switches to the open conversation in that sidebar position.
      if (
        (event.metaKey || event.ctrlKey) &&
        !event.shiftKey &&
        !event.altKey &&
        /^[1-9]$/.test(event.key) &&
        !showNew
      ) {
        const target = sidebarOrder(chats).ordered[Number(event.key) - 1];
        if (target) {
          event.preventDefault();
          setPage("history");
          setActiveChat(target.id);
        }
      }
      if (
        (event.metaKey || event.ctrlKey) &&
        event.key.toLowerCase() === "b" &&
        !event.shiftKey &&
        !event.altKey &&
        !showNew
      ) {
        event.preventDefault();
        setRail((on) => !on);
      }
      if ((event.metaKey || event.ctrlKey) && event.key === "," && !showNew) {
        event.preventDefault();
        setActiveChat("");
        setPage("settings");
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
          'button:not(:disabled), select, textarea:not(:disabled), input:not(:disabled), [tabindex="0"]',
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
  }, [
    showNew,
    showLibrary,
    busy,
    openNew,
    snapshot,
    activeChat,
    showRuns,
    chats,
    setRail,
  ]);
  useEffect(() => {
    if (showNew)
      (
        modal.current?.querySelector<HTMLElement>("textarea") ??
        modal.current?.querySelector<HTMLElement>("button")
      )?.focus();
  }, [showNew]);

  // Rename, pin or tag the selected conversation. These notes belong to ais;
  // the agent's transcript is not touched and no agent is started.
  async function annotate(change: AnnotationChange) {
    if (!selected) return;
    setError("");
    try {
      setSnapshot(await api.AnnotateSession(selected.key, change));
    } catch (e) {
      setError(errorText(e));
    }
  }
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
      if (fresh && newCompare) {
        const started = await api.StartComparison(
          newProject,
          newMessage.trim(),
        );
        setNewMessage("");
        setChats((items) => [
          ...items,
          ...started.chats.filter(
            (chat) => !items.some((item) => item.id === chat.id),
          ),
        ]);
        setActiveChat(started.chats[0]?.id || "");
        setPage("history");
        setShowNew(false);
        if (started.problem) setError(started.problem);
        return;
      }
      const live = fresh
        ? await api.NewChat(newProject, newTool)
        : await api.ResumeChat(selected!.key);
      // A task typed in the dialog is sent before the conversation opens. If
      // sending fails, the text is kept as that conversation's draft.
      if (fresh) rememberAgent(newProject, newTool);
      const first = fresh ? newMessage.trim() : "";
      let unsent = "";
      if (first) {
        try {
          await api.SendChat(live.id, first);
        } catch (e) {
          unsent = storeDraft(live.id, first)
            ? `The conversation is open, but your message was not sent: ${errorText(e)} Your text is in the message box.`
            : `The conversation is open, but your message was not sent and could not be saved: ${errorText(e)}`;
        }
      }
      setNewMessage("");
      setChats((items) =>
        items.some((item) => item.id === live.id) ? items : [...items, live],
      );
      setActiveChat(live.id);
      setShowRuns(false);
      setShowNew(false);
      setToast("");
      if (unsent) setError(unsent);
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
  const bothAgents =
    !!snapshot && snapshot.tools.filter((t) => t.available).length >= 2;
  const canStart =
    !!newProject &&
    !!snapshot?.projects.find((p) => p.path === newProject && p.available) &&
    (newCompare
      ? bothAgents && !!newMessage.trim()
      : !!snapshot?.tools.find((t) => t.name === newTool && t.available));

  const history: HistoryProps = {
    hidden: !!activeChat || otherPage,
    selected,
    sessions,
    loading,
    hasFilters,
    historyView,
    preview,
    busy,
    snapshot,
    setQuery,
    sessionRows,
    scopedSessions,
    canResume,
    tool,
    setSelectedKey,
    searchInput,
    searching,
    refresh,
    query,
    previewError,
    launch,
    clearFilters,
    annotate,
    setTool,
    setHistoryView,
    setHistorySort,
    project,
    pickProject,
    openNew,
    historySort,
  };

  return (
    <div className={`app-shell ${rail ? "rail" : ""}`}>
      <TopBar
        rail={rail}
        onToggleSidebar={() => setRail((on) => !on)}
        snapshot={snapshot}
        project={project}
        busy={busy}
        onProject={(path) => {
          showHistory();
          setProject(path);
          setQuery("");
        }}
        onAddProject={() => void pickProject()}
        onSearch={() => setShowPalette(true)}
      />
      <Sidebar
        view={otherPage ? page : activeChat ? "chat" : "history"}
        snapshot={snapshot}
        chats={chats}
        activeChat={otherPage ? "" : activeChat}
        closingChat={closingChat}
        newButton={newButton}
        canStart={!!snapshot && !busy}
        rail={rail}
        folded={folded}
        onFold={fold}
        onSettings={() => {
          setActiveChat("");
          setPage("settings");
        }}
        onBoard={() => {
          setActiveChat("");
          setPage("board");
        }}
        onIssues={() => {
          setActiveChat("");
          setPage("github");
        }}
        usage={usage.overview?.agents}
        onUsage={() => {
          setActiveChat("");
          setPage("usage");
        }}
        onNew={openNew}
        onHistory={showHistory}
        onRuns={() => {
          setActiveChat("");
          setShowRuns(true);
        }}
        onSaved={() => setShowLibrary(true)}
        onSelectChat={(id) => {
          setShowRuns(false);
          setActiveChat(id);
        }}
        onCloseChat={(id) => void closeChat(id)}
      />
      <main
        className={`workspace ${activeChat ? "has-chat" : ""} ${showRuns ? "has-runs" : ""}`}
      >
        {!activeChat && (
          <header className="workspace-header">
            <h1>
              {page === "settings"
                ? "Settings"
                : page === "usage"
                  ? "Usage limits"
                  : page === "github"
                    ? "Issues and pull requests"
                    : page === "board"
                      ? "Agents board"
                      : showRuns
                        ? "Parallel runs"
                        : project
                          ? currentProject?.name || basename(project)
                          : "Your conversations"}
            </h1>
            <p title={project}>
              {page === "settings"
                ? "Appearance, agents and shortcuts for this Mac."
                : page === "usage"
                  ? "How much of each agent's plan limits you have used, and when they reset."
                  : page === "github"
                    ? "Open issues and pull requests on GitHub. Start a task from an issue."
                    : page === "board"
                      ? boardSummary(chats)
                      : showRuns
                        ? project ||
                          "Launch independent tasks and review their changes."
                        : project ||
                          "Every conversation. Every project. One place to pick up where you left off."}
            </p>
          </header>
        )}
        <HistoryFilters {...history} />
        {incompatible.length > 0 && page !== "settings" && (
          <div className="notice error" role="alert">
            <span>
              {incompatible.map((item) => toolLabel(item.tool)).join(" and ")}{" "}
              may not work with this version of ais.
            </span>
            <button
              className="text-button"
              onClick={() => {
                setActiveChat("");
                setPage("settings");
              }}
            >
              See details in Settings
            </button>
          </div>
        )}
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
        {chats.map((live) => (
          <ChatWorkspace
            key={live.id}
            chat={live}
            session={snapshot?.sessions.find(
              (s) => s.tool === live.tool && s.id === live.sessionId,
            )}
            active={activeChat === live.id && !otherPage}
            reviewerAvailable={
              !!snapshot?.tools.some((t) => t.name !== live.tool && t.available)
            }
            detailsHidden={detailsHidden}
            onDetailsHidden={setDetailsHidden}
            onOpen={openChat}
            onSelect={(id) => {
              void api
                .ReopenChat(id)
                .then(openChat)
                .catch((e) => setError(errorText(e)));
            }}
            onState={chatChanged}
            onClosed={(ids) => ids.forEach(chatClosed)}
          />
        ))}
        {page === "board" && (
          <Board
            chats={chats}
            tools={snapshot?.tools || []}
            onOpen={(id) => {
              setPage("history");
              setActiveChat(id);
            }}
            onOpenRun={(run, task) => {
              setRunFocus({ run, task, at: Date.now() });
              setActiveChat("");
              setPage("runs");
            }}
          />
        )}
        {page === "github" && (
          <IssuesPage
            projects={snapshot?.projects || []}
            project={project}
            onStartTask={startTaskFrom}
          />
        )}
        {page === "usage" && (
          <UsagePage
            agents={usage.overview?.agents}
            tools={snapshot?.tools || []}
            loading={usage.loading}
            error={usage.error}
            onRefresh={() => void usage.refresh(true)}
          />
        )}
        {page === "settings" && (
          <Settings
            appearance={appearance}
            tools={snapshot?.tools || []}
            health={health}
            checking={loading}
            onCheck={() => {
              void refresh();
              checkHealth();
            }}
          />
        )}
        {showRuns && (
          <RunDashboard
            key={runFocus?.at || 0}
            focus={runFocus}
            project={project}
            projects={snapshot?.projects || []}
            tools={snapshot?.tools || []}
          />
        )}
        <HistoryBrowser {...history} />
      </main>
      {showPalette && (
        <CommandPalette
          commands={commands}
          onClose={() => setShowPalette(false)}
        />
      )}
      {showLibrary && (
        <SavedWorkspace
          project={project}
          onClose={() => setShowLibrary(false)}
          onOpen={openChat}
        />
      )}
      {showNew && (
        // Clicking the scrim dismisses the dialog; Escape and Cancel do the same.
        // eslint-disable-next-line jsx-a11y/no-static-element-interactions
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
              aria-label="Close new task"
              disabled={busy}
              onClick={() => {
                setShowNew(false);
                newButton.current?.focus();
              }}
            >
              <Icon name="close" />
            </button>
            <h2 id="new-session-title">New task</h2>
            <p>
              Describe what you want done. It starts as a new conversation in
              the project and with the agent you choose.
            </p>
            <label className="field-label" htmlFor="new-message">
              Task
            </label>
            <textarea
              id="new-message"
              className="new-task-message"
              placeholder="For example: add retries to the checkout request and cover them with tests"
              value={newMessage}
              disabled={busy}
              onChange={(e) => setNewMessage(e.target.value)}
              onKeyDown={(e) => {
                // Enter adds a line here; Cmd+Enter starts the task.
                if (
                  e.key === "Enter" &&
                  (e.metaKey || e.ctrlKey) &&
                  canStart &&
                  !busy
                ) {
                  e.preventDefault();
                  void launch(true);
                }
              }}
            />
            <label className="field-label" htmlFor="new-project">
              Project
            </label>
            <div className="project-select">
              <select
                id="new-project"
                value={newProject}
                onChange={(e) => {
                  setNewProject(e.target.value);
                  const remembered = rememberedAgent(e.target.value);
                  if (remembered) setNewTool(remembered);
                }}
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
                    className={`agent-option ${!newCompare && newTool === name ? "selected" : ""}`}
                    disabled={!available || busy}
                    aria-pressed={!newCompare && newTool === name}
                    onClick={() => {
                      setNewTool(name);
                      setNewCompare(false);
                    }}
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
              {bothAgents && (
                <button
                  className={`agent-option compare ${newCompare ? "selected" : ""}`}
                  disabled={busy}
                  aria-pressed={newCompare}
                  onClick={() => setNewCompare(true)}
                >
                  <span className="compare-marks" aria-hidden="true">
                    <AgentMark tool="claude" dot />
                    <AgentMark tool="codex" dot />
                  </span>
                  <span>
                    <strong>Both, compared</strong>
                    <small>Each in its own worktree</small>
                  </span>
                  <span className="radio-dot" />
                </button>
              )}
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
            <p className="new-task-note">
              {newCompare
                ? newMessage.trim()
                  ? "Both agents start on this task, each in its own Git worktree and branch from the last commit. Uncommitted changes in the project are not included, and nothing reaches it until you choose a version."
                  : "Describe the task to compare. Both agents get the same text."
                : newMessage.trim()
                  ? "The agent starts working as soon as you start the task."
                  : "Leave the task empty to open the conversation and write your message there. Nothing is sent until you do."}
            </p>
            <button
              className="button accent start-session"
              disabled={!canStart || busy}
              onClick={() => void launch(true)}
            >
              {busy
                ? "Starting…"
                : newCompare
                  ? "Start comparison"
                  : newMessage.trim()
                    ? "Start task"
                    : "Start in app"}
              {!busy && newMessage.trim() && <kbd>⌘⏎</kbd>}
              <Icon name="arrow" size={17} />
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
