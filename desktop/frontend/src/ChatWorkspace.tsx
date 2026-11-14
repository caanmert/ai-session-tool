import { useEffect, useRef, useState } from "react";
import ModelPicker from "./ModelPicker";
import DiffViewer from "./DiffViewer";
import AgentMark from "./AgentMark";
import Icon from "./Icon";
import MessageList, { SafeMarkdown } from "./chat/MessageList";
import { applyChatUpdate } from "./chatUpdates";
import { useChatDraft } from "./useChatDraft";
import {
  api,
  errorText,
  toolLabel,
  type ChatState,
  type ChatApproval,
  type ReviewDraft,
} from "./api";

export const chatBusy = (status: string) =>
  ["running", "waiting", "stopping"].includes(status);
const statusLabel = (status: string) =>
  ({
    idle: "Ready",
    running: "Working",
    waiting: "Needs your input",
    stopping: "Stopping",
    stopped: "Stopped",
    error: "Needs attention",
  })[status] || status;
type Props = {
  chat: ChatState;
  active: boolean;
  reviewerAvailable: boolean;
  onOpen: (chat: ChatState) => void;
  onSelect: (id: string) => void;
  onState: (chat: ChatState) => void;
};
export default function ChatWorkspace({
  chat,
  active,
  reviewerAvailable,
  onOpen,
  onSelect,
  onState,
}: Props) {
  const [state, setState] = useState(chat);
  const { prompt, updatePrompt, draftError, restored } = useChatDraft(chat.id);
  const [error, setError] = useState("");
  const [actionWorking, setWorking] = useState(false);
  const [settingsBusy, setSettingsBusy] = useState(false);
  const working = actionWorking || settingsBusy;
  const settingsOperation = useRef(false);
  function settingsPending(value: boolean) {
    settingsOperation.current = value;
    setSettingsBusy(value);
  }
  const [retry, setRetry] = useState(0);
  const [draft, setDraft] = useState<ReviewDraft>();
  const [selected, setSelected] = useState<string[]>([]);
  const [file, setFile] = useState("");
  const [line, setLine] = useState(0);
  const [showDiff, setShowDiff] = useState(false);
  const messages = useRef<HTMLDivElement>(null);
  const following = useRef(true);
  const [awayFromLatest, setAwayFromLatest] = useState(false);
  const activeRef = useRef(active);
  activeRef.current = active;
  const notify = useRef(onState);
  notify.current = onState;
  const input = useRef<HTMLTextAreaElement>(null);
  const operation = useRef(false);
  useEffect(() => {
    let disposed = false;
    let timer: ReturnType<typeof setTimeout>;
    let version = -1;
    let current: ChatState | undefined;
    let lastStatus = chat.status;
    let lastTitle = chat.title;
    // The backend holds each read until the conversation changes, so this loop
    // costs nothing while idle. Updates carry only the messages that changed.
    async function poll() {
      try {
        const update = await api.ReadChat(chat.id, version);
        if (disposed) return;
        const next = applyChatUpdate(current, update);
        if (!next) {
          // A partial update did not fit: read the whole conversation again.
          version = -1;
          timer = setTimeout(() => void poll(), 0);
          return;
        }
        version = update.version;
        if (update.state) {
          current = next;
          setState(next);
          if (next.status !== lastStatus || next.title !== lastTitle) {
            notify.current(next);
            lastStatus = next.status;
            lastTitle = next.title;
          }
        }
        // A reply without changes means the backend's wait ran out, or the
        // backend answers at once (tests, browser fixtures): pause before
        // asking again.
        timer = setTimeout(
          () => void poll(),
          update.state ? 0 : activeRef.current ? 120 : 500,
        );
      } catch (e) {
        if (!disposed) setError(errorText(e));
      }
    }
    void poll();
    return () => {
      disposed = true;
      clearTimeout(timer);
    };
  }, [chat.id, retry]);
  useEffect(() => {
    if (active && !chatBusy(state.status)) input.current?.focus();
  }, [active]);
  useEffect(() => {
    if (following.current && messages.current)
      messages.current.scrollTop = messages.current.scrollHeight;
  }, [state.version, active]);
  const busy = chatBusy(state.status);
  const other = state.tool === "codex" ? "claude" : "codex";
  async function perform(action: () => Promise<void>) {
    if (operation.current || settingsOperation.current) return;
    operation.current = true;
    setWorking(true);
    setError("");
    try {
      await action();
    } catch (e) {
      setError(errorText(e));
    } finally {
      operation.current = false;
      setWorking(false);
    }
  }
  async function send() {
    if (!prompt.trim() || busy || settingsOperation.current) return;
    const text = prompt;
    await perform(async () => {
      await api.SendChat(state.id, text);
      updatePrompt("");
      setRetry((n) => n + 1);
      following.current = true;
      setAwayFromLatest(false);
    });
  }
  return (
    <section
      className="chat-workspace"
      hidden={!active}
      aria-label={`${toolLabel(state.tool)} conversation`}
    >
      <header className="chat-header">
        <div className="chat-heading">
          <p className="chat-breadcrumb" title={state.project}>
            {state.project.split("/").filter(Boolean).at(-1) || state.project}
          </p>
          <h2>{state.title || "New conversation"}</h2>
          <div className="chat-chips">
            <span className={`chip agent ${state.tool}`}>
              <AgentMark tool={state.tool} dot />
              {toolLabel(state.tool)}
            </span>
            <span className={`chip status ${state.status}`}>
              {(state.status === "waiting" || state.status === "error") && (
                <Icon name="alert" size={12} />
              )}
              {statusLabel(state.status)}
            </span>
            {state.activeModel && !state.review && (
              <span className="chat-meta">{state.activeModel}</span>
            )}
          </div>
        </div>
        <div className="chat-actions">
          {!state.review && (
            <button
              className="button secondary"
              disabled={busy || working || !reviewerAvailable}
              title={
                reviewerAvailable
                  ? "Preview the current working-tree diff before sending it"
                  : "Install the other agent to request a second review"
              }
              onClick={() =>
                void perform(async () =>
                  setDraft(await api.PrepareReview(state.id)),
                )
              }
            >
              Review with {toolLabel(other)}
            </button>
          )}
        </div>
      </header>
      {(error || state.error) && (
        <div className="notice error" role="alert">
          <span>{error || state.error}</span>
          {error && (
            <button
              className="text-button"
              onClick={() => {
                setError("");
                setRetry((n) => n + 1);
              }}
            >
              Reconnect
            </button>
          )}
        </div>
      )}
      {state.review && (
        <div className="review-banner">
          <div>
            <strong>Independent review by {toolLabel(state.tool)}</strong>
            <span>
              Snapshot {state.review.snapshot.hash.slice(0, 12)} ·{" "}
              {state.review.snapshot.files.length}{" "}
              {state.review.snapshot.files.length === 1 ? "file" : "files"} ·
              Analysis only
              {state.activeModel && <> · {state.activeModel}</>}
            </span>
          </div>
          <button
            className="text-button"
            onClick={() => onSelect(state.review!.sourceId)}
          >
            Original conversation ↗
          </button>
          <button
            className="text-button"
            onClick={() => setShowDiff((value) => !value)}
          >
            {showDiff ? "Hide diff" : "View reviewed diff"}
          </button>
        </div>
      )}
      {state.review && showDiff && (
        <DiffViewer
          snapshot={state.review.snapshot}
          selectedFile={file}
          selectedLine={line}
          onFile={(path) => {
            setFile(path);
            setLine(0);
          }}
        />
      )}
      <div
        className="chat-messages"
        ref={messages}
        onScroll={() => {
          const el = messages.current;
          if (el)
            following.current =
              el.scrollHeight - el.scrollTop - el.clientHeight < 80;
          setAwayFromLatest(!following.current);
        }}
        aria-live="polite"
        aria-relevant="additions"
      >
        {state.shortened && (
          <p className="preview-note">
            Earlier messages were shortened for display. The agent retains its
            saved conversation.
          </p>
        )}
        {!state.messages.length && !state.review && (
          <div className="chat-welcome">
            <div className="eyebrow">A CONVERSATION, READY TO BEGIN</div>
            <h2>What are we working on?</h2>
            <p>
              Describe a change, ask a question, or paste an error.
              <br />
              {toolLabel(state.tool)} will work in this project.
            </p>
            <div className="chat-suggestions">
              Live replies <span>·</span> Tool activity <span>·</span> Approvals
              here
            </div>
          </div>
        )}
        <MessageList
          tool={state.tool}
          messages={
            state.review?.complete
              ? state.messages.filter(
                  (m) => !(m.role === "assistant" && m.kind === "text"),
                )
              : state.messages
          }
        />
        {state.review?.complete && (
          <div className="review-findings">
            <div className="eyebrow">REVIEW RESULTS</div>
            <h3>
              {state.review.findings.length
                ? `${state.review.findings.length} ${state.review.findings.length === 1 ? "finding" : "findings"} to consider`
                : "No actionable findings reported"}
            </h3>
            <p>{state.review.summary}</p>
            {state.review.findings.map((finding) => (
              <article
                className={`review-finding ${finding.sent ? "sent" : ""}`}
                key={finding.id}
              >
                <label>
                  <input
                    type="checkbox"
                    aria-label={`Select ${finding.title}`}
                    checked={selected.includes(finding.id)}
                    disabled={finding.sent || working}
                    onChange={(e) =>
                      setSelected((ids) =>
                        e.target.checked
                          ? [...ids, finding.id]
                          : ids.filter((id) => id !== finding.id),
                      )
                    }
                  />
                  <span className={`priority ${finding.priority}`}>
                    {finding.priority}
                  </span>
                  <strong>{finding.title}</strong>
                  {finding.sent && <small>Sent for fixes</small>}
                </label>
                <button
                  className="finding-location"
                  onClick={() => {
                    setFile(finding.file);
                    setLine(finding.line);
                    setShowDiff(true);
                  }}
                >
                  {finding.file}
                  {finding.line ? `:${finding.line}` : ""}
                </button>
                <div className="markdown">
                  <SafeMarkdown text={finding.body} />
                </div>
              </article>
            ))}
            {!!state.review.findings.length && (
              <button
                className="button primary"
                disabled={!selected.length || working}
                onClick={() =>
                  void perform(async () => {
                    const source = await api.SendReviewFindings(
                      state.id,
                      selected,
                    );
                    setSelected([]);
                    onSelect(source);
                  })
                }
              >
                Send {selected.length || "selected"} findings to{" "}
                {toolLabel(other)}
              </button>
            )}
            <p className="preview-note">
              Findings are suggestions. Selected findings go back as a request
              to investigate and fix; they do not apply a patch automatically.
            </p>
          </div>
        )}
        {busy && (
          <div className="chat-working" role="status">
            <span className="loader" />
            {state.status === "waiting"
              ? "Waiting for your response below"
              : state.status === "stopping"
                ? "Stopping the response…"
                : `${toolLabel(state.tool)} is working…`}
          </div>
        )}
      </div>
      {awayFromLatest && (
        <button
          className="jump-latest"
          onClick={() => {
            following.current = true;
            setAwayFromLatest(false);
            if (messages.current)
              messages.current.scrollTop = messages.current.scrollHeight;
          }}
        >
          Jump to latest ↓
        </button>
      )}
      {!!state.approvals.length && (
        <div className="approval-list">
          {state.approvals.map((approval) => (
            <ApprovalCard
              key={approval.id}
              approval={approval}
              onAnswer={(allow, answers) =>
                api.AnswerChat(state.id, approval.id, allow, answers)
              }
            />
          ))}
        </div>
      )}
      <form
        className={`chat-composer ${state.review ? "review-footer" : ""}`}
        onSubmit={(e) => {
          e.preventDefault();
          void send();
        }}
      >
        {!state.review && (
          <textarea
            ref={input}
            aria-label="Message agent"
            placeholder={`Message ${toolLabel(state.tool)} about this project…`}
            value={prompt}
            disabled={working}
            onChange={(e) => updatePrompt(e.target.value)}
            onKeyDown={(e) => {
              if (
                e.key === "Enter" &&
                !e.shiftKey &&
                !e.nativeEvent.isComposing &&
                e.nativeEvent.keyCode !== 229
              ) {
                e.preventDefault();
                void send();
              }
            }}
          />
        )}
        {!state.review && (prompt || draftError) && (
          <div className="draft-status">
            <span
              role={draftError ? "alert" : "status"}
              className={draftError ? "draft-error" : ""}
            >
              {draftError ||
                (restored
                  ? "Draft restored from this Mac"
                  : "Draft saved on this Mac")}
            </span>
            {!!prompt && (
              <button
                type="button"
                disabled={working}
                onClick={() => updatePrompt("")}
              >
                Discard draft
              </button>
            )}
          </div>
        )}
        {!state.review && (
          <ModelPicker
            id={state.id}
            settings={state.settings}
            active={active}
            disabled={busy || actionWorking}
            onPending={settingsPending}
            onSaved={() => setRetry((n) => n + 1)}
          />
        )}
        <div className="composer-footer">
          <span>
            {state.review
              ? "Reviewing a fixed snapshot of your uncommitted changes."
              : "Enter to send · Shift+Enter for a new line"}
          </span>
          {busy ? (
            <button
              type="button"
              className="button secondary stop"
              title="Ends the response. Edits already made are kept."
              disabled={working || state.status === "stopping"}
              onClick={() =>
                void perform(async () => {
                  await api.StopChat(state.id);
                  setRetry((n) => n + 1);
                })
              }
            >
              <Icon name="stop" size={13} /> Stop response
            </button>
          ) : (
            !state.review && (
              <button
                type="submit"
                className="button primary send"
                aria-label="Send"
                title="Send (Enter)"
                disabled={!prompt.trim() || working}
              >
                <Icon name="arrow-up" size={17} />
              </button>
            )
          )}
        </div>
      </form>
      {draft && (
        <ReviewPreview
          draft={draft}
          settings={state.reviewerSettings}
          onSettingsPending={settingsPending}
          onSettingsSaved={() => setRetry((n) => n + 1)}
          error={error}
          busy={working}
          onCancel={() => {
            if (!working) setDraft(undefined);
          }}
          onStart={() =>
            void perform(async () => {
              const review = await api.StartReview(draft.id);
              setDraft(undefined);
              onOpen(review);
            })
          }
        />
      )}
    </section>
  );
}

