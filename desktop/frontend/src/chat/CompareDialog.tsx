import { useEffect, useRef, useState } from "react";
import AgentMark from "../AgentMark";
import DiffViewer from "../DiffViewer";
import Icon from "../Icon";
import {
  api,
  errorText,
  toolLabel,
  type ComparisonView,
  type ReviewSnapshot,
} from "../api";
import ConfirmDialog from "../ui/ConfirmDialog";

const statusWords: Record<string, string> = {
  running: "Working",
  stopping: "Stopping",
  waiting: "Needs you",
  idle: "Ready",
  stopped: "Stopped",
  error: "Needs attention",
  closed: "Closed",
};
const REFRESH_MS = 4000;

/**
 * Both sides of a comparison: what each agent changed, file by file, and the
 * choice of which version to copy into the project. Nothing reaches the
 * project until the user confirms it.
 */
export default function CompareDialog({
  id,
  onClose,
  onOpenChat,
  onRemoved,
}: {
  id: string;
  onClose: () => void;
  onOpenChat: (chatId: string) => void;
  onRemoved: (chatIds: string[]) => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [view, setView] = useState<ComparisonView>();
  const [error, setError] = useState("");
  const [file, setFile] = useState("");
  const [diffs, setDiffs] = useState<Record<string, ReviewSnapshot>>({});
  const [confirm, setConfirm] = useState<"remove" | string>();
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState("");
  useEffect(() => {
    const el = dialog.current;
    if (el?.showModal) el.showModal();
    return () => el?.close?.();
  }, []);
  // Keep the view current while the agents work.
  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    const load = () =>
      api
        .ComparisonOverview(id)
        .then((next) => {
          if (!active) return;
          setView(next);
          setError("");
        })
        .catch((e) => active && setError(errorText(e)))
        .finally(() => {
          if (active) timer = setTimeout(load, REFRESH_MS);
        });
    load();
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [id]);
  const sides = view?.sides || [];
  const signature = sides
    .map((side) =>
      side.changes.files.map((f) => f.path + f.added + f.removed).join(),
    )
    .join("|");
  // Diffs are read again whenever the list of changes moves.
  useEffect(() => {
    if (!file) return;
    let active = true;
    for (const side of sides)
      api
        .ComparisonDiff(id, side.tool)
        .then(
          (snapshot) =>
            active && setDiffs((d) => ({ ...d, [side.tool]: snapshot })),
        )
        .catch((e) => active && setError(errorText(e)));
    return () => {
      active = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, !!file, signature]);

  const paths = [
    ...new Set(sides.flatMap((side) => side.changes.files.map((f) => f.path))),
  ].sort();
  const sideBusy = (status: string) =>
    ["running", "waiting", "stopping"].includes(status);
  async function use(tool: string) {
    setBusy(true);
    setError("");
    try {
      const files = await api.UseComparisonSide(id, tool);
      setDone(
        `Copied ${files.length} ${files.length === 1 ? "file" : "files"} from ${toolLabel(tool)} into ${view?.project}. Nothing was committed.`,
      );
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
      setConfirm(undefined);
    }
  }
  async function remove() {
    setBusy(true);
    setError("");
    try {
      await api.RemoveComparison(id);
      onRemoved(sides.map((side) => side.chatId));
      onClose();
    } catch (e) {
      setError(errorText(e));
      setBusy(false);
      setConfirm(undefined);
    }
  }
  const chosen = sides.find((side) => side.tool === confirm);
  return (
    <dialog
      ref={dialog}
      className="changes-dialog compare-dialog"
      aria-labelledby="compare-title"
      onClose={onClose}
      onCancel={(e) => {
        e.preventDefault();
        if (!busy && !confirm) onClose();
      }}
    >
      <header>
        <div>
          <h2 id="compare-title">Compare</h2>
          <p title={view?.prompt}>
            {view
              ? `${view.prompt.split("\n")[0]} · both started from ${view.base.slice(0, 7)}, each in its own worktree`
              : "Reading both sides…"}
          </p>
        </div>
        <button
          className="icon-button"
          aria-label="Close compare"
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
      {done && (
        <p className="compare-done" role="status">
          <Icon name="check" size={14} /> {done}
        </p>
      )}
      <div className="compare-body">
        <div className="compare-sides">
          {sides.map((side) => {
            const changes = side.changes;
            const working = sideBusy(side.status);
            return (
              <section
                key={side.tool}
                className="compare-side"
                aria-label={`${toolLabel(side.tool)} version`}
              >
                <header>
                  <AgentMark tool={side.tool} small />
                  <strong>{toolLabel(side.tool)}</strong>
                  <span className={`chip status ${side.status}`}>
                    {statusWords[side.status] || side.status}
                  </span>
                </header>
                <p className="compare-totals">
                  {changes.reason ||
                    (changes.files.length
                      ? `${changes.files.length} ${changes.files.length === 1 ? "file" : "files"} · `
                      : "No changes yet")}
                  {!changes.reason && changes.files.length > 0 && (
                    <>
                      <b className="added">+{changes.added}</b>{" "}
                      <b className="removed">−{changes.removed}</b>
                    </>
                  )}
                </p>
                <code className="compare-branch">{side.branch}</code>
                <div className="compare-actions">
                  <button
                    className="button secondary"
                    onClick={() => onOpenChat(side.chatId)}
                  >
                    Open conversation
                  </button>
                  <button
                    className="button primary"
                    disabled={busy || working || !changes.files.length}
                    title={
                      working
                        ? `Wait for ${toolLabel(side.tool)} to finish`
                        : "Copy these changes into the project's working tree"
                    }
                    onClick={() => setConfirm(side.tool)}
                  >
                    Use this version…
                  </button>
                </div>
              </section>
            );
          })}
        </div>
        {paths.length > 0 && (
          <table className="compare-files">
            <caption className="sr-only">Changed files on each side</caption>
            <thead>
              <tr>
                <th scope="col">File</th>
                {sides.map((side) => (
                  <th scope="col" key={side.tool}>
                    {toolLabel(side.tool)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {paths.map((path) => (
                <tr key={path} className={file === path ? "selected" : ""}>
                  <th scope="row">
                    <button
                      aria-pressed={file === path}
                      onClick={() => setFile(file === path ? "" : path)}
                    >
                      {path}
                    </button>
                  </th>
                  {sides.map((side) => {
                    const change = side.changes.files.find(
                      (f) => f.path === path,
                    );
                    return (
                      <td key={side.tool}>
                        {!change ? (
                          <span className="muted">No change</span>
                        ) : change.binary ? (
                          "binary"
                        ) : (
                          <>
                            {change.added > 0 && (
                              <b className="added">+{change.added}</b>
                            )}{" "}
                            {change.removed > 0 && (
                              <b className="removed">−{change.removed}</b>
                            )}
                          </>
                        )}
                      </td>
                    );
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        )}
        {file && (
          <div className="compare-diffs">
            {sides.map((side) => {
              const patch = diffs[side.tool]?.files.find(
                (f) => f.path === file,
              );
              return (
                <section
                  key={side.tool}
                  aria-label={`${toolLabel(side.tool)}: ${file}`}
                >
                  <h3>
                    <AgentMark tool={side.tool} dot /> {toolLabel(side.tool)}
                  </h3>
                  {patch ? (
                    <DiffViewer
                      snapshot={{ files: [patch] }}
                      selectedFile={file}
                      selectedLine={0}
                      onFile={() => {}}
                      hideFiles
                    />
                  ) : (
                    <p className="details-empty">
                      {diffs[side.tool]
                        ? "No change to this file."
                        : "Reading…"}
                    </p>
                  )}
                </section>
              );
            })}
          </div>
        )}
      </div>
      <footer className="compare-footer">
        <p>
          Nothing reaches your project until you use a version. Each side can
          also be committed or opened as a pull request from its conversation.
        </p>
        <button
          className="button secondary danger-text"
          disabled={busy || sides.some((side) => sideBusy(side.status))}
          onClick={() => setConfirm("remove")}
        >
          Remove comparison…
        </button>
      </footer>
      {chosen && (
        <ConfirmDialog
          title={`Use ${toolLabel(chosen.tool)}'s version?`}
          confirmLabel="Copy into project"
          cancelLabel="Cancel"
          busy={busy}
          onCancel={() => setConfirm(undefined)}
          onConfirm={() => void use(chosen.tool)}
        >
          <p>
            Copies {chosen.changes.files.length}{" "}
            {chosen.changes.files.length === 1
              ? "changed file"
              : "changed files"}{" "}
            into the working tree of {view?.project}. Nothing is committed.
          </p>
          <p>
            If your project has its own changes in any of these files, nothing
            is copied. The other version stays in its worktree.
          </p>
        </ConfirmDialog>
      )}
      {confirm === "remove" && (
        <ConfirmDialog
          title="Remove this comparison?"
          confirmLabel="Remove comparison"
          cancelLabel="Keep it"
          busy={busy}
          onCancel={() => setConfirm(undefined)}
          onConfirm={() => void remove()}
        >
          <p>
            Deletes both worktrees and their branches (
            {sides.map((side) => side.branch).join(", ")}) and closes both
            conversations.
          </p>
          <p>
            Changes you have not copied into the project or committed and pushed
            are lost. What you already copied stays.
          </p>
        </ConfirmDialog>
      )}
    </dialog>
  );
}
