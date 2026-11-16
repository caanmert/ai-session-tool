import { useState } from "react";
import Icon from "../Icon";
import ConfirmDialog from "../ui/ConfirmDialog";
import { api, errorText, type ChatState } from "../api";
import ChangesDialog from "./ChangesDialog";

/**
 * A conversation that works in its own copy of the project: what the copy
 * is, its changes, bringing them back into the project, and removing it.
 * Nothing reaches the project until the user brings changes back.
 */
export default function CopyPanel({
  chat,
  busy,
  onDone,
  onRemoved,
}: {
  chat: ChatState;
  busy: boolean;
  onDone: (text: string) => void;
  onRemoved: () => void;
}) {
  const copy = chat.copy!;
  const [viewing, setViewing] = useState<string>();
  const [bringing, setBringing] = useState<string[]>();
  const [removing, setRemoving] = useState<"ask" | "discard">();
  const [working, setWorking] = useState(false);
  const [error, setError] = useState("");
  const [unsaved, setUnsaved] = useState("");
  async function planBringBack() {
    setError("");
    try {
      const changes = await api.CopyChanges(chat.id);
      if (!changes.files.length) setError("The copy has no changes yet.");
      else setBringing(changes.files.map((f) => f.path));
    } catch (e) {
      setError(errorText(e));
    }
  }
  async function bringBack() {
    setWorking(true);
    try {
      const files = await api.BringBackCopy(chat.id);
      setBringing(undefined);
      onDone(
        `Brought ${files.length} ${files.length === 1 ? "file" : "files"} back into the project, uncommitted.`,
      );
    } catch (e) {
      setBringing(undefined);
      setError(errorText(e));
    } finally {
      setWorking(false);
    }
  }
  async function remove(discard: boolean) {
    setWorking(true);
    try {
      await api.RemoveCopy(chat.id, discard);
      setRemoving(undefined);
      onRemoved();
    } catch (e) {
      const text = errorText(e);
      if (!discard && text.includes("not brought back")) {
        setUnsaved(text);
        setRemoving("discard");
      } else {
        setRemoving(undefined);
        setError(text);
      }
    } finally {
      setWorking(false);
    }
  }
  const waitTitle = busy ? "Wait for the agent to finish first" : undefined;
  return (
    <section className="copy-panel" aria-label="Separate copy">
      <h3 className="details-section">Separate copy</h3>
      <p className="details-note">
        This conversation works in its own copy of{" "}
        <code title={copy.project}>{copy.project}</code>, on branch{" "}
        <code>{copy.branch}</code> from <code>{copy.base.slice(0, 8)}</code>.
        Your project is not changed until you bring the changes back.
      </p>
      <div className="copy-actions">
        <button
          className="button secondary"
          onClick={() => setViewing("")}
          title="Everything the copy changed since it started, committed or not"
        >
          <Icon name="code" size={14} /> Copy's changes
        </button>
        <button
          className="button primary"
          disabled={busy}
          title={waitTitle}
          onClick={() => void planBringBack()}
        >
          Bring changes back…
        </button>
        <button
          className="button secondary"
          disabled={busy}
          title={waitTitle}
          onClick={() => setRemoving("ask")}
        >
          Remove copy…
        </button>
      </div>
      {error && (
        <p className="details-note error" role="alert">
          {error}
        </p>
      )}
      {viewing !== undefined && (
        <ChangesDialog
          title="Changes in this copy"
          note={`${copy.branch} · since ${copy.base.slice(0, 8)}, committed or not · read-only`}
          load={() => api.CopyChanges(chat.id)}
          empty="The copy has no changes yet."
          file={viewing}
          onFile={setViewing}
          onClose={() => setViewing(undefined)}
        />
      )}
      {bringing && (
        <ConfirmDialog
          title={`Bring ${bringing.length} ${bringing.length === 1 ? "file" : "files"} back into the project?`}
          confirmLabel="Bring them back"
          cancelLabel="Not now"
          busy={working}
          onCancel={() => setBringing(undefined)}
          onConfirm={() => void bringBack()}
        >
          <p>
            The copy's changes are applied to <code>{copy.project}</code> as
            uncommitted changes, all or none. Files you have changed yourself
            are never overwritten. Nothing is committed.
          </p>
          <ul className="restore-list">
            {bringing.slice(0, 12).map((path) => (
              <li key={path}>
                <code>{path}</code>
              </li>
            ))}
            {bringing.length > 12 && <li>and {bringing.length - 12} more</li>}
          </ul>
        </ConfirmDialog>
      )}
      {removing === "ask" && (
        <ConfirmDialog
          title="Remove this copy?"
          confirmLabel="Remove copy"
          cancelLabel="Keep it"
          busy={working}
          onCancel={() => setRemoving(undefined)}
          onConfirm={() => void remove(false)}
        >
          <p>
            Deletes the copy's folder and its branch <code>{copy.branch}</code>,
            and closes this conversation. Your project is not touched. If the
            copy has changes you have not brought back, you are asked first.
          </p>
        </ConfirmDialog>
      )}
      {removing === "discard" && (
        <ConfirmDialog
          title="Discard the copy's changes?"
          confirmLabel="Discard and remove"
          cancelLabel="Keep the copy"
          busy={working}
          onCancel={() => setRemoving(undefined)}
          onConfirm={() => void remove(true)}
        >
          <p>{unsaved.charAt(0).toUpperCase() + unsaved.slice(1)}.</p>
          <p>Removing the copy now loses them for good.</p>
        </ConfirmDialog>
      )}
    </section>
  );
}