function ApprovalCard({
  approval,
  onAnswer,
}: {
  approval: ChatApproval;
  onAnswer: (allow: boolean, answers: Record<string, string>) => Promise<void>;
}) {
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const operation = useRef(false);
  async function answer(allow: boolean) {
    if (operation.current) return;
    operation.current = true;
    setBusy(true);
    setError("");
    try {
      await onAnswer(allow, answers);
    } catch (e) {
      setError(errorText(e));
      operation.current = false;
      setBusy(false);
    }
  }
  return (
    <section
      className="approval-card"
      aria-label={approval.title}
      onKeyDown={(e) => {
        // Shortcuts apply only while focus is inside the card, and never
        // while typing an answer.
        const typing =
          e.target instanceof HTMLInputElement ||
          e.target instanceof HTMLSelectElement;
        if (e.key === "Escape" && !busy) {
          e.preventDefault();
          void answer(false);
        } else if (
          e.key === "Enter" &&
          !typing &&
          !(e.target instanceof HTMLButtonElement) &&
          !busy &&
          !approval.questions.length
        ) {
          e.preventDefault();
          void answer(true);
        }
      }}
    >
      <h3>
        <Icon name="alert" size={17} />
        {approval.title}
      </h3>
      {approval.detail && (
        // Focusable so a long command can be scrolled from the keyboard, and
        // so the card's shortcuts can be used without the pointer.
        // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
        <pre tabIndex={0} aria-label="Action details">
          {approval.detail}
        </pre>
      )}
      {approval.questions.map((question) => (
        <label className="approval-question" key={question.id}>
          <strong>{question.text}</strong>
          {question.options.length > 0 && (
            <select
              aria-label={`Options for ${question.text}`}
              value={
                question.options.includes(answers[question.id])
                  ? answers[question.id]
                  : ""
              }
              onChange={(e) =>
                setAnswers((v) => ({ ...v, [question.id]: e.target.value }))
              }
            >
              <option value="">Choose or write an answer…</option>
              {question.options.map((option) => (
                <option key={option}>{option}</option>
              ))}
            </select>
          )}
          <input
            aria-label={question.text}
            type={question.secret ? "password" : "text"}
            value={answers[question.id] || ""}
            onChange={(e) =>
              setAnswers((v) => ({ ...v, [question.id]: e.target.value }))
            }
          />
        </label>
      ))}
      {error && <p role="alert">{error}</p>}
      <div className="approval-actions">
        <button
          className="button primary"
          disabled={
            busy || approval.questions.some((q) => !answers[q.id]?.trim())
          }
          onClick={() => void answer(true)}
        >
          {busy
            ? "Sending…"
            : approval.questions.length
              ? "Send answers"
              : "Allow once"}
        </button>
        <button
          className="button secondary"
          disabled={busy}
          onClick={() => void answer(false)}
        >
          Deny <kbd>Esc</kbd>
        </button>
      </div>
    </section>
  );
}

