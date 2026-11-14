import { useEffect, useRef, useState } from "react";
import Icon from "../Icon";
import type { AnnotationChange, Session } from "../api";

type Change = (change: AnnotationChange) => Promise<void>;

/** Title of a saved conversation with rename and pin. Names belong to ais; the
 * agent's own transcript is not changed. */
export function SessionTitle({
  session,
  onChange,
}: {
  session: Session;
  onChange: Change;
}) {
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState("");
  const [saving, setSaving] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const rename = useRef<HTMLButtonElement>(null);
  useEffect(() => setEditing(false), [session.key]);
  useEffect(() => {
    if (editing) input.current?.select();
  }, [editing]);
  async function apply(change: AnnotationChange) {
    setSaving(true);
    try {
      await onChange(change);
      setEditing(false);
    } finally {
      setSaving(false);
    }
  }
  function cancel() {
    setEditing(false);
    rename.current?.focus();
  }
  if (editing)
    return (
      <form
        className="title-edit"
        onSubmit={(e) => {
          e.preventDefault();
          void apply({ title: title.trim() });
        }}
      >
        <input
          ref={input}
          aria-label="Conversation title"
          value={title}
          maxLength={200}
          disabled={saving}
          onChange={(e) => setTitle(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              e.preventDefault();
              cancel();
            }
          }}
        />
        <button className="button primary" disabled={saving}>
          Save
        </button>
        <button
          type="button"
          className="button secondary"
          disabled={saving}
          onClick={cancel}
        >
          Cancel
        </button>
        <small>Leave empty to use the agent's own title.</small>
      </form>
    );
  return (
    <div className="title-row">
      <h2>{session.title || "Untitled conversation"}</h2>
      <button
        ref={rename}
        className="icon-button"
        aria-label="Rename conversation"
        title="Rename"
        onClick={() => {
          setTitle(session.title || "");
          setEditing(true);
        }}
      >
        <Icon name="pencil" size={15} />
      </button>
      <button
        className="icon-button pin-toggle"
        aria-label="Pin conversation"
        aria-pressed={!!session.pinned}
        title={session.pinned ? "Unpin" : "Pin to the top of lists"}
        disabled={saving}
        onClick={() => void apply({ pinned: !session.pinned })}
      >
        <Icon name="pin" size={15} />
      </button>
    </div>
  );
}

/** Tags of a saved conversation, with add and remove. */
export function SessionTags({
  session,
  onChange,
}: {
  session: Session;
  onChange: Change;
}) {
  const [tag, setTag] = useState("");
  const [saving, setSaving] = useState(false);
  async function apply(change: AnnotationChange) {
    setSaving(true);
    try {
      await onChange(change);
      setTag("");
    } finally {
      setSaving(false);
    }
  }
  return (
    <div className="tags">
      {session.tags?.map((name) => (
        <span key={name}>
          #{name}
          <button
            aria-label={`Remove tag ${name}`}
            disabled={saving}
            onClick={() => void apply({ removeTags: [name] })}
          >
            <Icon name="close" size={10} />
          </button>
        </span>
      ))}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (tag.trim()) void apply({ addTags: [tag] });
        }}
      >
        <input
          aria-label="Add a tag"
          placeholder="Add tag"
          value={tag}
          maxLength={41}
          disabled={saving}
          onChange={(e) => setTag(e.target.value)}
        />
      </form>
    </div>
  );
}
