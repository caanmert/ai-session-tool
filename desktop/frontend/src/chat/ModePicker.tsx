import { useEffect, useRef, useState } from "react";
import { api, errorText, type ModeOption } from "../api";

/**
 * How much the agent may do without asking, from the next message on. Only
 * the modes the agent itself provides are listed.
 */
export default function ModePicker({
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
  const [modes, setModes] = useState<ModeOption[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const operation = useRef(false);
  useEffect(() => {
    let active = true;
    api
      .ChatModes(chatId)
      .then((found) => active && setModes(found))
      // Without the list the agent keeps its normal behavior.
      .catch(() => active && setModes([]));
    return () => {
      active = false;
    };
  }, [chatId]);
  if (modes.length < 2) return null;
  const current = modes.find((option) => option.id === mode);
  async function choose(next: string) {
    if (operation.current) return;
    operation.current = true;
    setSaving(true);
    setError("");
    try {
      await api.SetChatMode(chatId, next);
      onSaved();
    } catch (e) {
      setError(errorText(e));
    } finally {
      operation.current = false;
      setSaving(false);
    }
  }
  return (
    <span className="mode-picker">
      <select
        aria-label="Permission mode"
        title={current?.detail}
        value={mode}
        disabled={disabled || saving}
        onChange={(e) => void choose(e.target.value)}
      >
        {modes.map((option) => (
          <option key={option.id} value={option.id} title={option.detail}>
            {option.label}
          </option>
        ))}
      </select>
      {error && (
        <span className="mode-error" role="alert">
          {error}
        </span>
      )}
    </span>
  );
}
