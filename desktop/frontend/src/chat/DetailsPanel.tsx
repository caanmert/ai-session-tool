import { useEffect, useRef, useState } from "react";
import DiffViewer from "../DiffViewer";
import Icon from "../Icon";
import {
  api,
  errorText,
  type ChangeSummary,
  type ChatState,
  type ReviewSnapshot,
} from "../api";

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
  summary,
  error,
  drawer,
  onRefresh,
  onClose,
}: {
  chat: ChatState;
  summary?: ChangeSummary;
  error: string;
  /** Shown over the conversation at narrow widths, with a close button. */
  drawer: boolean;
  onRefresh: () => void;
  onClose: () => void;
}) {
  const [review, setReview] = useState<string>();
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
              Compared with the last commit. Nothing is committed or pushed from
              here.
            </p>
          </>
        )}
        <h3 className="details-section">Session</h3>
        <dl className="details-facts">
          <div>
            <dt>Model</dt>
            <dd>
              {chat.activeModel || chat.settings?.model || "Agent default"}
            </dd>
          </div>
          <div>
            <dt>Project</dt>
            <dd title={chat.project}>{chat.project}</dd>
          </div>
        </dl>
      </aside>
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
