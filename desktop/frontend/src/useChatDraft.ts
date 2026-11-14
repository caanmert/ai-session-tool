import { useState } from "react";

const draftKey = (id: string) => `ais:chat-draft:v1:${id}`;

// ChatWorkspace is keyed by conversation ID, so each mounted workspace owns
// exactly one draft. Write on input instead of an effect: closing a tab or
// quitting immediately after typing must not lose the last edit.
export function useChatDraft(id: string) {
  const [initial] = useState(() => {
    try {
      return {
        text: window.localStorage.getItem(draftKey(id)) || "",
        error: "",
      };
    } catch {
      return {
        text: "",
        error:
          "Draft storage is unavailable. Keep a copy before closing this tab.",
      };
    }
  });
  const [prompt, setPrompt] = useState(initial.text);
  const [draftError, setDraftError] = useState(initial.error);
  const [restored, setRestored] = useState(!!initial.text);

  function updatePrompt(text: string) {
    setPrompt(text);
    setRestored(false);
    try {
      if (text) window.localStorage.setItem(draftKey(id), text);
      else window.localStorage.removeItem(draftKey(id));
      setDraftError("");
    } catch {
      setDraftError(
        text
          ? "Could not save this draft. Keep a copy before closing this tab."
          : "Could not clear the saved draft. It may reappear when you reopen this tab.",
      );
    }
  }

  return { prompt, updatePrompt, draftError, restored };
}
