import { useEffect, useRef, useState } from "react";
import DiffViewer from "../DiffViewer";
import Icon from "../Icon";
import { errorText, type ReviewSnapshot } from "../api";
import type { LineComment } from "./lineComments";

/**
 * A read-only diff in a dialog: the uncommitted changes, or what changed
 * since a message. Line comments can be added when onComment is given.
 */
export default function ChangesDialog({
  title,
  note,
  load,
  file,
  onFile,
  onClose,
  empty = "No changes.",
  comments,
  onComment,
  onRemoveComment,
}: {
  title: string;
  note: string;
  load: () => Promise<ReviewSnapshot>;
  file: string;
  onFile: (path: string) => void;
  onClose: () => void;
  empty?: string;
  comments?: LineComment[];
  onComment?: (comment: Omit<LineComment, "id">) => void;
  onRemoveComment?: (id: string) => void;
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
    load()
      .then((next) => active && setSnapshot(next))
      .catch((e) => active && setError(errorText(e)));
    return () => {
      active = false;
    };
    // Read once when the dialog opens.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
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
          <h2 id="changes-dialog-title">{title}</h2>
          <p title={note}>{note}</p>
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
            comments={comments}
            onComment={onComment}
            onRemoveComment={onRemoveComment}
          />
        ) : (
          <p className="details-empty">{empty}</p>
        ))}
      {snapshot?.warnings.map((warning) => (
        <p className="details-note" key={warning}>
          {warning}
        </p>
      ))}
    </dialog>
  );
}
