import { useEffect, useRef, useState } from "react";
import Markdown from "react-markdown";
import ModelPicker from "./ModelPicker";
import {
  api,
  errorText,
  toolLabel,
  type ChatState,
  type ChatApproval,
  type ReviewDraft,
  type ReviewSnapshot,
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
export function SafeMarkdown({ text }: { text: string }) {
  return (
    <Markdown
      components={{
        a: ({ children }) => (
          <span className="transcript-link">{children}</span>
        ),
        img: () => <span>[Image omitted]</span>,
      }}
    >
      {text}
    </Markdown>
  );
}
type Props = {
  chat: ChatState;
  active: boolean;
  reviewerAvailable: boolean;
  onOpen: (chat: ChatState) => void;
  onSelect: (id: string) => void;
  onClose: (id: string) => void;
  onState: (chat: ChatState) => void;
};
export default function ChatWorkspace({
  chat,
  active,
  reviewerAvailable,
  onOpen,
  onSelect,
  onClose,
  onState,
}: Props) {
  const [state, setState] = useState(chat);
  const [prompt, setPrompt] = useState("");
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
    let lastStatus = chat.status;
    let lastTitle = chat.title;
    async function poll() {
      try {
        const update = await api.ReadChat(chat.id, version);
        if (disposed) return;
        version = update.version;
        if (update.state) {
          const next = update.state;
          setState(next);
          if (next.status !== lastStatus || next.title !== lastTitle) {
            notify.current(next);
            lastStatus = next.status;
            lastTitle = next.title;
          }
        }
        timer = setTimeout(() => void poll(), activeRef.current ? 120 : 500);
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
  }, [state.version]);
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
      setPrompt("");
      setRetry((n) => n + 1);
      following.current = true;
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
          <span className={`chat-agent ${state.tool}`}>
            {state.tool === "claude" ? "✳" : "◈"}
          </span>
          <div>
            <h2>{state.title}</h2>
            <p title={state.project}>
              {toolLabel(state.tool)} <span>·</span> {state.project}
            </p>
          </div>
        </div>
        <div className="chat-actions">
          <span className={`chat-status ${state.status}`}>
            <i />
            {statusLabel(state.status)}
          </span>
          {!state.review && (
            <button
              className="button subtle"
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
          {!busy && (
            <button
              className="icon-button"
              aria-label="Close conversation tab"
              disabled={working}
              onClick={() =>
                void perform(async () => {
                  await api.CloseChat(state.id);
                  onClose(state.id);
                })
              }
            >
              ×
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
        {state.messages
          .filter(
            (message) =>
              !(
                state.review?.complete &&
                message.role === "assistant" &&
                message.kind === "text"
              ),
          )
          .map((message) =>
            message.kind === "tool" ? (
              <details key={message.id} className="chat-tool">
                <summary>
                  <span className={`tool-indicator ${message.status}`} />
                  <strong>{message.name || "Tool activity"}</strong>
                  <small>{message.status || "Saved"}</small>
                </summary>
                <pre>{message.text || "Waiting for output…"}</pre>
              </details>
            ) : (
              <article
                key={message.id}
                className={`chat-message ${message.role}`}
              >
                <div className="chat-message-author">
                  {message.role === "user"
                    ? "You"
                    : message.role === "system"
                      ? "ais"
                      : toolLabel(state.tool)}
                </div>
                <div className="markdown">
                  {message.role === "assistant" && message.model && (
                    <div className="message-model">{message.model}</div>
                  )}
                  <SafeMarkdown text={message.text} />
                </div>
              </article>
            ),
          )}
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
                className="button accent"
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
            onChange={(e) => setPrompt(e.target.value)}
            onKeyDown={(e) => {
              if (
                e.key === "Enter" &&
                !e.shiftKey &&
                !e.nativeEvent.isComposing
              ) {
                e.preventDefault();
                void send();
              }
            }}
          />
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
              className="button subtle"
              disabled={working || state.status === "stopping"}
              onClick={() =>
                void perform(async () => {
                  await api.StopChat(state.id);
                  setRetry((n) => n + 1);
                })
              }
            >
              Stop response
            </button>
          ) : (
            !state.review && (
              <button
                type="submit"
                className="button accent"
                disabled={!prompt.trim() || working}
              >
                Send ↑
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
    <section className="approval-card" aria-label={approval.title}>
      <div className="eyebrow">YOUR DECISION</div>
      <h3>{approval.title}</h3>
      <details>
        <summary>Action details</summary>
        <pre>{approval.detail}</pre>
      </details>
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
          className="button subtle"
          disabled={busy}
          onClick={() => void answer(false)}
        >
          Decline
        </button>
        <button
          className="button accent"
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
function DiffViewer({
  snapshot,
  selectedFile,
  selectedLine,
  onFile,
}: {
  snapshot: ReviewSnapshot;
  selectedFile: string;
  selectedLine: number;
  onFile: (file: string) => void;
}) {
  const selected =
    snapshot.files.find((f) => f.path === selectedFile) || snapshot.files[0];
  const highlight = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const row = highlight.current;
    const container = row?.parentElement;
    if (selectedLine && row && container) {
      container.scrollTop +=
        row.getBoundingClientRect().top -
        container.getBoundingClientRect().top -
        container.clientHeight / 2;
    }
  }, [selectedFile, selectedLine]);
  let oldLine = 0,
    newLine = 0;
  return (
    <div className="review-diff">
      <aside aria-label="Changed files">
        {snapshot.files.map((file) => (
          <button
            className={file.path === selected?.path ? "active" : ""}
            key={file.path}
            onClick={() => onFile(file.path)}
          >
            {file.path}
          </button>
        ))}
      </aside>
      <div className="diff-code" aria-label="Reviewed diff">
        {selected?.patch.split("\n").map((text, index) => {
          const hunk = text.match(/^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/);
          let old: number | undefined, next: number | undefined;
          let kind = "meta";
          if (hunk) {
            oldLine = Number(hunk[1]);
            newLine = Number(hunk[2]);
          } else if (text.startsWith("+") && !text.startsWith("+++")) {
            kind = "added";
            next = newLine++;
          } else if (text.startsWith("-") && !text.startsWith("---")) {
            kind = "removed";
            old = oldLine++;
          } else if (text.startsWith(" ")) {
            kind = "context";
            old = oldLine++;
            next = newLine++;
          }
          const focused = !!selectedLine && selectedLine === next;
          return (
            <div
              key={index}
              ref={focused ? highlight : undefined}
              className={`diff-line ${kind} ${focused ? "focused" : ""}`}
            >
              <span>{old ?? ""}</span>
              <span>{next ?? ""}</span>
              <code>{text || " "}</code>
            </div>
          );
        })}
      </div>
    </div>
  );
}
