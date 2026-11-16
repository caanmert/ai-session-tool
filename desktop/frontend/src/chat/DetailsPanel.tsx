import { useEffect, useRef, useState } from "react";
import Icon from "../Icon";
import ConfirmDialog from "../ui/ConfirmDialog";
import {
  api,
  errorText,
  type ChangeSummary,
  type ChatState,
  type ReviewSnapshot,
  type Session,
  type ToolName,
  toolLabel,
} from "../api";
import { sessionCost, sessionTokens } from "../ui/format";
import { conversationMarkdown } from "./conversation";
import { CommitDialog, PullRequestDialog } from "./GitDialogs";
import { busyChat } from "../shell/OpenSessions";
import NewIssueDialog from "../github/NewIssueDialog";
import InstructionsDialog from "./InstructionsDialog";
import RunPanel from "./RunPanel";
import ChangesDialog from "./ChangesDialog";
import CopyPanel from "./CopyPanel";
import {
  commentPlace,
  commentsMessage,
  isStale,
  useLineComments,
} from "./lineComments";
import { contextFill } from "./ContextMeter";

const usageBreakdown = (session: Session) =>
  [
    ["Input", session.usage.input],
    ["output", session.usage.output],
    ["cache read", session.usage.cacheRead],
    ["cache write", session.usage.cacheWrite],
  ]
    .map(([name, count]) => name + " " + count.toLocaleString())
    .join(" · ");

const statusName: Record<string, string> = {
  M: "Modified",
  A: "Added",
  D: "Deleted",
  "?": "New",
};

/**
 * Uncommitted changes in the conversation's project. Read-only: nothing here
 * stages, commits or reverts. The list refreshes when the conversation
 * changes, at most once every two seconds.
 */
/** A starting description for an issue: the agent's latest reply. */
export function issueStart(chat: ChatState) {
  for (let i = chat.messages.length - 1; i >= 0; i--) {
    const message = chat.messages[i];
    if (message.role === "assistant" && message.kind === "text" && message.text)
      return message.text.slice(0, 8000);
  }
  return "";
}

