import { useEffect, useId, useRef, useState } from "react";
import Icon from "../Icon";
import {
  api,
  errorText,
  toolLabel,
  type ChatState,
  type Pipeline,
} from "../api";

type StepState = "done" | "now" | "next";

/** Where each of the three steps stands. */
export function stepStates(
  step: Pipeline["step"],
): [StepState, StepState, StepState] {
  switch (step) {
    case "planning":
    case "unplanned":
      return ["now", "next", "next"];
    case "planned":
      return ["done", "now", "next"];
    case "building":
      return ["done", "now", "next"];
    case "built":
    case "reviewing":
      return ["done", "done", "now"];
    case "reviewed":
      return ["done", "done", "done"];
  }
}

/** What the task is waiting for, in words. */
export function stepNote(view: Pipeline) {
  const planner = toolLabel(view.planner);
  const builder = toolLabel(view.builder);
  switch (view.step) {
    case "planning":
      return `${planner} is planning. It changes nothing.`;
    case "unplanned":
      return `Planning ended without a plan. Ask ${planner} for one in the plan conversation.`;
    case "planned":
      return `The plan is ready. Read it, edit it if you like, then give it to ${builder}.`;
    case "building":
      return `${builder} is building the plan.`;
    case "built":
      return view.reviewChat
        ? `The review did not finish. Review the changes with ${planner} again.`
        : `Built. Review the changes with ${planner} next, or commit them yourself.`;
    case "reviewing":
      return `${planner} is reviewing the changes.`;
    case "reviewed":
      return view.findings
        ? `${planner} found ${view.findings} ${view.findings === 1 ? "thing" : "things"} to consider. Send the ones you agree with to ${builder} from the review.`
        : `${planner} found nothing to change. Commit when you are ready.`;
  }
}

/**
 * The guided task's steps — plan, build, review — above each of its
 * conversations: where it stands, links to each step's conversation, and the
 * next step as a button once it can start. Nothing starts on its own.
 */
export default function PipelineStrip({
  chat,
  active,
  onSelect,
  onOpen,
  onReview,
}: {
  chat: ChatState;
  active: boolean;
  onSelect: (chatId: string) => void;
  onOpen: (chat: ChatState) => void;
  /** Starts the usual review of this conversation's changes. */
  onReview?: () => void;
}) {
  const id = chat.pipeline?.id || "";
  const [view, setView] = useState<Pipeline>();
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<string>();
  const [building, setBuilding] = useState(false);
  // Read again when this conversation changes, and now and then while shown,
  // since the other steps' conversations change too.
  useEffect(() => {
    if (!id || !active) return;
    let live = true;
    const read = () =>
      api
        .PipelineView(id)
        .then((next) => {
          if (!live) return;
          setView(next);
          setError("");
        })
        .catch((e) => live && setError(errorText(e)));
    void read();
    const timer = window.setInterval(read, 4000);
    return () => {
      live = false;
      window.clearInterval(timer);
    };
  }, [id, active, chat.version]);
  if (!id) return null;
  async function build() {
    if (editing === undefined || !view) return;
    setBuilding(true);
    setError("");
    try {
      const started = await api.BuildPipeline(view.id, editing);
      setEditing(undefined);
      onOpen(started);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBuilding(false);
    }
  }
  const states = view ? stepStates(view.step) : undefined;
  const steps = view
    ? [
        { label: "Plan", tool: view.planner, chatId: view.planChat },
        { label: "Build", tool: view.builder, chatId: view.buildChat },
        { label: "Review", tool: view.planner, chatId: view.reviewChat },
      ]
    : [];
  const inBuild = !!view && chat.id === view.buildChat;
  return (
    <section className="pipeline-strip" aria-label="Guided task">
      <ol className="pipeline-steps">
        {steps.map((step, i) => {
          const here = step.chatId === chat.id;
          const label = `${step.label} · ${toolLabel(step.tool)}`;
          return (
            <li key={step.label} className={states?.[i]}>
              <span className="pipeline-mark" aria-hidden="true">
                {states?.[i] === "done" ? (
                  <Icon name="check" size={11} />
                ) : (
                  i + 1
                )}
              </span>
              {step.chatId && !here ? (
                <button
                  className="text-button"
                  title={`Open the ${step.label.toLowerCase()} conversation`}
                  onClick={() => onSelect(step.chatId!)}
                >
                  {label} ↗
                </button>
              ) : (
                <span aria-current={here ? "step" : undefined}>{label}</span>
              )}
            </li>
          );
        })}
      </ol>
      {view && (
        <p className="pipeline-note" role="status">
          {stepNote(view)}
        </p>
      )}
      {view?.step === "planned" && editing === undefined && (
        <button
          className="button primary"
          onClick={() => setEditing(view.plan || "")}
        >
          Build with {toolLabel(view.builder)}…
        </button>
      )}
      {view?.step === "built" && inBuild && onReview && (
        <button className="button primary" onClick={onReview}>
          Review with {toolLabel(view.planner)}…
        </button>
      )}
      {view?.step === "built" && !inBuild && view.buildChat && (
        <button
          className="button secondary"
          onClick={() => onSelect(view.buildChat!)}
        >
          Open the build to review it
        </button>
      )}
      {view?.step === "reviewed" &&
        view.reviewChat &&
        view.reviewChat !== chat.id && (
          <button
            className="button secondary"
            onClick={() => onSelect(view.reviewChat!)}
          >
            Open the review
          </button>
        )}
      {error && editing === undefined && (
        <p className="pipeline-note error" role="alert">
          {error}
        </p>
      )}
      {view && editing !== undefined && (
        <PlanDialog
          builder={toolLabel(view.builder)}
          plan={editing}
          busy={building}
          error={error}
          onChange={setEditing}
          onCancel={() => {
            setEditing(undefined);
            setError("");
          }}
          onBuild={() => void build()}
        />
      )}
    </section>
  );
}

/** The plan, to read and edit before the other agent builds it. */
function PlanDialog({
  builder,
  plan,
  busy,
  error,
  onChange,
  onCancel,
  onBuild,
}: {
  builder: string;
  plan: string;
  busy: boolean;
  error: string;
  onChange: (plan: string) => void;
  onCancel: () => void;
  onBuild: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const editor = useRef<HTMLTextAreaElement>(null);
  const id = useId();
  useEffect(() => {
    const el = dialog.current;
    if (el?.showModal) el.showModal();
    editor.current?.focus();
    return () => el?.close?.();
  }, []);
  return (
    <dialog
      ref={dialog}
      className="confirm-dialog plan-dialog"
      aria-labelledby={id}
      onClose={onCancel}
      onCancel={(e) => {
        e.preventDefault();
        if (!busy) onCancel();
      }}
    >
      <h2 id={id}>Build this plan with {builder}?</h2>
      <label className="pipeline-plan">
        <span>The plan {builder} will build. Edit it as you like.</span>
        <textarea
          ref={editor}
          value={plan}
          onChange={(e) => onChange(e.target.value)}
        />
      </label>
      <p className="confirm-body">
        Opens a new {builder} conversation in this project with the task and
        this plan, and starts it. Its edits and commands ask you as usual.
      </p>
      {error && (
        <p className="pipeline-note error" role="alert">
          {error}
        </p>
      )}
      <div className="confirm-actions">
        <button className="button secondary" disabled={busy} onClick={onCancel}>
          Cancel
        </button>
        <button
          className="button primary"
          disabled={busy || !plan.trim()}
          onClick={onBuild}
        >
          {busy ? "Starting…" : "Start the build"}
        </button>
      </div>
    </dialog>
  );
}
