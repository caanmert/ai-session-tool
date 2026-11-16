import { useEffect, useState } from "react";
import Icon from "../Icon";
import { api, errorText, type SavedPrompt } from "../api";

type Editing = { id: string; name: string; text: string; isNew: boolean };

/**
 * Reusable messages. Type "/" in a conversation to put one in the message
 * box; {project}, {branch} and {changed files} are filled in then.
 */
export default function SavedPromptsSection() {
  const [prompts, setPrompts] = useState<SavedPrompt[]>();
  const [editing, setEditing] = useState<Editing>();
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    api
      .SavedPrompts()
      .then((list) => setPrompts(list || []))
      .catch((e) => {
        setPrompts([]);
        setError(errorText(e));
      });
  }, []);
  async function save(next: SavedPrompt[]) {
    setSaving(true);
    setError("");
    try {
      await api.SetSavedPrompts(next);
      setPrompts(next);
      setEditing(undefined);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setSaving(false);
    }
  }
  function commit() {
    if (!editing || !prompts) return;
    const item = {
      id: editing.id,
      name: editing.name.trim(),
      text: editing.text.trim(),
    };
    void save(
      editing.isNew
        ? [...prompts, item]
        : prompts.map((p) => (p.id === item.id ? item : p)),
    );
  }
  const form = editing && (
    <div className="prompt-form">
      <label>
        <span>Name</span>
        <input
          value={editing.name}
          maxLength={80}
          // The form opens on the user's request.
          // eslint-disable-next-line jsx-a11y/no-autofocus
          autoFocus
          onChange={(e) => setEditing({ ...editing, name: e.target.value })}
        />
      </label>
      <label>
        <span>Text</span>
        <textarea
          value={editing.text}
          onChange={(e) => setEditing({ ...editing, text: e.target.value })}
        />
      </label>
      <div className="prompt-form-actions">
        <button
          className="button secondary"
          disabled={saving}
          onClick={() => setEditing(undefined)}
        >
          Cancel
        </button>
        <button
          className="button primary"
          disabled={saving || !editing.name.trim() || !editing.text.trim()}
          onClick={commit}
        >
          {saving ? "Saving…" : "Save prompt"}
        </button>
      </div>
    </div>
  );
  return (
    <section aria-labelledby="settings-prompts">
      <div className="settings-heading">
        <h2 id="settings-prompts">Saved prompts</h2>
        <button
          className="button secondary"
          disabled={!prompts || !!editing}
          onClick={() =>
            setEditing({
              id: `p-${Date.now()}`,
              name: "",
              text: "",
              isNew: true,
            })
          }
        >
          <Icon name="plus" size={14} /> New prompt
        </button>
      </div>
      <p className="setting-note">
        Type <kbd>/</kbd> in a conversation to put one in the message box; you
        can edit it before sending. <code>{"{project}"}</code>,{" "}
        <code>{"{branch}"}</code> and <code>{"{changed files}"}</code> are
        filled in.
      </p>
      <div className="setting-group">
        {!prompts && <p className="setting-note">Reading…</p>}
        {prompts && !prompts.length && !editing && (
          <p className="setting-note">No saved prompts yet.</p>
        )}
        {prompts && prompts.length > 0 && (
          <ul className="prompt-list">
            {prompts.map((p) =>
              editing?.id === p.id ? (
                <li key={p.id}>{form}</li>
              ) : (
                <li key={p.id}>
                  <div>
                    <strong>{p.name}</strong>
                    <span title={p.text}>{p.text}</span>
                  </div>
                  <button
                    className="text-button"
                    aria-label={`Edit ${p.name}`}
                    disabled={!!editing}
                    onClick={() => setEditing({ ...p, isNew: false })}
                  >
                    Edit
                  </button>
                  <button
                    className="text-button"
                    aria-label={`Remove ${p.name}`}
                    disabled={!!editing || saving}
                    onClick={() =>
                      void save(prompts.filter((item) => item.id !== p.id))
                    }
                  >
                    Remove
                  </button>
                </li>
              ),
            )}
          </ul>
        )}
        {editing?.isNew && form}
        {error && (
          <p className="setting-note" role="alert">
            {error}
          </p>
        )}
      </div>
    </section>
  );
}
