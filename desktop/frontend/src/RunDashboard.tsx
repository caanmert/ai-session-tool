import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  api,
  errorText,
  toolLabel,
  type Project,
  type Tool,
  type Run,
  type RunStatus,
  type RunPatch,
} from "./api";
import DiffViewer from "./DiffViewer";
import "./runs.css";

type Props = { project: string; projects: Project[]; tools: Tool[] };
export default function RunDashboard({ project, projects, tools }: Props) {
  const [runs, setRuns] = useState<Run[]>([]);
  const [scope, setScope] = useState(project);
  const [runId, setRunId] = useState("");
  const [task, setTask] = useState("");
  const [file, setFile] = useState("");
  const [status, setStatus] = useState<RunStatus>();
  const [patch, setPatch] = useState<RunPatch & { context: string }>();
  const [error, setError] = useState("");
  const [statusError, setStatusError] = useState("");
  const [patchError, setPatchError] = useState("");
  const [loading, setLoading] = useState(true);
  const [patchLoading, setPatchLoading] = useState(false);
  const [revision, setRevision] = useState(0);
  const [showLaunch, setShowLaunch] = useState(false);
  const [stopTarget, setStopTarget] = useState<Run>();
  const [stopping, setStopping] = useState(false);
  const [copyNote, setCopyNote] = useState("");
  const stopOperation = useRef(false);
  const visible = runs.filter((run) => !scope || run.repository === scope);
  const selected = visible.find((run) => run.id === runId) || visible[0];
  const selectedTask =
    status && selected && status.id === selected.id
      ? status.tasks.find((item) => item.task.window === task) ||
        status.tasks[0]
      : undefined;
  const selectedFile = selectedTask?.files.some((item) => item.path === file)
    ? file
    : "";
  const patchContext = JSON.stringify([
    selected?.id,
    selectedTask?.task.window,
    selectedFile,
  ]);
  const displayedPatch = patch?.context === patchContext ? patch : undefined;

  useEffect(() => {
    let active = true;
    setLoading(true);
    api
      .ListRuns()
      .then((items) => {
        if (active) {
          setRuns(items);
          setError("");
        }
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
  useEffect(() => {
    let active = true,
      timer: ReturnType<typeof setTimeout>;
    setStatus((current) =>
      current?.id === selected?.id ? current : undefined,
    );
    if (!selected) return;
    async function poll() {
      try {
        const next = await api.RunStatus(selected!.id);
        if (active) {
          setStatus(next);
          setStatusError("");
        }
      } catch (e) {
        if (active) setStatusError(errorText(e));
      }
      if (active) timer = setTimeout(() => void poll(), 5000);
    }
    void poll();
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [selected?.id, revision]);
  useEffect(() => {
    let active = true;
    setPatchError("");
    if (!selected || !selectedTask || selectedTask.error) {
      setPatch(undefined);
      setPatchLoading(false);
      return;
    }
    setPatchLoading(true);
    api
      .RunDiff(selected.id, selectedTask.task.window, selectedFile)
      .then((next) => {
        if (active) setPatch({ ...next, context: patchContext });
      })
      .catch((e) => {
        if (active) {
          setPatchError(errorText(e));
          setPatch(undefined);
        }
      })
      .finally(() => {
        if (active) setPatchLoading(false);
      });
    return () => {
      active = false;
    };
  }, [selected?.id, selectedTask, selectedFile, status]);
  function chooseRun(id: string) {
    setRunId(id);
    setTask("");
    setFile("");
    setPatch(undefined);
  }
  async function stop() {
    if (!stopTarget || stopOperation.current) return;
    stopOperation.current = true;
    setStopping(true);
    try {
      await api.StopRun(stopTarget.id);
      setStopTarget(undefined);
      setRevision((n) => n + 1);
    } catch (e) {
      setError(errorText(e));
    } finally {
      stopOperation.current = false;
      setStopping(false);
    }
  }
  return (
    <section className="runs-workspace" aria-label="Parallel run dashboard">
      <div className="runs-toolbar">
        <label>
          Project{" "}
          <select
            aria-label="Run project filter"
            value={scope}
            onChange={(e) => setScope(e.target.value)}
          >
            <option value="">All projects</option>
            {[
              ...new Set([
                ...projects.map((p) => p.path),
                ...runs.map((run) => run.repository),
                ...(scope ? [scope] : []),
              ]),
            ]
              .sort()
              .map((path) => (
                <option key={path} value={path}>
                  {path}
                </option>
              ))}
          </select>
        </label>
        <span className="runs-refresh-note">
          Task status refreshes every 5 seconds
        </span>
        <button
          className="button subtle"
          disabled={loading}
          onClick={() => setRevision((n) => n + 1)}
        >
          Refresh runs
        </button>
        <button className="button primary" onClick={() => setShowLaunch(true)}>
          New parallel run
        </button>
      </div>
      {(error || statusError) && (
        <div className="notice error" role="alert">
          {error || statusError}
        </div>
      )}
      <div className="runs-grid">
        <aside className="runs-list" aria-label="Saved runs">
          <div className="runs-heading">
            Saved runs <b>{visible.length}</b>
          </div>
          {loading && (
            <p role="status" className="runs-empty">
              Loading runs…
            </p>
          )}
          {!loading && !visible.length && (
            <div className="runs-empty">
              <h3>No runs yet</h3>
              <p>
                Launch independent tasks in separate branches and worktrees, or
                choose another project.
              </p>
            </div>
          )}
          {visible.map((run) => (
            <button
              key={run.id}
              className={run.id === selected?.id ? "selected" : ""}
              aria-current={run.id === selected?.id ? "true" : undefined}
              onClick={() => chooseRun(run.id)}
            >
              <strong>
                {run.repository.split("/").filter(Boolean).at(-1)}
              </strong>
              <span>
                {toolLabel(run.tool)} · {run.tasks.length}{" "}
                {run.tasks.length === 1 ? "task" : "tasks"}
              </span>
              <time>
                {new Date(run.createdAt).toLocaleString([], {
                  dateStyle: "short",
                  timeStyle: "short",
                })}
              </time>
              <small>{run.id}</small>
            </button>
          ))}
        </aside>
        <div className="runs-detail">
          {selected ? (
            <>
              <header className="run-heading">
                <div>
                  <h2>{selected.id}</h2>
                  <p title={selected.repository}>{selected.repository}</p>
                </div>
                <button
                  className="button subtle"
                  onClick={() => setStopTarget(selected)}
                >
                  Stop run…
                </button>
              </header>
              <p className="run-scope">
                Changes since {selected.base.slice(0, 12)} · stopping preserves
                branches, worktrees and saved runs.
              </p>
              {status?.id === selected.id && status.warning && (
                <p className="runs-warning" role="status">
                  Pane inspection: {status.warning}. Git changes remain
                  available.
                </p>
              )}
              <div className="run-tasks" aria-label="Run tasks">
                {(status?.id === selected.id ? status.tasks : []).map(
                  (item) => (
                    <button
                      key={item.task.window}
                      className={
                        item.task.window === selectedTask?.task.window
                          ? "selected"
                          : ""
                      }
                      aria-pressed={
                        item.task.window === selectedTask?.task.window
                      }
                      onClick={() => {
                        setTask(item.task.window);
                        setFile("");
                        setPatch(undefined);
                      }}
                    >
                      <span className={`run-state ${item.state}`}>
                        {item.state}
                        {item.exitCode !== undefined
                          ? ` (${item.exitCode})`
                          : ""}
                        {item.signal ? ` · ${item.signal}` : ""}
                      </span>
                      <strong>{item.task.window}</strong>
                      <span className="task-prompt" title={item.task.prompt}>
                        {item.task.prompt}
                      </span>
                      <small>
                        {item.error
                          ? "Files unavailable"
                          : `${item.files.length} changed files`}
                      </small>
                      {item.attention && <em>Agent event</em>}
                      {item.quiet && <em>Silence alert</em>}
                    </button>
                  ),
                )}
                {!status && (
                  <p className="runs-empty" role="status">
                    Reading task status…
                  </p>
                )}
              </div>
              {selectedTask && (
                <>
                  <div className="run-task-meta">
                    <div className="run-task-path">
                      <span title={selectedTask.task.path}>
                        {selectedTask.currentBranch || selectedTask.task.branch}{" "}
                        · {selectedTask.task.path}
                      </span>
                      <button
                        className="button subtle"
                        disabled={!selectedTask.pane}
                        title={
                          selectedTask.pane
                            ? "Copy a command to open this task in a new terminal"
                            : "No terminal pane is available"
                        }
                        onClick={() =>
                          void (async () => {
                            try {
                              const command = await api.RunAttachCommand(
                                selected.id,
                                selectedTask.task.window,
                              );
                              await navigator.clipboard.writeText(command);
                              setCopyNote(
                                `Attach command copied for ${selectedTask.task.window}`,
                              );
                            } catch (e) {
                              setError(
                                `Could not copy attach command: ${errorText(e)}`,
                              );
                            }
                          })()
                        }
                      >
                        Copy attach command
                      </button>
                    </div>
                    {copyNote && (
                      <small role="status">
                        {copyNote}. Paste it into a new terminal to answer the
                        agent.
                      </small>
                    )}
                    <small>
                      Pane state describes a process. Alerts record events, not
                      proof an agent is still waiting.
                    </small>
                  </div>
                  {selectedTask.error ? (
                    <p className="runs-warning" role="alert">
                      {selectedTask.error}
                    </p>
                  ) : (
                    <div className="run-review-grid">
                      <aside
                        className="run-files"
                        aria-label="Task changed files"
                      >
                        <button
                          className={!selectedFile ? "selected" : ""}
                          onClick={() => setFile("")}
                        >
                          All changes
                        </button>
                        {selectedTask.files.map((change) => (
                          <button
                            key={change.path}
                            className={
                              selectedFile === change.path ? "selected" : ""
                            }
                            onClick={() => setFile(change.path)}
                          >
                            <span>{change.status}</span>
                            {change.path}
                          </button>
                        ))}
                      </aside>
                      <div className="run-patch" aria-busy={patchLoading}>
                        {patchError && (
                          <p className="runs-warning" role="alert">
                            {patchError}
                          </p>
                        )}
                        {displayedPatch?.truncated && (
                          <p className="runs-warning">
                            Preview shortened at 512 KiB. Select a file or use
                            ais run diff for the complete patch.
                          </p>
                        )}
                        {displayedPatch?.patch ? (
                          <DiffViewer
                            hideFiles
                            snapshot={{
                              files: [
                                {
                                  path: selectedFile || "All changes",
                                  patch: displayedPatch.patch,
                                },
                              ],
                            }}
                            selectedFile={selectedFile || "All changes"}
                            selectedLine={0}
                            onFile={() => {}}
                          />
                        ) : (
                          <p className="runs-empty" role="status">
                            {patchLoading
                              ? "Loading changes…"
                              : "No changes in this task."}
                          </p>
                        )}
                      </div>
                    </div>
                  )}
                </>
              )}
            </>
          ) : (
            !loading && (
              <div className="runs-empty">
                <h2>Independent tasks, one view</h2>
                <p>
                  Review saved runs here, including work from stopped agents.
                  Nothing is committed or merged automatically.
                </p>
              </div>
            )
          )}
        </div>
      </div>
      {showLaunch && (
        <LaunchRun
          project={scope}
          projects={projects}
          tools={tools}
          onClose={() => setShowLaunch(false)}
          onStarted={(run) => {
            setScope("");
            chooseRun(run.id);
            setRuns((items) => [
              run,
              ...items.filter((item) => item.id !== run.id),
            ]);
            setShowLaunch(false);
            setRevision((n) => n + 1);
          }}
          onPartial={() => setRevision((n) => n + 1)}
        />
      )}
      {stopTarget && (
        <RunDialog
          title="Stop this parallel run?"
          pending={stopping}
          onClose={() => setStopTarget(undefined)}
        >
          <p>
            This terminates the local agents in {stopTarget.id}. Branches,
            worktrees and saved changes remain. A daemon-backed agent may
            continue after its terminal exits.
          </p>
          {error && (
            <p className="runs-warning" role="alert">
              {error}
            </p>
          )}
          <footer>
            <button
              className="button subtle"
              disabled={stopping}
              onClick={() => setStopTarget(undefined)}
            >
              Keep running
            </button>
            <button
              className="button danger"
              disabled={stopping}
              onClick={() => void stop()}
            >
              {stopping ? "Stopping…" : "Stop agents"}
            </button>
          </footer>
        </RunDialog>
      )}
    </section>
  );
}

function RunDialog({
  title,
  pending,
  onClose,
  children,
}: {
  title: string;
  pending: boolean;
  onClose: () => void;
  children: ReactNode;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const dialog = ref.current;
    dialog?.showModal();
    return () => {
      dialog?.close();
      previous?.focus();
    };
  }, []);
  return (
    <dialog
      ref={ref}
      className="run-dialog"
      aria-label={title}
      onCancel={(e) => {
        if (pending) e.preventDefault();
        else onClose();
      }}
    >
      <header>
        <h2>{title}</h2>
        <button
          className="icon-button"
          aria-label="Close run dialog"
          disabled={pending}
          onClick={onClose}
        >
          ×
        </button>
      </header>
      {children}
    </dialog>
  );
}

function LaunchRun({
  project,
  projects,
  tools,
  onClose,
  onStarted,
  onPartial,
}: Props & {
  onClose: () => void;
  onStarted: (run: Run) => void;
  onPartial: () => void;
}) {
  const [directory, setDirectory] = useState(
    project || projects.find((p) => p.available)?.path || "",
  );
  const [tool, setTool] = useState(
    tools.find((t) => t.available)?.name || "codex",
  );
  const [prompts, setPrompts] = useState(["", ""]);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const operation = useRef(false);
  async function start() {
    if (
      operation.current ||
      !directory ||
      prompts.some((prompt) => !prompt.trim())
    )
      return;
    operation.current = true;
    setPending(true);
    setError("");
    try {
      onStarted(await api.StartRun(directory, tool, prompts));
    } catch (e) {
      setError(errorText(e));
      onPartial();
    } finally {
      operation.current = false;
      setPending(false);
    }
  }
  async function browse() {
    if (operation.current) return;
    operation.current = true;
    setPending(true);
    try {
      const path = await api.PickProject();
      if (path) setDirectory(path);
    } catch (e) {
      setError(errorText(e));
    } finally {
      operation.current = false;
      setPending(false);
    }
  }
  return (
    <RunDialog
      title="Launch independent tasks"
      pending={pending}
      onClose={onClose}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void start();
        }}
      >
        <p>
          Each task starts at the project's committed HEAD in its own branch,
          Git worktree and tmux window. Uncommitted edits are not copied. Agents
          use their normal permissions.
        </p>
        <div className="run-launch-fields">
          <label>
            Project
            <select
              aria-label="Launch project"
              value={directory}
              disabled={pending}
              onChange={(e) => setDirectory(e.target.value)}
            >
              <option value="">Choose a project…</option>
              {[
                ...new Set([
                  ...projects.filter((p) => p.available).map((p) => p.path),
                  ...(directory ? [directory] : []),
                ]),
              ].map((path) => (
                <option key={path} value={path}>
                  {path}
                </option>
              ))}
            </select>
          </label>
          <button
            type="button"
            className="button subtle"
            disabled={pending}
            onClick={() => void browse()}
          >
            Browse…
          </button>
          <label>
            Agent
            <select
              aria-label="Run agent"
              value={tool}
              disabled={pending}
              onChange={(e) => setTool(e.target.value as typeof tool)}
            >
              {tools.map((agent) => (
                <option
                  key={agent.name}
                  value={agent.name}
                  disabled={!agent.available}
                >
                  {toolLabel(agent.name)}
                  {!agent.available ? " · Not installed" : ""}
                </option>
              ))}
            </select>
          </label>
        </div>
        <div className="run-prompt-list">
          {prompts.map((prompt, index) => (
            <div key={index} className="run-prompt">
              <label htmlFor={`run-task-${index}`}>Task {index + 1}</label>
              <textarea
                id={`run-task-${index}`}
                value={prompt}
                maxLength={20000}
                disabled={pending}
                onChange={(e) =>
                  setPrompts((items) =>
                    items.map((item, i) =>
                      i === index ? e.target.value : item,
                    ),
                  )
                }
                placeholder="Describe one independent task…"
              />
              <button
                type="button"
                className="text-button"
                disabled={pending || prompts.length === 1}
                onClick={() =>
                  setPrompts((items) => items.filter((_, i) => i !== index))
                }
              >
                Remove task {index + 1}
              </button>
            </div>
          ))}
        </div>
        <button
          type="button"
          className="button subtle"
          disabled={pending || prompts.length >= 32}
          onClick={() => setPrompts((items) => [...items, ""])}
        >
          Add task
        </button>
        {error && (
          <p className="runs-warning" role="alert">
            {error}
          </p>
        )}
        <footer>
          <span>
            Requires Git, tmux and the selected agent. Work is preserved if
            setup fails.
          </span>
          <button
            type="button"
            className="button subtle"
            disabled={pending}
            onClick={onClose}
          >
            Cancel
          </button>
          <button
            type="submit"
            className="button primary"
            disabled={
              pending ||
              !directory ||
              !tools.some((agent) => agent.name === tool && agent.available) ||
              prompts.some((prompt) => !prompt.trim())
            }
          >
            {pending
              ? "Launching…"
              : `Launch ${prompts.length} ${prompts.length === 1 ? "agent" : "agents"}`}
          </button>
        </footer>
      </form>
    </RunDialog>
  );
}