export function useChanges(chat: ChatState, enabled: boolean) {
  const [summary, setSummary] = useState<ChangeSummary>();
  const [error, setError] = useState("");
  const [manual, setManual] = useState(0);
  const last = useRef(0);
  useEffect(() => {
    if (!enabled) return;
    let active = true;
    const wait = Math.max(0, last.current + 2000 - Date.now());
    const timer = setTimeout(() => {
      last.current = Date.now();
      api
        .ChatChanges(chat.id)
        .then((next) => {
          if (!active) return;
          setSummary(next);
          setError("");
        })
        .catch((e) => {
          if (active) setError(errorText(e));
        });
    }, wait);
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [chat.id, chat.version, enabled, manual]);
  return { summary, error, refresh: () => setManual((n) => n + 1) };
}

export default function DetailsPanel({
  chat,
  session,
  summary,
  error,
  drawer,
  onRefresh,
  onClose,
  handOff,
  onDraft,
  onCopyRemoved,
}: {
  chat: ChatState;
  session?: Session;
  summary?: ChangeSummary;
  error: string;
  /** Shown over the conversation at narrow widths, with a close button. */
  drawer: boolean;
  onRefresh: () => void;
  onClose: () => void;
  /** Continue this conversation with the other agent. */
  handOff?: { to: ToolName; blocked: string; run: () => Promise<void> };
  /** Puts text in the message box, after anything already there. */
  onDraft?: (text: string) => void;
  /** The conversation closed because its copy was removed. */
  onCopyRemoved?: () => void;
}) {
  const [review, setReview] = useState<string>();
  const [dialog, setDialog] = useState<
    "commit" | "pr" | "issue" | "instructions"
  >();
  // The file whose changes are about to be discarded, after a confirmation.
  const [discarding, setDiscarding] = useState("");
  const [discardBusy, setDiscardBusy] = useState(false);
  const [discardError, setDiscardError] = useState("");
  async function discard() {
    setDiscardBusy(true);
    setDiscardError("");
    try {
      await api.DiscardFile(chat.id, discarding);
      setDone(`Discarded the changes to ${discarding}`);
      setDiscarding("");
      onRefresh();
    } catch (e) {
      setDiscardError(errorText(e));
    } finally {
      setDiscardBusy(false);
    }
  }
  const [done, setDone] = useState("");
  const busy = busyChat(chat.status);
  const context = contextFill(chat.context);
  const [revealError, setRevealError] = useState("");
  const [handingOff, setHandingOff] = useState(false);
  // Comments on changed lines, and the diff they are checked against.
  const lineComments = useLineComments(chat.id);
  const [editing, setEditing] = useState<{ id: string; note: string }>();
  const [current, setCurrent] = useState<ReviewSnapshot>();
  const commentCount = lineComments.comments.length;
  useEffect(() => {
    if (!commentCount || chat.review) return;
    let active = true;
    api
      .ChatDiff(chat.id)
      .then((next) => active && setCurrent(next))
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [chat.id, chat.review, summary, commentCount]);
  const stale = new Set(
    lineComments.comments.filter((c) => isStale(c, current)).map((c) => c.id),
  );
  function sendComments() {
    if (!onDraft || !commentCount) return;
    onDraft(commentsMessage(lineComments.comments, stale));
    lineComments.clear();
    setEditing(undefined);
    setDone(
      "Your comments are in the message box. Nothing is sent until you send it.",
    );
    if (drawer) onClose();
  }
  const [copied, setCopied] = useState("");
  useEffect(() => {
    if (!copied) return;
    const timer = setTimeout(() => setCopied(""), 1600);
    return () => clearTimeout(timer);
  }, [copied]);
  const close = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (drawer) close.current?.focus();
  }, [drawer]);
  const files = summary?.files || [];
  return (
    <>
      {drawer && (
        // The scrim covers only the conversation; Escape and Close also dismiss.
        // eslint-disable-next-line jsx-a11y/no-static-element-interactions
        <div className="details-scrim" onMouseDown={onClose} />
      )}
      <aside
        className={`chat-details ${drawer ? "drawer" : ""}`}
        aria-label="Conversation details"
        onKeyDown={(e) => {
          if (drawer && e.key === "Escape") {
            e.stopPropagation();
            onClose();
          }
        }}
      >
        <header>
          <h3>Changes</h3>
          {summary?.available && files.length > 0 && (
            <span className="details-totals">
              {files.length} {files.length === 1 ? "file" : "files"}{" "}
              <b className="added">+{summary.added}</b>{" "}
              <b className="removed">−{summary.removed}</b>
            </span>
          )}
          <button
            className="icon-button"
            aria-label="Refresh changes"
            title="Refresh changes"
            onClick={onRefresh}
          >
            <Icon name="refresh" size={14} />
          </button>
          {drawer && (
            <button
              ref={close}
              className="icon-button"
              aria-label="Close details"
              onClick={onClose}
            >
              <Icon name="close" size={15} />
            </button>
          )}
        </header>
        {error && (
          <p className="details-note error" role="alert">
            {error}
          </p>
        )}
        {!summary && !error && (
          <p className="details-empty">Reading changes…</p>
        )}
        {summary && !summary.available && (
          <p className="details-empty">{summary.reason}</p>
        )}
        {summary?.available && !files.length && (
          <p className="details-empty">
            No uncommitted changes in this project.
          </p>
        )}
        {files.length > 0 && (
          <>
            <ul className="changed-files">
              {files.map((file) => (
                <li key={file.path}>
                  <button
                    title={`${statusName[file.status] || file.status}: ${file.path}`}
                    onClick={() => setReview(file.path)}
                  >
                    <span
                      className={`change-status s-${file.status === "?" ? "new" : file.status}`}
                      aria-label={statusName[file.status] || file.status}
                    >
                      {file.status === "?" ? "A" : file.status}
                    </span>
                    <span className="change-path">{file.path}</span>
                    {file.binary ? (
                      <small>binary</small>
                    ) : (
                      <small>
                        {file.added > 0 && (
                          <b className="added">+{file.added}</b>
                        )}{" "}
                        {file.removed > 0 && (
                          <b className="removed">−{file.removed}</b>
                        )}
                      </small>
                    )}
                  </button>
                  {!chat.review && (
                    <button
                      className="discard-file"
                      aria-label={`Discard changes to ${file.path}`}
                      title={
                        busy
                          ? "Wait for the response to finish"
                          : "Discard the uncommitted changes to this file"
                      }
                      disabled={busy}
                      onClick={() => setDiscarding(file.path)}
                    >
                      <Icon name="undo" size={13} />
                    </button>
                  )}
                </li>
              ))}
            </ul>
            {summary?.truncated && (
              <p className="details-note">
                Showing the first {files.length} files.
              </p>
            )}
            <button
              className="button secondary review-all"
              onClick={() => setReview("")}
            >
              Review all changes
            </button>
            <p className="details-note">
              Compared with the last commit. Click a line number in a diff to
              comment on it. Nothing is committed or pushed until you confirm
              it.
            </p>
          </>
        )}
        {summary?.available && !chat.review && (
          <div className="git-actions">
            <button
              className="button secondary"
              disabled={!files.length || busy}
              title={
                busy
                  ? "Wait for the agent to finish before committing"
                  : files.length
                    ? "Commit chosen files locally"
                    : "No uncommitted changes"
              }
              onClick={() => setDialog("commit")}
            >
              Commit…
            </button>
            <button
              className="button secondary"
              title="Push this branch and open a pull request on GitHub"
              onClick={() => setDialog("pr")}
            >
              Pull request…
            </button>
          </div>
        )}
        {done && (
          <p className="details-note done" role="status">
            {done}
          </p>
        )}
        {chat.copy && (
          <CopyPanel
            chat={chat}
            busy={busy}
            onDone={(text) => {
              setDone(text);
              onRefresh();
            }}
            onRemoved={() => onCopyRemoved?.()}
          />
        )}
        {commentCount > 0 && !chat.review && (
          <section className="line-comments" aria-label="Comments on changes">
            <h3 className="details-section">
              Comments <b className="count">{commentCount}</b>
            </h3>
            <ul>
              {lineComments.comments.map((c) => (
                <li key={c.id}>
                  <button
                    className="comment-place"
                    title="Show these lines"
                    onClick={() => setReview(c.path)}
                  >
                    {commentPlace(c)}
                  </button>
                  {stale.has(c.id) && (
                    <span
                      className="comment-stale"
                      title="These lines changed since you commented. The comment can still be sent."
                    >
                      out of date
                    </span>
                  )}
                  {editing?.id === c.id ? (
                    <>
                      <textarea
                        aria-label={`Edit the comment on ${commentPlace(c)}`}
                        value={editing.note}
                        onChange={(e) =>
                          setEditing({ id: c.id, note: e.target.value })
                        }
                      />
                      <span className="comment-actions">
                        <button
                          className="text-button"
                          onClick={() => setEditing(undefined)}
                        >
                          Cancel
                        </button>
                        <button
                          className="text-button"
                          disabled={!editing.note.trim()}
                          onClick={() => {
                            lineComments.update(c.id, editing.note.trim());
                            setEditing(undefined);
                          }}
                        >
                          Save
                        </button>
                      </span>
                    </>
                  ) : (
                    <>
                      <p>{c.note}</p>
                      <span className="comment-actions">
                        <button
                          className="text-button"
                          aria-label={`Edit the comment on ${commentPlace(c)}`}
                          onClick={() => setEditing({ id: c.id, note: c.note })}
                        >
                          Edit
                        </button>
                        <button
                          className="text-button"
                          aria-label={`Remove the comment on ${commentPlace(c)}`}
                          onClick={() => lineComments.remove(c.id)}
                        >
                          Remove
                        </button>
                      </span>
                    </>
                  )}
                </li>
              ))}
            </ul>
            <button
              className="button primary"
              disabled={!onDraft}
              onClick={sendComments}
            >
              Send to agent
            </button>
            <p className="details-note">
              Puts the comments in the message box to edit. Nothing is sent
              until you send it.
            </p>
          </section>
        )}
        {!chat.review && <RunPanel chatId={chat.id} onDraft={onDraft} />}
        <h3 className="details-section">Session</h3>
        <dl className="details-facts">
          <div>
            <dt>Model</dt>
            <dd>
              {chat.activeModel || chat.settings?.model || "Agent default"}
            </dd>
          </div>
          {session && (
            <>
              <div>
                <dt>Turns</dt>
                <dd>
                  {session.userTurns} prompts · {session.assistantTurns} replies
                </dd>
              </div>
              <div title={usageBreakdown(session)}>
                <dt>Tokens</dt>
                <dd>{sessionTokens(session)}</dd>
              </div>
              {sessionCost(session) && (
                <div title="What this usage would cost at API list prices. A subscription is not billed this way.">
                  <dt>At API prices</dt>
                  <dd>{sessionCost(session)}</dd>
                </div>
              )}
            </>
          )}
          {context && (
            <div title={`${context.tokens} after the latest response`}>
              <dt>Context</dt>
              <dd className={`context-fact ${context.level}`}>
                <span className="usage-meter" aria-hidden="true">
                  <i style={{ width: `${context.share * 100}%` }} />
                </span>
                {context.percent}% used
              </dd>
            </div>
          )}
          <div>
            <dt>Project</dt>
            <dd title={chat.project}>{chat.project}</dd>
          </div>
        </dl>
        <button
          className="button secondary reveal"
          onClick={() => {
            setRevealError("");
            api
              .RevealProject(chat.id)
              .catch((e) => setRevealError(errorText(e)));
          }}
        >
          <Icon name="folder" size={14} /> Show in Finder
        </button>
        <button
          className="button secondary reveal"
          title="Copies messages and the names of tool steps, without tool output"
          onClick={() => {
            navigator.clipboard
              .writeText(conversationMarkdown(chat))
              .then(() => setCopied("Copied"))
              .catch(() => setCopied("Copy failed"));
          }}
        >
          <Icon name="copy" size={14} />
          {copied ? <span role="status">{copied}</span> : "Copy as Markdown"}
        </button>
        <button
          className="button secondary reveal"
          title="Write a GitHub issue, starting from this conversation"
          onClick={() => setDialog("issue")}
        >
          <Icon name="issue" size={14} /> File an issue…
        </button>
        {handOff && (
          <button
            className="button secondary reveal"
            disabled={!!handOff.blocked || handingOff}
            title={
              handOff.blocked ||
              `Open a new ${toolLabel(handOff.to)} conversation with a summary of this one to edit and send`
            }
            onClick={() => {
              setHandingOff(true);
              setRevealError("");
              handOff
                .run()
                .catch((e) => setRevealError(errorText(e)))
                .finally(() => setHandingOff(false));
            }}
          >
            <Icon name="message" size={14} />
            {handingOff ? "Opening…" : `Continue with ${toolLabel(handOff.to)}`}
          </button>
        )}
        <button
          className="button secondary reveal"
          title="The CLAUDE.md and AGENTS.md files the agents read for this project"
          onClick={() => setDialog("instructions")}
        >
          <Icon name="file" size={14} /> Project instructions…
        </button>
        {revealError && (
          <p className="details-note error" role="alert">
            {revealError}
          </p>
        )}
      </aside>
      {dialog === "commit" && (
        <CommitDialog
          chatId={chat.id}
          onClose={() => setDialog(undefined)}
          onCommitted={(result) => {
            setDialog(undefined);
            setDone(
              `Committed ${result.commit} on ${result.branch}: ${result.subject}`,
            );
            onRefresh();
          }}
        />
      )}
      {dialog === "pr" && (
        <PullRequestDialog
          chatId={chat.id}
          onClose={() => {
            setDialog(undefined);
            onRefresh();
          }}
        />
      )}
      {discarding && (
        <ConfirmDialog
          title={`Discard changes to ${discarding}?`}
          confirmLabel="Discard changes"
          cancelLabel="Keep them"
          busy={discardBusy}
          onCancel={() => {
            setDiscarding("");
            setDiscardError("");
          }}
          onConfirm={() => void discard()}
        >
          <p>
            The file goes back to the last commit; a file that is new since then
            is removed. The current version is kept first, so you can undo this
            from the note it leaves in the conversation.
          </p>
          {discardError && <p role="alert">{discardError}</p>}
        </ConfirmDialog>
      )}
      {dialog === "instructions" && (
        <InstructionsDialog
          project={chat.project}
          onClose={() => setDialog(undefined)}
        />
      )}
      {dialog === "issue" && (
        <NewIssueDialog
          project={chat.project}
          initialTitle={chat.title}
          initialBody={issueStart(chat)}
          onClose={() => setDialog(undefined)}
          onCreated={(issue) => setDone(`Created issue #${issue.number}`)}
        />
      )}
      {review !== undefined && (
        <ChangesDialog
          title="Uncommitted changes"
          note={`${chat.project} · compared with the last commit · read-only; click a line number to comment`}
          load={() => api.ChatDiff(chat.id)}
          empty="No uncommitted changes."
          file={review}
          onFile={setReview}
          onClose={() => setReview(undefined)}
          comments={chat.review ? undefined : lineComments.comments}
          onComment={chat.review ? undefined : lineComments.add}
          onRemoveComment={lineComments.remove}
        />
      )}
    </>
  );
}
