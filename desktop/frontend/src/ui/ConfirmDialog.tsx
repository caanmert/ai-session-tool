import { useEffect, useId, useRef, type ReactNode } from "react";

/**
 * Confirmation for an action that changes or ends work. It says what happens
 * and what is kept. The safe choice has the initial focus and Escape cancels;
 * the destructive button never gets default focus.
 */
export default function ConfirmDialog({
  title,
  children,
  confirmLabel,
  cancelLabel,
  busy = false,
  onConfirm,
  onCancel,
}: {
  title: string;
  children: ReactNode;
  confirmLabel: string;
  cancelLabel: string;
  busy?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const cancel = useRef<HTMLButtonElement>(null);
  const id = useId();
  useEffect(() => {
    const el = dialog.current;
    if (el?.showModal) el.showModal();
    cancel.current?.focus();
    return () => el?.close?.();
  }, []);
  return (
    <dialog
      ref={dialog}
      className="confirm-dialog"
      aria-labelledby={id}
      onClose={onCancel}
      onCancel={(e) => {
        e.preventDefault();
        if (!busy) onCancel();
      }}
    >
      <h2 id={id}>{title}</h2>
      <div className="confirm-body">{children}</div>
      <div className="confirm-actions">
        <button
          ref={cancel}
          className="button secondary"
          disabled={busy}
          onClick={onCancel}
        >
          {cancelLabel}
        </button>
        <button className="button danger" disabled={busy} onClick={onConfirm}>
          {confirmLabel}
        </button>
      </div>
    </dialog>
  );
}