function ReviewPreview({
  draft,
  error,
  busy,
  settings,
  onSettingsPending,
  onSettingsSaved,
  onCancel,
  onStart,
}: {
  draft: ReviewDraft;
  settings?: import("./api").ModelSettings;
  onSettingsPending: (value: boolean) => void;
  onSettingsSaved: () => void;
  error: string;
  busy: boolean;
  onCancel: () => void;
  onStart: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [file, setFile] = useState("");
  useEffect(() => {
    const el = dialog.current;
    if (el?.showModal) el.showModal();
    else el?.setAttribute("open", "");
    return () => el?.close?.();
  }, []);
  return (
    <dialog
      className="review-dialog"
      ref={dialog}
      onCancel={(e) => {
        e.preventDefault();
        if (!busy) onCancel();
      }}
      aria-labelledby="review-preview-title"
    >
      <header>
        <div className="eyebrow">REVIEW WITH ANOTHER AGENT</div>
        <h2 id="review-preview-title">
          Send this diff to {toolLabel(draft.tool)}?
        </h2>
        <p>
          All current staged, unstaged and eligible untracked changes in this
          Git working tree. This can include changes made outside this
          conversation.
        </p>
        <small>
          {draft.snapshot.files.length}{" "}
          {draft.snapshot.files.length === 1 ? "file" : "files"} · Base{" "}
          {draft.snapshot.base.slice(0, 10)} · Snapshot{" "}
          {draft.snapshot.hash.slice(0, 12)}
        </small>
      </header>
      <ModelPicker
        id={draft.sourceId}
        settings={settings || draft.settings}
        reviewer
        disabled={busy}
        onPending={onSettingsPending}
        onSaved={onSettingsSaved}
      />
      {!!draft.snapshot.warnings.length && (
        <div className="review-warnings">
          {draft.snapshot.warnings.map((w) => (
            <p key={w}>{w}</p>
          ))}
        </div>
      )}
      <DiffViewer
        snapshot={draft.snapshot}
        selectedFile={file}
        selectedLine={0}
        onFile={setFile}
      />
      {error && (
        <div className="notice error" role="alert">
          {error}
        </div>
      )}
      <footer>
        <span>
          The other agent receives this snapshot for analysis. No fixes are
          applied during review.
        </span>
        <button className="button subtle" disabled={busy} onClick={onCancel}>
          Cancel
        </button>
        <button className="button accent" disabled={busy} onClick={onStart}>
          {busy
            ? "Starting review…"
            : `Send snapshot to ${toolLabel(draft.tool)}`}
        </button>
      </footer>
    </dialog>
  );
}
