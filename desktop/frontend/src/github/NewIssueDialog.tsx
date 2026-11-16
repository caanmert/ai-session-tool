import { useEffect, useRef, useState } from "react";
import {
  api,
  errorText,
  type GitHubIssueCreated,
  type GitHubIssueForm,
} from "../api";
import { Dialog } from "../chat/GitDialogs";

const TITLE_LIMIT = 256;
const BODY_LIMIT = 65536;
const labelColor = (color: string) =>
  /^[0-9a-f]{6}$/i.test(color) ? `#${color}` : "transparent";

/**
 * Write a GitHub issue and create it in the project's repository through the
 * GitHub CLI. Nothing is sent until Create issue; Enter in the description
 * adds a line and Cmd+Enter creates.
 */
export default function NewIssueDialog({
  project,
  initialTitle = "",
  initialBody = "",
  onClose,
  onCreated,
}: {
  project: string;
  initialTitle?: string;
  initialBody?: string;
  onClose: () => void;
  onCreated?: (issue: GitHubIssueCreated) => void;
}) {
  const [form, setForm] = useState<GitHubIssueForm>();
  const [error, setError] = useState("");
  const [title, setTitle] = useState(initialTitle);
  const [body, setBody] = useState(initialBody);
  const [labels, setLabels] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [created, setCreated] = useState<GitHubIssueCreated>();
  const titleBox = useRef<HTMLInputElement>(null);
  const openButton = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    let active = true;
    api
      .GitHubIssueForm(project)
      .then((next) => {
        if (!active) return;
        setForm(next);
      })
      .catch((e) => active && setError(errorText(e)));
    return () => {
      active = false;
    };
  }, [project]);
  // You can start writing at once; the repository is read meanwhile.
  useEffect(() => titleBox.current?.focus(), []);
  const titleLength = [...title.trim()].length;
  const bodyLength = [...body].length;
  const tooLong = titleLength > TITLE_LIMIT || bodyLength > BODY_LIMIT;
  const ready =
    !!form && !form.reason && !busy && !created && titleLength > 0 && !tooLong;
  async function create() {
    if (!ready) return;
    setBusy(true);
    setError("");
    try {
      const issue = await api.CreateGitHubIssue(project, title, body, labels);
      setCreated(issue);
      onCreated?.(issue);
      setTimeout(() => openButton.current?.focus());
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  const createOnShortcut = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      void create();
    }
  };
  return (
    <Dialog
      title={created ? "Issue created" : "New issue"}
      className="issue-dialog"
      busy={busy}
      onClose={onClose}
    >
      {() =>
        created ? (
          <>
            <p className="confirm-body" role="status">
              Created issue #{created.number} in {form?.repository}.
            </p>
            <div className="confirm-actions">
              <button className="button secondary" onClick={onClose}>
                Close
              </button>
              <button
                ref={openButton}
                className="button primary"
                onClick={() =>
                  void api
                    .OpenURL(created.url)
                    .catch((e) => setError(errorText(e)))
                }
              >
                Open on GitHub
              </button>
            </div>
            {error && <p role="alert">{error}</p>}
          </>
        ) : (
          <>
            {form?.reason && <p className="confirm-body">{form.reason}</p>}
            {!form?.reason && (
              <>
                <p className="git-target">
                  {form ? (
                    <>
                      in <code>{form.repository}</code>
                    </>
                  ) : (
                    "Reading the repository…"
                  )}
                </p>
                <label className="git-field">
                  <span>Title</span>
                  <input
                    ref={titleBox}
                    className="field"
                    aria-label="Issue title"
                    value={title}
                    disabled={busy}
                    placeholder="What is wrong or missing, in one line"
                    onChange={(e) =>
                      setTitle(e.target.value.replace(/\r?\n/g, " "))
                    }
                    onKeyDown={(e) => {
                      // Enter alone must not create the issue.
                      if (e.key === "Enter" && !e.metaKey && !e.ctrlKey)
                        e.preventDefault();
                      createOnShortcut(e);
                    }}
                  />
                  {titleLength > TITLE_LIMIT && (
                    <small className="field-error">
                      {titleLength} of {TITLE_LIMIT} characters
                    </small>
                  )}
                </label>
                <label className="git-field">
                  <span>Description</span>
                  <textarea
                    aria-label="Issue description"
                    value={body}
                    disabled={busy}
                    placeholder="What happens, the evidence, and what done means. Markdown works."
                    onChange={(e) => setBody(e.target.value)}
                    onKeyDown={createOnShortcut}
                  />
                  {bodyLength > BODY_LIMIT && (
                    <small className="field-error">
                      Longer than GitHub allows ({BODY_LIMIT} characters)
                    </small>
                  )}
                </label>
                {!!form?.labels.length && (
                  <fieldset className="issue-labels">
                    <legend>Labels</legend>
                    {form.labels.map((label) => (
                      <label
                        key={label.name}
                        className="issue-label"
                        title={label.description || undefined}
                      >
                        <input
                          type="checkbox"
                          checked={labels.includes(label.name)}
                          disabled={busy}
                          onChange={(e) =>
                            setLabels((current) =>
                              e.target.checked
                                ? [...current, label.name]
                                : current.filter((name) => name !== label.name),
                            )
                          }
                        />
                        <i
                          aria-hidden="true"
                          style={{ background: labelColor(label.color) }}
                        />
                        {label.name}
                      </label>
                    ))}
                  </fieldset>
                )}
                {form?.partial && <p className="git-note">{form.partial}</p>}
                {form && (
                  <p className="git-note">
                    Creates the issue on GitHub as you, through the GitHub CLI.
                    Anyone who can see {form.repository} can read it.
                  </p>
                )}
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
                onClick={() => void create()}
              >
                {busy ? "Creating…" : "Create issue"}
                {!busy && <kbd>⌘⏎</kbd>}
              </button>
            </div>
          </>
        )
      }
    </Dialog>
  );
}
