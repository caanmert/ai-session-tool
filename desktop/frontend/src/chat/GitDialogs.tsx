import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import {
  api,
  errorText,
  type CommitPlan,
  type CommitResult,
  type PullRequestPlan,
  type PullRequestResult,
} from "../api";

/** A modal dialog: Escape and Cancel close it, and focus starts inside. */
export function Dialog({
  title,
  className,
  busy,
  onClose,
  children,
}: {
  title: string;
  className: string;
  busy: boolean;
  onClose: () => void;
  children: (titleId: string) => ReactNode;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const id = useId();
  useEffect(() => {
    const el = dialog.current;
    if (el?.showModal) el.showModal();
    return () => el?.close?.();
  }, []);
  return (
    <dialog
      ref={dialog}
      className={`confirm-dialog git-dialog ${className}`}
      aria-labelledby={id}
      onClose={onClose}
      onCancel={(e) => {
        e.preventDefault();
        if (!busy) onClose();
      }}
    >
      <h2 id={id}>{title}</h2>
      {children(id)}
    </dialog>
  );
}

const statusName: Record<string, string> = {
  M: "Modified",
  A: "Added",
  D: "Deleted",
  "?": "New",
};

/**
 * Commit chosen files. Only a local commit is made: nothing is pushed or
 * merged. Enter adds a line to the message; Cmd+Enter commits.
 */
export function CommitDialog({
  chatId,
  onClose,
  onCommitted,
}: {
  chatId: string;
  onClose: () => void;
  onCommitted: (result: CommitResult) => void;
}) {
  const [plan, setPlan] = useState<CommitPlan>();
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [chosen, setChosen] = useState<Set<string>>(new Set());
  const [onNewBranch, setOnNewBranch] = useState(false);
  const [branch, setBranch] = useState("");
  const [busy, setBusy] = useState(false);
  const messageBox = useRef<HTMLTextAreaElement>(null);
  useEffect(() => {
    let active = true;
    api
      .CommitPlan(chatId)
      .then((next) => {
        if (!active) return;
        setPlan(next);
        setMessage(next.message);
        setChosen(new Set(next.files.map((file) => file.path)));
        setOnNewBranch(next.onDefault);
        setBranch(next.newBranch);
        setTimeout(() => messageBox.current?.focus());
      })
      .catch((e) => active && setError(errorText(e)));
    return () => {
      active = false;
    };
  }, [chatId]);
  const files = plan?.files || [];
  const selected = files.filter((file) => chosen.has(file.path));
  const added = selected.reduce((sum, file) => sum + file.added, 0);
  const removed = selected.reduce((sum, file) => sum + file.removed, 0);
  const ready =
    !!plan &&
    !busy &&
    !!message.trim() &&
    selected.length > 0 &&
    (!onNewBranch || !!branch.trim());
  async function commit() {
    if (!ready) return;
    setBusy(true);
    setError("");
    try {
      onCommitted(
        await api.Commit(chatId, {
          message,
          files: selected.map((file) => file.path),
          newBranch: onNewBranch ? branch.trim() : "",
        }),
      );
    } catch (e) {
      setError(errorText(e));
      setBusy(false);
    }
  }
  const toggle = (path: string) =>
    setChosen((current) => {
      const next = new Set(current);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  return (
    <Dialog
      title="Commit changes"
      className="commit-dialog"
      busy={busy}
      onClose={onClose}
    >
      {() => (
        <>
          {!plan && !error && <p className="confirm-body">Reading changes…</p>}
          {plan && (
            <>
              <p className="git-target">
                on <code>{onNewBranch ? branch || "…" : plan.branch}</code>
              </p>
              {plan.onDefault && (
                <div className="git-branch-choice">
                  <label className="setting-check">
                    <input
                      type="checkbox"
                      checked={onNewBranch}
                      disabled={busy}
                      onChange={(e) => setOnNewBranch(e.target.checked)}
                    />
                    <span>
                      Commit on a new branch
                      <small>
                        You are on {plan.branch}, the branch pull requests go
                        into.
                      </small>
                    </span>
                  </label>
                  {onNewBranch && (
                    <input
                      className="field"
                      aria-label="New branch name"
                      value={branch}
                      disabled={busy}
                      onChange={(e) => setBranch(e.target.value)}
                    />
                  )}
                </div>
              )}
              <label className="git-field">
                <span>Message</span>
                <textarea
                  ref={messageBox}
                  aria-label="Commit message"
                  value={message}
                  disabled={busy}
                  placeholder="Summary on the first line, details below"
                  onChange={(e) => setMessage(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                      e.preventDefault();
                      void commit();
                    }
                  }}
                />
              </label>
              <fieldset className="git-files">
                <legend>
                  <label>
                    <input
                      type="checkbox"
                      aria-label="All files"
                      checked={selected.length === files.length}
                      ref={(el) => {
                        if (el)
                          el.indeterminate =
                            selected.length > 0 &&
                            selected.length < files.length;
                      }}
                      disabled={busy}
                      onChange={(e) =>
                        setChosen(
                          new Set(
                            e.target.checked
                              ? files.map((file) => file.path)
                              : [],
                          ),
                        )
                      }
                    />
                    {selected.length} of {files.length} files
                  </label>
                  <span>
                    <b className="added">+{added}</b>{" "}
                    <b className="removed">−{removed}</b>
                  </span>
                </legend>
                <ul>
                  {files.map((file) => (
                    <li key={file.path}>
                      <label title={file.path}>
                        <input
                          type="checkbox"
                          checked={chosen.has(file.path)}
                          disabled={busy}
                          onChange={() => toggle(file.path)}
                        />
                        <span
                          className={`change-status s-${file.status === "?" ? "new" : file.status}`}
                          aria-label={statusName[file.status] || file.status}
                        >
                          {file.status === "?" ? "A" : file.status}
                        </span>
                        <span className="change-path">{file.path}</span>
                        <small>
                          {file.binary
                            ? "binary"
                            : [
                                file.added ? `+${file.added}` : "",
                                file.removed ? `−${file.removed}` : "",
                              ]
                                .filter(Boolean)
                                .join(" ")}
                        </small>
                      </label>
                    </li>
                  ))}
                </ul>
              </fieldset>
              {plan.truncated && (
                <p className="git-note">
                  Only the first {files.length} changed files are listed.
                </p>
              )}
              <p className="git-note">
                Creates a local commit only. Nothing is pushed or merged.
              </p>
            </>
          )}
          {error && <p role="alert">{error}</p>}
          <div className="confirm-actions">
            <button
              className="button secondary"
              disabled={busy}
              onClick={onClose}
            >
              Cancel
            </button>
            <button
              className="button primary"
              disabled={!ready}
              onClick={() => void commit()}
            >
              {busy
                ? "Committing…"
                : `Commit ${selected.length} ${selected.length === 1 ? "file" : "files"}`}
              {!busy && <kbd>⌘⏎</kbd>}
            </button>
          </div>
        </>
      )}
    </Dialog>
  );
}

/**
 * Push the branch and open a pull request with the GitHub CLI, or push to the
 * one already open. Nothing is merged, and nothing is force-pushed.
 */
export function PullRequestDialog({
  chatId,
  onClose,
}: {
  chatId: string;
  onClose: () => void;
}) {
  const [plan, setPlan] = useState<PullRequestPlan>();
  const [error, setError] = useState("");
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [draft, setDraft] = useState(false);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState<PullRequestResult>();
  const titleBox = useRef<HTMLInputElement>(null);
  useEffect(() => {
    let active = true;
    api
      .PullRequestPlan(chatId)
      .then((next) => {
        if (!active) return;
        setPlan(next);
        setTitle(next.title || "");
        setBody(next.body || "");
        setTimeout(() => titleBox.current?.focus());
      })
      .catch((e) => active && setError(errorText(e)));
    return () => {
      active = false;
    };
  }, [chatId]);
  const existing = plan?.existing;
  const upToDate = !!existing && plan?.unpushed === 0;
  const ready =
    !!plan &&
    !plan.reason &&
    !busy &&
    !upToDate &&
    (!!existing || !!title.trim());
  async function submit() {
    if (!ready) return;
    setBusy(true);
    setError("");
    try {
      setDone(await api.CreatePullRequest(chatId, { title, body, draft }));
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  const open = (link: string) =>
    api.OpenURL(link).catch((e) => setError(errorText(e)));
  const commits = (count: number) =>
    `${count} ${count === 1 ? "commit" : "commits"}`;
  const heading = done
    ? done.created
      ? `Pull request #${done.number} opened`
      : `Pushed to pull request #${done.number}`
    : existing
      ? `Pull request #${existing.number}`
      : "Open a pull request";
  return (
    <Dialog title={heading} className="pr-dialog" busy={busy} onClose={onClose}>
      {() => (
        <>
          {!plan && !error && (
            <p className="confirm-body">Checking the branch and GitHub…</p>
          )}
          {done && (
            <p className="confirm-body">
              {done.pushed > 0 && `${commits(done.pushed)} pushed. `}
              {done.url}
            </p>
          )}
          {plan?.reason && !done && (
            <p className="confirm-body" role="status">
              {plan.reason}
            </p>
          )}
          {plan && !plan.reason && !done && (
            <>
              <p className="confirm-body">
                {existing
                  ? upToDate
                    ? `${existing.title || "This pull request"} is up to date with ${plan.branch}.`
                    : `Pushes ${commits(plan.unpushed)} on ${plan.branch} to ${plan.remote}, updating this pull request on ${plan.repository}.`
                  : `Pushes ${plan.branch} to ${plan.remote} and opens a pull request into ${plan.base} on ${plan.repository}.`}{" "}
                Nothing is merged.
              </p>
              {!existing && (
                <>
                  <label className="git-field">
                    <span>Title</span>
                    <input
                      ref={titleBox}
                      className="field"
                      value={title}
                      disabled={busy}
                      onChange={(e) => setTitle(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                          e.preventDefault();
                          void submit();
                        }
                      }}
                    />
                  </label>
                  <label className="git-field">
                    <span>Description</span>
                    <textarea
                      value={body}
                      disabled={busy}
                      onChange={(e) => setBody(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                          e.preventDefault();
                          void submit();
                        }
                      }}
                    />
                  </label>
                  <label className="setting-check">
                    <input
                      type="checkbox"
                      checked={draft}
                      disabled={busy}
                      onChange={(e) => setDraft(e.target.checked)}
                    />
                    <span>Open as a draft</span>
                  </label>
                </>
              )}
              {plan.commits.length > 0 && (
                <details className="git-commits">
                  <summary>
                    {commits(plan.commits.length)} not on {plan.base}
                  </summary>
                  <ul>
                    {plan.commits.map((subject, index) => (
                      <li key={index}>{subject}</li>
                    ))}
                  </ul>
                </details>
              )}
              {plan.uncommitted > 0 && (
                <p className="git-note">
                  {plan.uncommitted} uncommitted{" "}
                  {plan.uncommitted === 1 ? "file is" : "files are"} not
                  included. Commit first to include{" "}
                  {plan.uncommitted === 1 ? "it" : "them"}.
                </p>
              )}
            </>
          )}
          {error && <p role="alert">{error}</p>}
          <div className="confirm-actions">
            {(done || existing) && (
              <button
                className="button secondary"
                onClick={() => void open(done?.url || existing!.url)}
              >
                Open on GitHub
              </button>
            )}
            <button
              className="button secondary"
              disabled={busy}
              onClick={onClose}
            >
              {done || plan?.reason || upToDate ? "Close" : "Cancel"}
            </button>
            {plan && !plan.reason && !done && !upToDate && (
              <button
                className="button primary"
                disabled={!ready}
                onClick={() => void submit()}
              >
                {busy
                  ? "Pushing…"
                  : existing
                    ? `Push ${commits(plan.unpushed)}`
                    : "Push and open pull request"}
              </button>
            )}
          </div>
        </>
      )}
    </Dialog>
  );
}
