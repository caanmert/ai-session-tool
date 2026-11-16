import { useState } from "react";
import Icon from "../Icon";
import { api, errorText } from "../api";
import Popover from "../ui/Popover";

export const webSearchChoices = [
  { id: "", label: "Codex's own setting", detail: "As set in Codex" },
  { id: "disabled", label: "Off", detail: "No web search" },
  { id: "cached", label: "Cached", detail: "Searches an index of pages" },
  { id: "live", label: "Live", detail: "Fetches pages as they are now" },
];

/**
 * Codex's web search for this conversation, from the next message: a globe
 * button with a short menu. It is marked when search is on.
 */
export default function WebSearchPicker({
  chatId,
  mode,
  disabled,
  onSaved,
}: {
  chatId: string;
  mode: string;
  disabled: boolean;
  onSaved: () => void;
}) {
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const current =
    webSearchChoices.find((choice) => choice.id === mode) ||
    webSearchChoices[0];
  const on = mode === "cached" || mode === "live";
  async function choose(next: string) {
    if (next === mode) return;
    setSaving(true);
    setError("");
    try {
      await api.SetChatWebSearch(chatId, next);
      onSaved();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setSaving(false);
    }
  }
  return (
    <span className="web-search-picker">
      <Popover
        label={`Web search: ${current.label}`}
        title={`Web search: ${current.label}. ${current.detail}.`}
        buttonClassName={`pill-button icon-pill ${on ? "on" : ""}`}
        panelLabel="Web search"
        role="menu"
        disabled={disabled || saving}
        button={
          <>
            <Icon name="globe" size={15} />
            {on && <span>{current.label}</span>}
          </>
        }
      >
        {(close) => (
          <>
            <p className="popover-title">Web search · from the next message</p>
            {webSearchChoices.map((choice) => (
              <button
                key={choice.id}
                type="button"
                role="menuitemradio"
                aria-checked={choice.id === mode}
                className="popover-item"
                onClick={() => {
                  close();
                  void choose(choice.id);
                }}
              >
                <span className="radio-dot" aria-hidden="true" />
                <strong>{choice.label}</strong>
                <small>{choice.detail}</small>
              </button>
            ))}
          </>
        )}
      </Popover>
      {error && (
        <span className="mode-error" role="alert">
          {error}
        </span>
      )}
    </span>
  );
}
