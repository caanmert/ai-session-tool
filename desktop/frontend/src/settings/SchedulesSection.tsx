import { useEffect, useState } from "react";
import Icon from "../Icon";
import ConfirmDialog from "../ui/ConfirmDialog";
import { basename } from "../ui/format";
import {
  api,
  errorText,
  toolLabel,
  type Project,
  type Schedule,
  type Tool,
  type ToolName,
} from "../api";

const DAYS: [string, string][] = [
  ["daily", "Every day"],
  ["weekdays", "Weekdays"],
  ["mon", "Mondays"],
  ["tue", "Tuesdays"],
  ["wed", "Wednesdays"],
  ["thu", "Thursdays"],
  ["fri", "Fridays"],
  ["sat", "Saturdays"],
  ["sun", "Sundays"],
];

/** "Weekdays at 18:00". */
export function whenText(task: Pick<Schedule, "days" | "time">) {
  const days = DAYS.find(([id]) => id === task.days)?.[1] || task.days;
  return `${days} at ${task.time}`;
}

function modeLabel(task: Pick<Schedule, "mode" | "tool">) {
  if (task.mode === "plan") return "Plan";
  return task.tool === "claude" ? "Ask" : "Default";
}

function stamp(value: string) {
  return new Date(value).toLocaleString("en-US", {
    weekday: "short",
    day: "numeric",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

/** What came of the latest scheduled time, in words. */
export function lastText(task: Schedule) {
  if (!task.lastAt || !task.lastResult) return "";
  const when = stamp(task.lastAt);
  if (task.lastResult === "started") return `Ran ${when}`;
  if (task.lastResult === "missed")
    return `Missed ${when}: ${task.lastNote || ""}`.trim();
  return `Skipped ${when}: ${task.lastNote || ""}`.trim();
}

type Draft = Omit<Schedule, "enabled"> & { enabled?: boolean };

const blank = (tool: ToolName, project: string): Draft => ({
  id: "",
  name: "",
  project,
  tool,
  prompt: "",
  mode: "plan",
  days: "weekdays",
  time: "18:00",
});

/**
 * Saved prompts an agent is sent at a set time, while ais is open. Each task
 * starts off; turning it on asks first. A time that passes while ais is
 * closed is missed, never made up later.
 */
export default function SchedulesSection({
  tools,
  projects,
  onOpenChat,
}: {
  tools: Tool[];
  projects: Project[];
  onOpenChat?: (chatId: string) => void;
}) {
  const [tasks, setTasks] = useState<Schedule[]>();
  const [draft, setDraft] = useState<Draft>();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [enabling, setEnabling] = useState<Schedule>();
  const [removing, setRemoving] = useState<Schedule>();
  const installed = tools.filter((t) => t.available).map((t) => t.name);
  const choices = projects.filter((p) => p.available);
  function load() {
    return api
      .Schedules()
      .then((list) => setTasks(list || []))
      .catch((e) => {
        setTasks((current) => current || []);
        setError(errorText(e));
      });
  }
  useEffect(() => {
    void load();
    // Keep "next" and "last" fresh while the page is open.
    const timer = window.setInterval(() => void load(), 30_000);
    return () => window.clearInterval(timer);
  }, []);
  async function act(id: string, action: () => Promise<unknown>) {
    setBusy(id);
    setError("");
    try {
      await action();
      await load();
      return true;
    } catch (e) {
      setError(errorText(e));
      return false;
    } finally {
      setBusy("");
    }
  }
  async function save() {
    if (!draft) return;
    const ok = await act(draft.id || "new", () =>
      api.SaveSchedule({ ...draft, enabled: false }),
    );
    if (ok) setDraft(undefined);
  }
  async function choose() {
    if (!draft) return;
    try {
      const path = await api.PickProject();
      if (path) setDraft({ ...draft, project: path });
    } catch (e) {
      setError(errorText(e));
    }
  }
  const form = draft && (
    <div className="prompt-form schedule-form">
      <label>
        <span>Name</span>
        <input
          value={draft.name}
          maxLength={80}
          placeholder="Evening review"
          // The form opens on the user's request.
          // eslint-disable-next-line jsx-a11y/no-autofocus
          autoFocus
          onChange={(e) => setDraft({ ...draft, name: e.target.value })}
        />
      </label>
      <div className="schedule-fields">
        <label>
          <span>Project</span>
          <span className="schedule-project">
            <select
              value={draft.project}
              onChange={(e) => setDraft({ ...draft, project: e.target.value })}
            >
              {!draft.project && <option value="">Choose a project</option>}
              {draft.project &&
                !choices.some((p) => p.path === draft.project) && (
                  <option value={draft.project}>
                    {basename(draft.project)}
                  </option>
                )}
              {choices.map((p) => (
                <option key={p.path} value={p.path}>
                  {p.name}
                </option>
              ))}
            </select>
            <button
              type="button"
              className="button secondary"
              onClick={() => void choose()}
            >
              Choose…
            </button>
          </span>
        </label>
        <label>
          <span>Agent</span>
          <select
            value={draft.tool}
            onChange={(e) =>
              setDraft({ ...draft, tool: e.target.value as ToolName })
            }
          >
            {(["codex", "claude"] as ToolName[]).map((tool) => (
              <option
                key={tool}
                value={tool}
                disabled={!installed.includes(tool)}
              >
                {toolLabel(tool)}
              </option>
            ))}
          </select>
        </label>
        <label>
          <span>Mode</span>
          <select
            value={draft.mode}
            onChange={(e) => setDraft({ ...draft, mode: e.target.value })}
          >
            <option value="plan">
              Plan: reads and proposes, changes nothing
            </option>
            <option value="">
              {draft.tool === "claude"
                ? "Ask: asks before edits and commands"
                : "Default: edits the project, asks before more"}
            </option>
          </select>
        </label>
        <label>
          <span>Days</span>
          <select
            value={draft.days}
            onChange={(e) => setDraft({ ...draft, days: e.target.value })}
          >
            {DAYS.map(([id, label]) => (
              <option key={id} value={id}>
                {label}
              </option>
            ))}
          </select>
        </label>
        <label>
          <span>Time</span>
          <input
            type="time"
            value={draft.time}
            required
            onChange={(e) => setDraft({ ...draft, time: e.target.value })}
          />
        </label>
      </div>
      <label>
        <span>Message</span>
        <textarea
          value={draft.prompt}
          placeholder="Review today's uncommitted changes for bugs. List what you find; change nothing."
          onChange={(e) => setDraft({ ...draft, prompt: e.target.value })}
        />
      </label>
      <div className="prompt-form-actions">
        <button
          className="button secondary"
          disabled={!!busy}
          onClick={() => setDraft(undefined)}
        >
          Cancel
        </button>
        <button
          className="button primary"
          disabled={
            !!busy ||
            !draft.name.trim() ||
            !draft.prompt.trim() ||
            !draft.project ||
            !draft.time
          }
          onClick={() => void save()}
        >
          {busy ? "Saving…" : "Save task"}
        </button>
      </div>
    </div>
  );
  return (
    <section aria-labelledby="settings-schedules">
      <div className="settings-heading">
        <h2 id="settings-schedules">Scheduled tasks</h2>
        <button
          className="button secondary"
          disabled={!tasks || !!draft || !installed.length}
          title={installed.length ? undefined : "Install an agent first"}
          onClick={() =>
            setDraft(
              blank(
                (installed[0] as ToolName) || "codex",
                choices[0]?.path || "",
              ),
            )
          }
        >
          <Icon name="plus" size={14} /> New task
        </button>
      </div>
      <p className="setting-note">
        Sends a message to an agent at a set time, in a new conversation named
        after the task. Tasks run only while ais is open; a time that passes
        while it is closed is missed, not made up later. A task starts off, and
        the agent still asks you before anything its mode doesn&apos;t allow.
      </p>
      <div className="setting-group">
        {!tasks && <p className="setting-note">Reading…</p>}
        {tasks && !tasks.length && !draft && (
          <p className="setting-note">No scheduled tasks yet.</p>
        )}
        {!!tasks?.length && (
          <ul className="prompt-list schedule-list">
            {tasks.map((task) =>
              draft?.id === task.id ? (
                <li key={task.id}>{form}</li>
              ) : (
                <li key={task.id}>
                  <label
                    className="schedule-switch"
                    title={task.enabled ? "Turn off" : "Turn on"}
                  >
                    <input
                      type="checkbox"
                      role="switch"
                      aria-label={`Run ${task.name} on schedule`}
                      checked={task.enabled}
                      disabled={!!busy || !!draft}
                      onChange={() =>
                        task.enabled
                          ? void act(task.id, () =>
                              api.SetScheduleEnabled(task.id, false),
                            )
                          : setEnabling(task)
                      }
                    />
                  </label>
                  <div>
                    <strong>{task.name}</strong>
                    <span>
                      {whenText(task)} · {toolLabel(task.tool)} ·{" "}
                      {basename(task.project)} · {modeLabel(task)}
                    </span>
                    {task.enabled && task.next && (
                      <span>Next: {stamp(task.next)}</span>
                    )}
                    {lastText(task) && (
                      <span
                        className={
                          task.lastResult === "started" ? "" : "schedule-miss"
                        }
                      >
                        {lastText(task)}
                        {task.lastResult === "started" &&
                          task.lastChat &&
                          onOpenChat && (
                            <>
                              {" · "}
                              <button
                                className="text-button"
                                onClick={() => onOpenChat(task.lastChat!)}
                              >
                                Open conversation
                              </button>
                            </>
                          )}
                      </span>
                    )}
                  </div>
                  <button
                    className="text-button"
                    aria-label={`Run ${task.name} now`}
                    disabled={!!busy || !!draft}
                    title="Open its conversation and send the message now"
                    onClick={() =>
                      void act(task.id, async () => {
                        const chat = await api.RunSchedule(task.id);
                        onOpenChat?.(chat.id);
                      })
                    }
                  >
                    {busy === task.id ? "Starting…" : "Run now"}
                  </button>
                  <button
                    className="text-button"
                    aria-label={`Edit ${task.name}`}
                    disabled={!!busy || !!draft}
                    onClick={() => setDraft({ ...task })}
                  >
                    Edit
                  </button>
                  <button
                    className="text-button"
                    aria-label={`Remove ${task.name}`}
                    disabled={!!busy || !!draft}
                    onClick={() => setRemoving(task)}
                  >
                    Remove
                  </button>
                </li>
              ),
            )}
          </ul>
        )}
        {draft && !draft.id && form}
        {error && (
          <p className="setting-note" role="alert">
            {error}
          </p>
        )}
      </div>
      {enabling && (
        <ConfirmDialog
          title={`Turn on “${enabling.name}”?`}
          confirmLabel="Turn on"
          cancelLabel="Cancel"
          busy={busy === enabling.id}
          onCancel={() => setEnabling(undefined)}
          onConfirm={() =>
            void act(enabling.id, () =>
              api.SetScheduleEnabled(enabling.id, true),
            ).then(() => setEnabling(undefined))
          }
        >
          <p>
            {whenText(enabling)}, while ais is open, it will start{" "}
            {toolLabel(enabling.tool)} on its own in{" "}
            {basename(enabling.project)} and send the task&apos;s message, in{" "}
            {modeLabel(enabling)} mode. This uses your{" "}
            {toolLabel(enabling.tool)} plan like any message.
          </p>
        </ConfirmDialog>
      )}
      {removing && (
        <ConfirmDialog
          title={`Remove “${removing.name}”?`}
          confirmLabel="Remove"
          cancelLabel="Keep"
          busy={busy === removing.id}
          onCancel={() => setRemoving(undefined)}
          onConfirm={() =>
            void act(removing.id, () => api.DeleteSchedule(removing.id)).then(
              () => setRemoving(undefined),
            )
          }
        >
          <p>Conversations it already opened stay.</p>
        </ConfirmDialog>
      )}
    </section>
  );
}
