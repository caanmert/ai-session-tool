import { useEffect, useRef, useState } from "react";
import { api, errorText, type ModelCatalog, type ModelSettings } from "./api";

export const defaultModelSettings: ModelSettings = {
  model: "",
  effort: "",
  configured: false,
};
type Props = {
  id: string;
  settings?: ModelSettings;
  reviewer?: boolean;
  active?: boolean;
  disabled: boolean;
  onPending: (pending: boolean) => void;
  onSaved: () => void;
};
export default function ModelPicker({
  id,
  settings = defaultModelSettings,
  reviewer = false,
  active = true,
  disabled,
  onPending,
  onSaved,
}: Props) {
  const [catalog, setCatalog] = useState<ModelCatalog>();
  const [choice, setChoice] = useState(settings);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [revision, setRevision] = useState(0);
  const operation = useRef(false);
  useEffect(() => {
    setChoice(settings);
  }, [settings.model, settings.effort, settings.configured]);
  useEffect(() => {
    if (!active) return;
    let disposed = false;
    setLoading(true);
    setError("");
    api
      .ChatModels(id, reviewer, revision > 0)
      .then((result) => {
        if (!disposed) setCatalog(result);
      })
      .catch((e) => {
        if (!disposed) setError(errorText(e));
      })
      .finally(() => {
        if (!disposed) setLoading(false);
      });
    return () => {
      disposed = true;
    };
  }, [id, reviewer, active, revision]);
  const options = catalog?.models || [];
  const selected =
    options.find((option) => option.id === choice.model) ||
    (!choice.model
      ? options.find((option) => option.id === catalog?.defaultModel)
      : undefined);
  const unavailable =
    !!choice.model && !options.some((option) => option.id === choice.model);
  async function save(next: ModelSettings) {
    if (operation.current || disabled) return;
    operation.current = true;
    setSaving(true);
    onPending(true);
    setError("");
    const previous = choice;
    setChoice(next);
    try {
      await api.SetChatSettings(id, { ...next, configured: true }, reviewer);
      onSaved();
    } catch (e) {
      setChoice(previous);
      setError(errorText(e));
    } finally {
      operation.current = false;
      setSaving(false);
      onPending(false);
    }
  }
  const prefix = reviewer ? "Reviewer " : "";
  return (
    <div
      className="model-settings"
      aria-label={reviewer ? "Reviewer settings" : "Conversation settings"}
    >
      <div className="model-settings-fields">
        <label>
          <span>{prefix}Model</span>
          <select
            aria-label={reviewer ? "Reviewer model" : "Model"}
            disabled={disabled || saving || loading}
            value={choice.model}
            onChange={(e) =>
              void save({ model: e.target.value, effort: "", configured: true })
            }
          >
            <option value="">
              Agent default
              {catalog?.defaultModel ? ` · ${catalog.defaultModel}` : ""}
            </option>
            {unavailable && (
              <option value={choice.model} disabled>
                {choice.model} · {loading ? "Loading" : "Unavailable"}
              </option>
            )}
            {options
              .filter((option) => option.id)
              .map((option) => (
                <option
                  key={option.id}
                  value={option.id}
                  title={option.description}
                >
                  {option.name || option.id}
                </option>
              ))}
          </select>
        </label>
        <label>
          <span>Reasoning</span>
          <select
            aria-label={
              reviewer ? "Reviewer reasoning effort" : "Reasoning effort"
            }
            value={choice.effort}
            disabled={
              disabled ||
              saving ||
              loading ||
              !selected?.efforts.length ||
              unavailable
            }
            onChange={(e) =>
              void save({ ...choice, effort: e.target.value, configured: true })
            }
          >
            <option value="">Agent default</option>
            {choice.effort && !selected?.efforts.includes(choice.effort) && (
              <option value={choice.effort} disabled>
                {choice.effort} · Unavailable
              </option>
            )}
            {selected?.efforts.map((effort) => (
              <option key={effort} value={effort}>
                {effort[0].toUpperCase() + effort.slice(1)}
              </option>
            ))}
          </select>
        </label>
        <button
          type="button"
          className="text-button"
          disabled={loading || saving || disabled}
          onClick={() => setRevision((n) => n + 1)}
          aria-label={reviewer ? "Refresh reviewer models" : "Refresh models"}
        >
          {loading ? "Loading models…" : "Refresh"}
        </button>
        <span className="model-settings-note">
          {saving
            ? "Saving…"
            : disabled
              ? "Available after this response"
              : reviewer
                ? "Saved separately for reviews"
                : "Applies to your next message"}
        </span>
      </div>
      {error && (
        <p className="model-settings-error" role="alert">
          {error} {!catalog && "Agent default remains available."}
        </p>
      )}
      {!loading && unavailable && !error && (
        <p className="model-settings-error">
          This saved model is not in the current catalog. Refresh or choose
          another model.
        </p>
      )}
    </div>
  );
}
