import { useEffect, useRef, useState } from "react";
import AgentMark from "../AgentMark";
import {
  api,
  errorText,
  toolLabel,
  type InstructionFile,
  type InstructionText,
  type ToolName,
} from "../api";
import { Dialog } from "./GitDialogs";

const size = (bytes: number) =>
  bytes < 1024 ? `${bytes} B` : `${(bytes / 1024).toFixed(1)} KB`;

/**
 * The instructions each agent reads for a project. Files in the project can
 * be edited and saved; a save is refused if the file changed on disk since it
 * was opened. User-level files are shown and opened in Finder.
 */
export default function InstructionsDialog({
  project,
  onClose,
}: {
  project: string;
  onClose: () => void;
}) {
  const [files, setFiles] = useState<InstructionFile[]>();
  const [chosen, setChosen] = useState("");
  const [open, setOpen] = useState<InstructionText>();
  const [text, setText] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState("");
  // A choice that would drop unsaved edits waits for a confirmation.
  const [leaving, setLeaving] = useState<(() => void) | null>(null);
  const editor = useRef<HTMLTextAreaElement>(null);
  const dirty = !!open && text !== open.text;
  function list() {
    return api
      .ProjectInstructions(project)
      .then((found) => {
        setFiles(found);
        return found;
      })
      .catch((e) => {
        setError(errorText(e));
        return [] as InstructionFile[];
      });
  }
  useEffect(() => {
    void list().then((found) => {
      const first =
        found.find((f) => f.scope === "project" && f.exists) ||
        found.find((f) => f.scope === "project");
      if (first) void choose(first.name);
    });
    // Only when the dialog opens.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [project]);
  async function choose(name: string) {
    setChosen(name);
    setSaved("");
    setError("");
    const file = files?.find((f) => f.name === name);
    if (file && file.scope !== "project") {
      setOpen(undefined);
      return;
    }
    try {
      const next = await api.ReadInstruction(project, name);
      setOpen(next);
      setText(next.text);
      setTimeout(() => editor.current?.focus());
    } catch (e) {
      setOpen(undefined);
      setError(errorText(e));
    }
  }
  const guard = (then: () => void) => (dirty ? setLeaving(() => then) : then());
  async function save() {
    if (!open || !dirty) return;
    setBusy(true);
    setError("");
    try {
      const next = await api.WriteInstruction(
        project,
        open.name,
        text,
        open.version,
      );
      setOpen(next);
      setText(next.text);
      setSaved(`Saved ${next.name}. The agent reads it from its next message.`);
      void list();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  const selected = files?.find((f) => f.name === chosen);
  const agents: ToolName[] = ["claude", "codex"];
  return (
    <Dialog
      title="Project instructions"
      className="instructions-dialog"
      busy={busy || dirty}
      onClose={() => guard(onClose)}
    >
      {() => (
        <>
          <p className="git-target">
            The files each agent reads for standing guidance in{" "}
            <code>{project}</code>.
          </p>
          <div className="instructions-body">
            <nav className="instructions-files" aria-label="Instructions files">
              {agents.map((agent) => (
                <section key={agent}>
                  <h3>
                    <AgentMark tool={agent} dot /> {toolLabel(agent)}
                  </h3>
                  {(files || [])
                    .filter((f) => f.agent === agent)
                    .map((file) => (
                      <button
                        key={file.scope + file.name}
                        aria-pressed={chosen === file.name}
                        onClick={() => guard(() => void choose(file.name))}
                      >
                        <code>{file.name}</code>
                        <small>
                          {file.scope === "user" ? "you · " : ""}
                          {file.exists ? size(file.size) : "not created"}
                        </small>
                      </button>
                    ))}
                </section>
              ))}
            </nav>
            <div className="instructions-editor">
              {!files && <p className="confirm-body">Reading…</p>}
              {selected?.scope === "user" && (
                <div className="instructions-user">
                  <p>
                    <code>{selected.path}</code>
                  </p>
                  <p>
                    Your own instructions for{" "}
                    {toolLabel(selected.agent as ToolName)} in every project.
                    They are {selected.exists ? "" : "not created yet and are "}
                    edited outside ais.
                  </p>
                  <button
                    className="button secondary"
                    onClick={() =>
                      void api
                        .RevealInstruction(project, selected.name)
                        .catch((e) => setError(errorText(e)))
                    }
                  >
                    Show in Finder
                  </button>
                </div>
              )}
              {open && selected?.scope === "project" && (
                <>
                  <label className="git-field">
                    <span>
                      {open.name}
                      {!open.exists && " · new file, created when you save"}
                    </span>
                    <textarea
                      ref={editor}
                      aria-label={`Contents of ${open.name}`}
                      value={text}
                      disabled={busy}
                      spellCheck={false}
                      onChange={(e) => {
                        setText(e.target.value);
                        setSaved("");
                      }}
                      onKeyDown={(e) => {
                        if (e.key === "s" && (e.metaKey || e.ctrlKey)) {
                          e.preventDefault();
                          void save();
                        }
                      }}
                    />
                  </label>
                </>
              )}
            </div>
          </div>
          {leaving && (
            <p className="instructions-leave" role="alert">
              You have unsaved changes to {open?.name}.{" "}
              <button
                className="button secondary"
                onClick={() => setLeaving(null)}
              >
                Keep editing
              </button>{" "}
              <button
                className="button danger"
                onClick={() => {
                  const then = leaving;
                  setLeaving(null);
                  if (open) setText(open.text);
                  then();
                }}
              >
                Discard changes
              </button>
            </p>
          )}
          {saved && (
            <p className="details-note done" role="status">
              {saved}
            </p>
          )}
          {error && <p role="alert">{error}</p>}
          <div className="confirm-actions">
            <button
              className="button secondary"
              disabled={busy}
              onClick={() => guard(onClose)}
            >
              Close
            </button>
            {open && selected?.scope === "project" && (
              <button
                className="button primary"
                disabled={!dirty || busy}
                onClick={() => void save()}
              >
                {busy ? "Saving…" : "Save"}
                {!busy && <kbd>⌘S</kbd>}
              </button>
            )}
          </div>
        </>
      )}
    </Dialog>
  );
}
