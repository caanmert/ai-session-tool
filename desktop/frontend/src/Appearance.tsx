import { useEffect, useState } from "react";
import { api } from "./api";

type Appearance = "system" | "light" | "dark";
const key = "ais:appearance:v1";
function read(): Appearance {
  try {
    const saved = window.localStorage.getItem(key);
    if (saved === "light" || saved === "dark") return saved;
  } catch {
    /* System appearance remains available without storage. */
  }
  return "system";
}

export type AppearanceControl = {
  mode: Appearance;
  error: string;
  choose: (next: Appearance) => void;
};

/**
 * Applies the appearance for the whole app and follows the system setting.
 * Call it once, high in the tree, so it keeps working on every page.
 */
export function useAppearance(): AppearanceControl {
  const [mode, setMode] = useState<Appearance>(read);
  const [systemDark, setSystemDark] = useState(
    () => window.matchMedia?.("(prefers-color-scheme: dark)").matches || false,
  );
  const [error, setError] = useState("");
  const resolved = mode === "system" ? (systemDark ? "dark" : "light") : mode;
  useEffect(() => {
    const media = window.matchMedia?.("(prefers-color-scheme: dark)");
    if (!media) return;
    const update = () => setSystemDark(media.matches);
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);
  useEffect(() => {
    document.documentElement.dataset.theme = resolved;
    document.documentElement.style.colorScheme = resolved;
    let active = true;
    api.SetAppearance(resolved).catch(() => {
      if (active)
        setError(
          "Window appearance could not be updated. Your page theme is active.",
        );
    });
    return () => {
      active = false;
    };
  }, [resolved]);
  function choose(next: Appearance) {
    setMode(next);
    setError("");
    try {
      window.localStorage.setItem(key, next);
    } catch {
      setError("Appearance changed for this visit. It could not be saved.");
    }
  }
  return { mode, error, choose };
}

/** The System / Light / Dark control. */
export function AppearanceSetting({ mode, error, choose }: AppearanceControl) {
  return (
    <div className="appearance">
      <label htmlFor="appearance">Appearance</label>
      <select
        id="appearance"
        value={mode}
        onChange={(e) => choose(e.target.value as Appearance)}
      >
        <option value="system">System</option>
        <option value="light">Light</option>
        <option value="dark">Dark</option>
      </select>
      {error && <p role="status">{error}</p>}
    </div>
  );
}

/** Self-contained control, for use where no parent owns the appearance. */
export default function Appearance() {
  return <AppearanceSetting {...useAppearance()} />;
}
