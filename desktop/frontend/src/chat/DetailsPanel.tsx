import { useEffect, useRef, useState } from "react";
import DiffViewer from "../DiffViewer";
import Icon from "../Icon";
import {
  api,
  errorText,
  type ChangeSummary,
  type ChatState,
  type ReviewSnapshot,
  type Session,
} from "../api";
import { sessionCost, sessionTokens } from "../ui/format";
import { conversationMarkdown } from "./conversation";
import { CommitDialog, PullRequestDialog } from "./GitDialogs";
import { busyChat } from "../shell/OpenSessions";
import NewIssueDialog from "../github/NewIssueDialog";

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

function ChangesDialog({
  chat,
  file,
  onFile,
  onClose,
}: {
  chat: ChatState;
  file: string;
  onFile: (path: string) => void;
  onClose: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [snapshot, setSnapshot] = useState<ReviewSnapshot>();
  const [error, setError] = useState("");
  useEffect(() => {
    const el = dialog.current;
    if (el?.showModal) el.showModal();
    return () => el?.close?.();
  }, []);
  useEffect(() => {
    let active = true;
    api
      .ChatDiff(chat.id)
      .then((next) => active && setSnapshot(next))
      .catch((e) => active && setError(errorText(e)));
    return () => {
      active = false;
    };
  }, [chat.id]);
  return (
    <dialog
      ref={dialog}
      className="changes-dialog"
      aria-labelledby="changes-dialog-title"
      onClose={onClose}
      onCancel={onClose}
    >
      <header>
        <div>
          <h2 id="changes-dialog-title">Uncommitted changes</h2>
          <p title={chat.project}>
            {chat.project} · compared with the last commit · read-only
          </p>
        </div>
        <button
          className="icon-button"
          aria-label="Close changes"
          onClick={onClose}
        >
          <Icon name="close" size={16} />
        </button>
      </header>
      {error && (
        <p className="notice error" role="alert">
          {error}
        </p>
      )}
      {!snapshot && !error && <p className="details-empty">Reading changes…</p>}
      {snapshot &&
        (snapshot.files.length ? (
          <DiffViewer
            snapshot={snapshot}
            selectedFile={file}
            selectedLine={0}
            onFile={onFile}
          />
        ) : (
          <p className="details-empty">No uncommitted changes.</p>
        ))}
      {snapshot?.warnings.map((warning) => (
        <p className="details-note" key={warning}>
          {warning}
        </p>
      ))}
    </dialog>
  );
}

export default function DetailsPanel({
  chat,
  session,
  summary,
  error,
  drawer,
  onRefresh,
  onClose,
}: {
  chat: ChatState;
  session?: Session;
  summary?: ChangeSummary;
  error: string;
  /** Shown over the conversation at narrow widths, with a close button. */
  drawer: boolean;
  onRefresh: () => void;
  onClose: () => void;
}) {
  const [review, setReview] = useState<string>();
  const [dialog, setDialog] = useState<"commit" | "pr" | "issue">();
  const [done, setDone] = useState("");
  const busy = busyChat(chat.status);
  const [revealError, setRevealError] = useState("");
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
              Compared with the last commit. Nothing is committed or pushed
              until you confirm it.
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
          chat={chat}
          file={review}
          onFile={setReview}
          onClose={() => setReview(undefined)}
        />
      )}
    </>
  );
}
