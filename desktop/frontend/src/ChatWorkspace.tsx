import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import ModelPicker from "./ModelPicker";
import DiffViewer from "./DiffViewer";
import AgentMark from "./AgentMark";
import Icon from "./Icon";
import MessageList from "./chat/MessageList";
import { SafeMarkdown } from "./ui/SafeMarkdown";
import DetailsPanel, { useChanges } from "./chat/DetailsPanel";
import ConfirmDialog from "./ui/ConfirmDialog";
import { useFileMentions } from "./chat/FileMentions";
import { useSkillPicker } from "./chat/SkillPicker";
import { conversationStatus, requestSummary } from "./chat/conversation";
import FindBar from "./chat/FindBar";
import { useQueuedMessage } from "./chat/queuedMessage";
import ContextMeter from "./chat/ContextMeter";
import Popover from "./ui/Popover";
import { revealAround } from "./ui/Foldable";
import { useNarrow } from "./ui/useNarrow";
import ChangesDialog from "./chat/ChangesDialog";
import CompareDialog from "./chat/CompareDialog";
import ModePicker from "./chat/ModePicker";
import WebSearchPicker from "./chat/WebSearchPicker";
import {
  AttachedImages,
  imageFiles,
  useImageAttachments,
} from "./chat/ImageAttachments";
import { applyChatUpdate } from "./chatUpdates";
import { storeDraft, useChatDraft } from "./useChatDraft";
import {
  api,
  errorText,
  toolLabel,
  type ChatState,
  type ChatApproval,
  type Session,
  type ReviewDraft,
  type Checkpoint,
  type RestoreChange,
} from "./api";

export const chatBusy = (status: string) =>
  ["running", "waiting", "stopping"].includes(status);
// Time since this view first saw the agent working on the current response.
// It restarts with the app; it is a guide, not a record.
function Elapsed() {
  const [start] = useState(() => Date.now());
  const [now, setNow] = useState(start);
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);
  const seconds = Math.floor((now - start) / 1000);
  return (
    <span className="elapsed" aria-hidden="true">
      {seconds < 60
        ? `${seconds}s`
        : `${Math.floor(seconds / 60)}m ${String(seconds % 60).padStart(2, "0")}s`}
    </span>
  );
}

/** How many messages a conversation draws at first, and adds per request. */
export const MESSAGE_WINDOW = 150;

type Props = {
  chat: ChatState;
  /** The saved session behind this conversation, once the agent has one. */
  session?: Session;
  active: boolean;
  reviewerAvailable: boolean;
  onOpen: (chat: ChatState) => void;
  onSelect: (id: string) => void;
  onState: (chat: ChatState) => void;
  /** Conversations that were closed from here, such as by removing a comparison. */
  onClosed?: (ids: string[]) => void;
  /** Whether this pane takes the keyboard: false for the other pane of a split. */
  focused?: boolean;
  /** Which side this conversation is on in a split. */
  pane?: "first" | "second";
  /** A split pane asks for focus when clicked or tabbed into. */
  onFocusPane?: () => void;
  /** Closes this pane of a split, keeping the conversation open. */
  onClosePane?: () => void;
  /** A message to show, for example one found by search. */
  focusMessage?: { id: string; at: number };
  /** The details panel is hidden on wide windows (a remembered choice). */
  detailsHidden?: boolean;
  onDetailsHidden?: (hidden: boolean) => void;
};
export default function ChatWorkspace({
  chat,
  session,
  active,
  reviewerAvailable,
  onOpen,
  onSelect,
  onState,
  onClosed,
  detailsHidden = false,
  onDetailsHidden,
  focusMessage,
  focused = true,
  pane,
  onFocusPane,
  onClosePane,
}: Props) {
  const [state, setState] = useState(chat);
  const { prompt, updatePrompt, draftError, restored } = useChatDraft(chat.id);
  const [error, setError] = useState("");
  const [actionWorking, setWorking] = useState(false);
  const [settingsBusy, setSettingsBusy] = useState(false);
  const working = actionWorking || settingsBusy;
  const settingsOperation = useRef(false);
  function settingsPending(value: boolean) {
    settingsOperation.current = value;
    setSettingsBusy(value);
  }
  const [retry, setRetry] = useState(0);
  const [draft, setDraft] = useState<ReviewDraft>();
  const [selected, setSelected] = useState<string[]>([]);
  const [file, setFile] = useState("");
  const [line, setLine] = useState(0);
  const [showDiff, setShowDiff] = useState(false);
  const messages = useRef<HTMLDivElement>(null);
  const following = useRef(true);
  const [awayFromLatest, setAwayFromLatest] = useState(false);
  const activeRef = useRef(active);
  activeRef.current = active;
  const focusedRef = useRef(focused);
  focusedRef.current = focused;
  const notify = useRef(onState);
  notify.current = onState;
  const input = useRef<HTMLTextAreaElement>(null);
  const operation = useRef(false);
  useEffect(() => {
    let disposed = false;
    let timer: ReturnType<typeof setTimeout>;
    let version = -1;
    let current: ChatState | undefined;
    let lastStatus = chat.status;
    let lastTitle = chat.title;
    let lastApprovals = chat.approvals.map((a) => a.id).join();
    // The backend holds each read until the conversation changes, so this loop
    // costs nothing while idle. Updates carry only the messages that changed.
    async function poll() {
      try {
        const update = await api.ReadChat(chat.id, version);
        if (disposed) return;
        const next = applyChatUpdate(current, update);
        if (!next) {
          // A partial update did not fit: read the whole conversation again.
          version = -1;
          timer = setTimeout(() => void poll(), 0);
          return;
        }
        version = update.version;
        if (update.state) {
          current = next;
          setState(next);
          const approvals = next.approvals.map((a) => a.id).join();
          if (
            next.status !== lastStatus ||
            next.title !== lastTitle ||
            approvals !== lastApprovals
          ) {
            notify.current(next);
            lastStatus = next.status;
            lastTitle = next.title;
            lastApprovals = approvals;
          }
        }
        // A reply without changes means the backend's wait ran out, or the
        // backend answers at once (tests, browser fixtures): pause before
        // asking again.
        timer = setTimeout(
          () => void poll(),
          update.state ? 0 : activeRef.current ? 120 : 500,
        );
      } catch (e) {
        if (!disposed) setError(errorText(e));
      }
    }
    void poll();
    return () => {
      disposed = true;
      clearTimeout(timer);
    };
  }, [chat.id, retry]);
  useEffect(() => {
    if (active && focusedRef.current && !chatBusy(state.status))
      input.current?.focus();
  }, [active]);
  useEffect(() => {
    if (following.current && messages.current)
      messages.current.scrollTop = messages.current.scrollHeight;
  }, [state.version, active]);
  const attachments = useImageAttachments();
  const imagePicker = useRef<HTMLInputElement>(null);
  const mentions = useFileMentions({
    chatId: state.id,
    prompt,
    updatePrompt,
    input,
  });
  const skills = useSkillPicker({
    chatId: state.id,
    tool: state.tool,
    project: state.project,
    prompt,
    updatePrompt,
    input,
  });
  const trackCaret = () => {
    mentions.trackCaret();
    skills.trackCaret();
  };
  // Up and Down in an empty message box step through your earlier messages
  // in this conversation, newest first.
  const [recalled, setRecalled] = useState(-1);
  function recall(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key !== "ArrowUp" && e.key !== "ArrowDown") return false;
    const sent = state.messages
      .filter((m) => m.role === "user" && m.kind === "text" && m.text)
      .map((m) => m.text)
      .reverse();
    const stepping = recalled >= 0 && prompt === sent[recalled];
    if (!stepping && prompt) return false;
    const next = (stepping ? recalled : -1) + (e.key === "ArrowUp" ? 1 : -1);
    if (next >= sent.length || (next < -1 && !stepping)) return false;
    e.preventDefault();
    setRecalled(Math.max(next, -1));
    updatePrompt(next >= 0 ? sent[next] : "");
    return true;
  }
  // The message box grows with its text, up to the CSS max-height.
  useEffect(() => {
    const el = input.current;
    if (!el) return;
    el.style.height = "auto";
    if (el.scrollHeight) el.style.height = `${el.scrollHeight}px`;
  }, [prompt, active]);
  const busy = chatBusy(state.status);
  // Put one of your messages back in the message box, after any text you
  // have already written there. Nothing is sent.
  const promptNow = useRef(prompt);
  promptNow.current = prompt;
  const reuse = useCallback(
    (text: string) => {
      const current = promptNow.current.trimEnd();
      updatePrompt(current ? `${current}\n\n${text}` : text);
      input.current?.focus();
    },
    [updatePrompt],
  );
  // A message written while the agent works waits here and is sent when the
  // turn ends normally. It is never sent while another action is under way.
  const sendQueued = useCallback(
    async (text: string) => {
      if (operation.current || settingsOperation.current) return false;
      try {
        await api.SendChat(state.id, text);
        setRetry((n) => n + 1);
        following.current = true;
        setAwayFromLatest(false);
        return true;
      } catch (e) {
        setError(errorText(e));
        return false;
      }
    },
    [state.id],
  );
  const queue = useQueuedMessage(state.id, state.status, sendQueued);
  const [queueNote, setQueueNote] = useState("");
  // Details sit beside the conversation on wide windows and in a drawer below
  // 1200px. Reviews have their own fixed snapshot instead.
  // A pane of a split is narrow whatever the window: details open as a drawer.
  const narrow = useNarrow() || !!pane;
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [confirmStop, setConfirmStop] = useState(false);
  const [comparing, setComparing] = useState(false);
  const detailsToggle = useRef<HTMLButtonElement>(null);
  const showDetails = !state.review && (narrow ? detailsOpen : !detailsHidden);
  // Wide windows hide or show the panel; narrow ones open it as a drawer.
  const toggleDetails = useCallback(() => {
    if (narrow) setDetailsOpen((open) => !open);
    else onDetailsHidden?.(!detailsHidden);
  }, [narrow, detailsHidden, onDetailsHidden]);
  const changes = useChanges(state, active && !state.review);
  // Going back to a checkpoint: the files it would change are read first and
  // shown; nothing changes until the user confirms.
  const checkpointsByMessage = useMemo(
    () =>
      Object.fromEntries(
        (state.checkpoints || []).map((point) => [point.messageId, point]),
      ),
    [state.checkpoints],
  );
  const [turnDiff, setTurnDiff] = useState<Checkpoint>();
  const [turnFile, setTurnFile] = useState("");
  // Puts text in the message box after what is already there.
  function draftAppend(text: string) {
    updatePrompt(prompt.trim() ? `${prompt.trimEnd()}\n\n${text}` : text);
    setTimeout(() => input.current?.focus());
  }
  const [restoring, setRestoring] = useState<{
    point: Checkpoint;
    plan?: RestoreChange[];
    error?: string;
  }>();
  const [restoreBusy, setRestoreBusy] = useState(false);
  const askRestore = useCallback(
    (point: Checkpoint) => {
      setRestoring({ point });
      api
        .RestorePlan(state.id, point.id)
        .then((plan) =>
          setRestoring((current) =>
            current?.point.id === point.id ? { ...current, plan } : current,
          ),
        )
        .catch((e) =>
          setRestoring((current) =>
            current?.point.id === point.id
              ? { ...current, error: errorText(e) }
              : current,
          ),
        );
    },
    [state.id],
  );
  async function restoreNow() {
    if (!restoring?.plan?.length) return;
    setRestoreBusy(true);
    try {
      await api.RestoreCheckpoint(state.id, restoring.point.id);
      setRestoring(undefined);
      setRetry((n) => n + 1);
      changes.refresh();
    } catch (e) {
      setRestoring((current) => current && { ...current, error: errorText(e) });
    } finally {
      setRestoreBusy(false);
    }
  }
  function closeDetails() {
    setDetailsOpen(false);
    detailsToggle.current?.focus();
  }
  // A long conversation draws only its latest messages; earlier ones are added
  // on request, a screenful at a time, keeping the reading position.
  const listed = useMemo(
    () =>
      state.review?.complete
        ? state.messages.filter(
            (m) => !(m.role === "assistant" && m.kind === "text"),
          )
        : state.messages,
    [state.messages, state.review?.complete],
  );
  const [shown, setShown] = useState(MESSAGE_WINDOW);
  const earlier = Math.max(0, listed.length - shown);
  const heightBefore = useRef(0);
  function showEarlier() {
    heightBefore.current = messages.current?.scrollHeight || 0;
    following.current = false;
    setShown((count) => count + MESSAGE_WINDOW);
  }
  useLayoutEffect(() => {
    const el = messages.current;
    if (!el || !heightBefore.current) return;
    // Messages were added above: move down by their height so the view stays put.
    el.scrollTop += el.scrollHeight - heightBefore.current;
    heightBefore.current = 0;
  }, [shown]);
  // Show a message found by search: draw it if it is among the earlier ones,
  // open whatever folds it, scroll to it and mark it for a moment.
  useEffect(() => {
    if (!focusMessage || !active) return;
    const at = listed.findIndex((m) => m.id === focusMessage.id);
    if (at < 0) return;
    if (at < listed.length - shown) {
      setShown(listed.length - at);
      return;
    }
    const frame = requestAnimationFrame(() => {
      const box = messages.current;
      const el = box?.querySelector<HTMLElement>(
        `[data-message-id="${CSS.escape(focusMessage.id)}"]`,
      );
      if (!box || !el) return;
      following.current = false;
      revealAround(el, box);
      el.scrollIntoView?.({ block: "center" });
      el.classList.add("found");
      setTimeout(() => el.classList.remove("found"), 1600);
    });
    return () => cancelAnimationFrame(frame);
  }, [focusMessage, active, listed, shown]);
  // Cmd+F searches this conversation; the window has no page search of its own.
  const [finding, setFinding] = useState(false);
  useEffect(() => {
    if (!active || !focused) return;
    const key = (e: KeyboardEvent) => {
      if (
        (e.metaKey || e.ctrlKey) &&
        e.key.toLowerCase() === "f" &&
        !document.querySelector("dialog[open]")
      ) {
        e.preventDefault();
        setFinding(true);
      }
    };
    document.addEventListener("keydown", key);
    return () => document.removeEventListener("keydown", key);
  }, [active, focused]);
  useEffect(() => {
    if (!active || !focused || state.review) return;
    const key = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "\\") {
        e.preventDefault();
        toggleDetails();
      }
    };
    document.addEventListener("keydown", key);
    return () => document.removeEventListener("keydown", key);
  }, [active, focused, state.review, toggleDetails]);
  const other = state.tool === "codex" ? "claude" : "codex";
  // Only a conversation with something asked can be handed over.
  const hasAsked = state.messages.some(
    (m) => m.role === "user" && m.kind === "text",
  );
  const handOffBlocked = !reviewerAvailable
    ? `Install ${toolLabel(other)} to continue with it`
    : !hasAsked
      ? "Send a message first; there is nothing to hand over yet"
      : busy
        ? "Wait for the agent to finish first"
        : "";
  async function handOff() {
    const handed = await api.HandOffChat(state.id);
    // Without draft storage, the summary goes to the clipboard.
    if (!storeDraft(handed.chat.id, handed.prompt))
      await navigator.clipboard?.writeText(handed.prompt).catch(() => {});
    onOpen(handed.chat);
  }
  // Add the paths of chosen files to the message. The agent opens them itself;
  // nothing is uploaded or copied.
  async function mentionFiles() {
    try {
      const paths = await api.PickFiles(state.id);
      if (!paths.length) return;
      const mentions = paths
        .map((path) => (/\s/.test(path) ? `"${path}"` : path))
        .join(" ");
      updatePrompt(
        prompt && !/\s$/.test(prompt)
          ? `${prompt} ${mentions} `
          : `${prompt}${mentions} `,
      );
      input.current?.focus();
    } catch (e) {
      setError(errorText(e));
    }
  }
  async function perform(action: () => Promise<void>) {
    if (operation.current || settingsOperation.current) return;
    operation.current = true;
    setWorking(true);
    setError("");
    try {
      await action();
    } catch (e) {
      setError(errorText(e));
    } finally {
      operation.current = false;
      setWorking(false);
    }
  }
  async function send() {
    if (!prompt.trim() || settingsOperation.current) return;
    if (busy) {
      // While the agent works, Enter queues the message for the turn's end.
      if (attachments.images.length) {
        setQueueNote(
          "Images can be sent once the response ends. Your message and images stay in the box.",
        );
        return;
      }
      queue.add(prompt.trim());
      updatePrompt("");
      setQueueNote("");
      return;
    }
    setQueueNote("");
    const text = prompt;
    const images = attachments.payload();
    await perform(async () => {
      if (images.length) await api.SendChatWithImages(state.id, text, images);
      else await api.SendChat(state.id, text);
      attachments.clear();
      updatePrompt("");
      setRetry((n) => n + 1);
      following.current = true;
      setAwayFromLatest(false);
    });
  }
  return (
    <section
      className={`chat-workspace ${pane ? `pane ${pane}` : ""} ${pane && focused ? "focused" : ""}`}
      hidden={!active}
      aria-label={`${toolLabel(state.tool)} conversation`}
      onPointerDownCapture={pane && !focused ? onFocusPane : undefined}
      onFocusCapture={pane && !focused ? onFocusPane : undefined}
    >
      <div className="chat-main">
        <header className="chat-header">
          <div className="chat-heading">
            <p className="chat-breadcrumb" title={state.project}>
              {state.project.split("/").filter(Boolean).at(-1) || state.project}
            </p>
            <h2>{state.title || "New conversation"}</h2>
            <div className="chat-chips">
              <span className={`chip agent ${state.tool}`}>
                <AgentMark tool={state.tool} dot />
                {toolLabel(state.tool)}
              </span>
              <span className={`chip status ${state.status}`}>
                {(state.status === "waiting" || state.status === "error") && (
                  <Icon name="alert" size={12} />
                )}
                {conversationStatus(state)}
                {state.status === "running" && <Elapsed />}
              </span>
              {state.activeModel && !state.review && (
                <span className="chat-meta">{state.activeModel}</span>
              )}
              {state.handedFrom && (
                <button
                  className="text-button chat-link"
                  title="Open the conversation this one continues"
                  onClick={() => onSelect(state.handedFrom!)}
                >
                  Continued from {toolLabel(other)} ↗
                </button>
              )}
              {!!state.handedTo?.length && (
                <button
                  className="text-button chat-link"
                  title="Open the conversation that continues this one"
                  onClick={() => onSelect(state.handedTo!.at(-1)!)}
                >
                  Continued in {toolLabel(other)} ↗
                </button>
              )}
            </div>
          </div>
          <div className="chat-actions">
            {onClosePane && (
              <button
                className="icon-button close-pane"
                aria-label="Close this pane"
                title="Close this pane; the conversation stays open"
                onClick={onClosePane}
              >
                <Icon name="close" size={14} />
              </button>
            )}
            {!state.review && (
              <button
                ref={detailsToggle}
                className="button secondary"
                aria-pressed={showDetails}
                title={`${showDetails ? "Hide" : "Show"} changed files and session details (⌘\\)`}
                onClick={toggleDetails}
              >
                Changes
                {!!changes.summary?.files.length && (
                  <b className="count">{changes.summary.files.length}</b>
                )}
              </button>
            )}
            {state.comparison && (
              <button
                className="button secondary"
                title="See both agents' versions side by side"
                onClick={() => setComparing(true)}
              >
                Compare
              </button>
            )}
            {!state.review && (
              <button
                className="button secondary"
                disabled={busy || working || !reviewerAvailable}
                title={
                  reviewerAvailable
                    ? "Preview the current working-tree diff before sending it"
                    : "Install the other agent to request a second review"
                }
                onClick={() =>
                  void perform(async () =>
                    setDraft(await api.PrepareReview(state.id)),
                  )
                }
              >
                Review with {toolLabel(other)}
              </button>
            )}
          </div>
        </header>
        {(error || state.error) && (
          <div className="notice error" role="alert">
            <span>{error || state.error}</span>
            {error && (
              <button
                className="text-button"
                onClick={() => {
                  setError("");
                  setRetry((n) => n + 1);
                }}
              >
                Reconnect
              </button>
            )}
          </div>
        )}
        {state.review && (
          <div className="review-banner">
            <div>
              <strong>Independent review by {toolLabel(state.tool)}</strong>
              <span>
                Snapshot {state.review.snapshot.hash.slice(0, 12)} ·{" "}
                {state.review.snapshot.files.length}{" "}
                {state.review.snapshot.files.length === 1 ? "file" : "files"} ·
                Analysis only
                {state.activeModel && <> · {state.activeModel}</>}
              </span>
            </div>
            <button
              className="text-button"
              onClick={() => onSelect(state.review!.sourceId)}
            >
              Original conversation ↗
            </button>
            <button
              className="text-button"
              onClick={() => setShowDiff((value) => !value)}
            >
              {showDiff ? "Hide diff" : "View reviewed diff"}
            </button>
          </div>
        )}
        {state.review && showDiff && (
          <DiffViewer
            snapshot={state.review.snapshot}
            selectedFile={file}
            selectedLine={line}
            onFile={(path) => {
              setFile(path);
              setLine(0);
            }}
          />
        )}
        {finding && (
          <FindBar
            container={messages}
            version={state.version}
            onClose={() => {
              setFinding(false);
              input.current?.focus();
            }}
          />
        )}
        <div
          className="chat-messages"
          ref={messages}
          onScroll={() => {
            const el = messages.current;
            if (el)
              following.current =
                el.scrollHeight - el.scrollTop - el.clientHeight < 80;
            setAwayFromLatest(!following.current);
          }}
          aria-live="polite"
          aria-relevant="additions"
        >
          {state.shortened && (
            <p className="preview-note">
              Earlier messages were shortened for display. The agent retains its
              saved conversation.
            </p>
          )}
          {!state.messages.length && !state.review && (
            <div className="chat-welcome">
              <div className="eyebrow">A CONVERSATION, READY TO BEGIN</div>
              <h2>What are we working on?</h2>
              <p>
                Describe a change, ask a question, or paste an error.
                <br />
                {toolLabel(state.tool)} will work in this project.
              </p>
              <div className="chat-suggestions">
                Live replies <span>·</span> Tool activity <span>·</span>{" "}
                Approvals here
              </div>
            </div>
          )}
          {earlier > 0 && (
            <button className="show-earlier" onClick={showEarlier}>
              Show {Math.min(earlier, MESSAGE_WINDOW)} earlier messages
              <small>{earlier} not shown</small>
            </button>
          )}
          <MessageList
            tool={state.tool}
            project={state.project}
            busy={busy}
            onReuse={reuse}
            checkpoints={checkpointsByMessage}
            onRestore={askRestore}
            onTurnDiff={state.review ? undefined : setTurnDiff}
            messages={earlier > 0 ? listed.slice(earlier) : listed}
          />
          {state.review?.complete && (
            <div className="review-findings">
              <div className="eyebrow">REVIEW RESULTS</div>
              <h3>
                {state.review.findings.length
                  ? `${state.review.findings.length} ${state.review.findings.length === 1 ? "finding" : "findings"} to consider`
                  : "No actionable findings reported"}
              </h3>
              <p>{state.review.summary}</p>
              {state.review.findings.map((finding) => (
                <article
                  className={`review-finding ${finding.sent ? "sent" : ""}`}
                  key={finding.id}
                >
                  <label>
                    <input
                      type="checkbox"
                      aria-label={`Select ${finding.title}`}
                      checked={selected.includes(finding.id)}
                      disabled={finding.sent || working}
                      onChange={(e) =>
                        setSelected((ids) =>
                          e.target.checked
                            ? [...ids, finding.id]
                            : ids.filter((id) => id !== finding.id),
                        )
                      }
                    />
                    <span className={`priority ${finding.priority}`}>
                      {finding.priority}
                    </span>
                    <strong>{finding.title}</strong>
                    {finding.sent && <small>Sent for fixes</small>}
                  </label>
                  <button
                    className="finding-location"
                    onClick={() => {
                      setFile(finding.file);
                      setLine(finding.line);
                      setShowDiff(true);
                    }}
                  >
                    {finding.file}
                    {finding.line ? `:${finding.line}` : ""}
                  </button>
                  <div className="markdown">
                    <SafeMarkdown text={finding.body} />
                  </div>
                </article>
              ))}
              {!!state.review.findings.length && (
                <button
                  className="button primary"
                  disabled={!selected.length || working}
                  onClick={() =>
                    void perform(async () => {
                      const source = await api.SendReviewFindings(
                        state.id,
                        selected,
                      );
                      setSelected([]);
                      onSelect(source);
                    })
                  }
                >
                  Send {selected.length || "selected"} findings to{" "}
                  {toolLabel(other)}
                </button>
              )}
              <p className="preview-note">
                Findings are suggestions. Selected findings go back as a request
                to investigate and fix; they do not apply a patch automatically.
              </p>
            </div>
          )}
          {busy && (
            <div className="chat-working" role="status">
              <span className="loader" />
              {state.status === "waiting"
                ? "Waiting for your response below"
                : state.status === "stopping"
                  ? "Stopping the response…"
                  : `${toolLabel(state.tool)} is working…`}
            </div>
          )}
        </div>
        {awayFromLatest && (
          <button
            className="jump-latest"
            onClick={() => {
              following.current = true;
              setAwayFromLatest(false);
              if (messages.current)
                messages.current.scrollTop = messages.current.scrollHeight;
            }}
          >
            Jump to latest ↓
          </button>
        )}
        {!!state.approvals.length && (
          <div className="approval-list">
            {state.approvals.map((approval) => (
              <ApprovalCard
                key={approval.id}
                approval={approval}
                onAnswer={(allow, answers) =>
                  api.AnswerChat(state.id, approval.id, allow, answers)
                }
                onOption={(option) =>
                  api.AnswerChatOption(state.id, approval.id, option)
                }
              />
            ))}
          </div>
        )}
        <form
          className={`chat-composer ${state.review ? "review-footer" : ""}`}
          onSubmit={(e) => {
            e.preventDefault();
            void send();
          }}
          onDragOver={(e) => {
            // Allow dropping image files onto the message box.
            if (!state.review && e.dataTransfer.types.includes("Files"))
              e.preventDefault();
          }}
          onDrop={(e) => {
            const dropped = imageFiles(e.dataTransfer);
            if (state.review || !dropped.length) return;
            e.preventDefault();
            void attachments.add(dropped);
          }}
        >
          {!state.review && (skills.popup || mentions.popup)}
          {!state.review && queue.queued && (
            <div
              className={`queued-message ${queue.queued.held ? "held" : ""}`}
              role="group"
              aria-label={
                queue.queued.held ? "Message not sent" : "Queued message"
              }
            >
              <p className="queued-head" role="status">
                <Icon name={queue.queued.held ? "alert" : "clock"} size={13} />
                <strong>{queue.queued.held ? "Not sent" : "Queued"}</strong>
                <span>
                  {queue.queued.held
                    ? queue.queued.held.replace(/^Not sent: /, "")
                    : `Sent when ${toolLabel(state.tool)} finishes this response.`}
                </span>
              </p>
              <p className="queued-text">{queue.queued.text}</p>
              <div className="queued-actions">
                {queue.queued.held && (
                  <button
                    type="button"
                    className="button primary"
                    disabled={busy || working}
                    title={
                      busy
                        ? `Wait for ${toolLabel(state.tool)} to finish`
                        : undefined
                    }
                    onClick={() => {
                      const text = queue.queued!.text;
                      void perform(async () => {
                        await api.SendChat(state.id, text);
                        queue.clear();
                        setRetry((n) => n + 1);
                        following.current = true;
                        setAwayFromLatest(false);
                      });
                    }}
                  >
                    Send now
                  </button>
                )}
                <button
                  type="button"
                  className="button secondary"
                  disabled={working}
                  title="Move it back into the message box"
                  onClick={() => {
                    const text = queue.queued!.text;
                    queue.clear();
                    reuse(text);
                  }}
                >
                  Edit
                </button>
                <button
                  type="button"
                  className="button secondary"
                  disabled={working}
                  onClick={queue.clear}
                >
                  Discard
                </button>
              </div>
            </div>
          )}
          {!state.review && queueNote && (
            <p className="queue-note" role="status">
              {queueNote}
            </p>
          )}
          {!state.review && (
            <textarea
              ref={input}
              aria-label="Message agent"
              aria-controls={skills.listId || mentions.listId}
              aria-activedescendant={
                skills.activeOption || mentions.activeOption
              }
              placeholder={`Message ${toolLabel(state.tool)} — @ for a file, / for a skill`}
              value={prompt}
              disabled={working}
              onChange={(e) => {
                updatePrompt(e.target.value);
                setRecalled(-1);
                trackCaret();
              }}
              onPaste={(e) => {
                // A pasted screenshot is attached; pasted text goes in as text.
                const pasted = imageFiles(e.clipboardData);
                if (!pasted.length) return;
                e.preventDefault();
                void attachments.add(pasted);
              }}
              onKeyUp={trackCaret}
              onClick={trackCaret}
              onKeyDown={(e) => {
                if (skills.onKeyDown(e)) return;
                if (mentions.onKeyDown(e)) return;
                if (recall(e)) return;
                if (
                  e.key === "Enter" &&
                  !e.shiftKey &&
                  !e.nativeEvent.isComposing &&
                  e.nativeEvent.keyCode !== 229
                ) {
                  e.preventDefault();
                  void send();
                }
              }}
            />
          )}
          {!state.review && (prompt || draftError) && (
            <div className="draft-status">
              <span
                role={draftError ? "alert" : "status"}
                className={draftError ? "draft-error" : ""}
              >
                {draftError ||
                  (restored
                    ? "Draft restored from this Mac"
                    : "Draft saved on this Mac")}
              </span>
              {!!prompt && (
                <button
                  type="button"
                  disabled={working}
                  onClick={() => updatePrompt("")}
                >
                  Discard draft
                </button>
              )}
            </div>
          )}
          {!state.review && (
            <AttachedImages
              images={attachments.images}
              error={attachments.error}
              disabled={working}
              onRemove={attachments.remove}
            />
          )}
          <div className="composer-footer">
            {!state.review && (
              <>
                <Popover
                  label="Add images or files"
                  title="Attach images or mention project files. You can also paste or drop images."
                  buttonClassName="pill-button icon-pill"
                  panelLabel="Add to the message"
                  role="menu"
                  disabled={working}
                  button={<Icon name="plus" size={15} />}
                >
                  {(close) => (
                    <>
                      <button
                        type="button"
                        role="menuitem"
                        className="popover-item"
                        onClick={() => {
                          close();
                          imagePicker.current?.click();
                        }}
                      >
                        <Icon name="image" size={15} />
                        <strong>Attach images…</strong>
                        <small>or paste, or drop them here</small>
                      </button>
                      <button
                        type="button"
                        role="menuitem"
                        className="popover-item"
                        onClick={() => {
                          close();
                          void mentionFiles();
                        }}
                      >
                        <Icon name="paperclip" size={15} />
                        <strong>Mention files…</strong>
                        <small>adds their paths; or type @</small>
                      </button>
                    </>
                  )}
                </Popover>
                <input
                  ref={imagePicker}
                  type="file"
                  accept="image/png,image/jpeg,image/gif,image/webp"
                  multiple
                  hidden
                  aria-label="Choose images to attach"
                  onChange={(e) => {
                    void attachments.add([...(e.target.files || [])]);
                    e.target.value = "";
                  }}
                />
              </>
            )}
            {!state.review && (
              <ModePicker
                chatId={state.id}
                mode={state.mode || ""}
                disabled={busy || working}
                onSaved={() => setRetry((n) => n + 1)}
              />
            )}
            {!state.review && state.tool === "codex" && (
              <WebSearchPicker
                chatId={state.id}
                mode={state.webSearch || ""}
                disabled={busy || working}
                onSaved={() => setRetry((n) => n + 1)}
              />
            )}
            {!state.review && (
              <ModelPicker
                id={state.id}
                settings={state.settings}
                compact
                active={active}
                disabled={busy || actionWorking}
                onPending={settingsPending}
                onSaved={() => setRetry((n) => n + 1)}
              />
            )}
            <span
              className="composer-hint"
              title={
                state.review
                  ? undefined
                  : `Enter ${busy ? "queues" : "sends"} the message; Shift+Enter adds a line.`
              }
            >
              {state.review ? (
                "Reviewing a fixed snapshot of your uncommitted changes."
              ) : (
                <>
                  <kbd>⏎</kbd> {busy ? "queue" : "send"}
                </>
              )}
            </span>
            {!state.review && <ContextMeter context={state.context} />}
            {busy && !state.review && !!prompt.trim() && (
              <button
                type="submit"
                className="button secondary"
                title={`Send this message when ${toolLabel(state.tool)} finishes (Enter)`}
                disabled={working}
              >
                Queue
              </button>
            )}
            {busy ? (
              <button
                type="button"
                className="button secondary stop"
                title="Ends the response. Edits already made are kept."
                disabled={working || state.status === "stopping"}
                onClick={() => setConfirmStop(true)}
              >
                <Icon name="stop" size={13} /> Stop response
              </button>
            ) : (
              !state.review && (
                <button
                  type="submit"
                  className="button primary send"
                  aria-label="Send"
                  title="Send (Enter)"
                  disabled={!prompt.trim() || working}
                >
                  <Icon name="arrow-up" size={17} />
                </button>
              )
            )}
          </div>
        </form>
        {draft && (
          <ReviewPreview
            draft={draft}
            settings={state.reviewerSettings}
            onSettingsPending={settingsPending}
            onSettingsSaved={() => setRetry((n) => n + 1)}
            error={error}
            busy={working}
            onCancel={() => {
              if (!working) setDraft(undefined);
            }}
            onStart={() =>
              void perform(async () => {
                const review = await api.StartReview(draft.id);
                setDraft(undefined);
                onOpen(review);
              })
            }
          />
        )}
      </div>
      {restoring && (
        <ConfirmDialog
          title={
            restoring.point.kind === "turn"
              ? "Restore files to before this turn?"
              : "Undo this?"
          }
          confirmLabel={
            restoring.plan?.length
              ? `Restore ${restoring.plan.length} ${restoring.plan.length === 1 ? "file" : "files"}`
              : "Restore"
          }
          cancelLabel="Cancel"
          busy={restoreBusy}
          confirmDisabled={!restoring.plan?.length}
          onCancel={() => setRestoring(undefined)}
          onConfirm={() => void restoreNow()}
        >
          {restoring.error ? (
            <p role="alert">{restoring.error}</p>
          ) : !restoring.plan ? (
            <p>Reading which files would change…</p>
          ) : !restoring.plan.length ? (
            <p>Nothing to restore: the files are already as they were then.</p>
          ) : (
            <>
              <p>These files go back to how they were:</p>
              <ul className="restore-list">
                {restoring.plan.slice(0, 12).map((change) => (
                  <li key={change.path}>
                    <code>{change.path}</code>
                    <small>
                      {change.change === "remove"
                        ? "removed (new since)"
                        : "restored"}
                    </small>
                  </li>
                ))}
              </ul>
              {restoring.plan.length > 12 && (
                <p>and {restoring.plan.length - 12} more.</p>
              )}
              <p>
                The files as they are now are kept first, so you can undo this.
                Nothing is committed, and staged changes stay staged.
              </p>
            </>
          )}
        </ConfirmDialog>
      )}
      {comparing && state.comparison && (
        <CompareDialog
          id={state.comparison}
          onClose={() => setComparing(false)}
          onOpenChat={(id) => {
            setComparing(false);
            onSelect(id);
          }}
          onRemoved={(ids) => onClosed?.(ids)}
        />
      )}
      {confirmStop && (
        <ConfirmDialog
          title={`Stop ${toolLabel(state.tool)}?`}
          confirmLabel="Stop response"
          cancelLabel="Keep running"
          busy={working}
          onCancel={() => setConfirmStop(false)}
          onConfirm={() =>
            void perform(async () => {
              try {
                await api.StopChat(state.id);
                setRetry((n) => n + 1);
              } finally {
                setConfirmStop(false);
              }
            })
          }
        >
          <p>
            The response ends now. Files the agent already edited stay as they
            are, and the conversation is kept so you can continue it.
          </p>
        </ConfirmDialog>
      )}
      {turnDiff && (
        <ChangesDialog
          title="Changes since this message"
          note={`${state.project} · from just before this turn to now · read-only`}
          load={() => api.TurnDiff(state.id, turnDiff.id)}
          empty="Nothing has changed since this message."
          file={turnFile}
          onFile={setTurnFile}
          onClose={() => {
            setTurnDiff(undefined);
            setTurnFile("");
          }}
        />
      )}
      {showDetails && (
        <DetailsPanel
          chat={state}
          session={session}
          summary={changes.summary}
          error={changes.error}
          drawer={narrow}
          onRefresh={changes.refresh}
          onClose={closeDetails}
          handOff={
            state.review
              ? undefined
              : { to: other, blocked: handOffBlocked, run: handOff }
          }
          onDraft={state.review ? undefined : draftAppend}
        />
      )}
    </section>
  );
}

function ApprovalCard({
  approval,
  onAnswer,
  onOption,
}: {
  approval: ChatApproval;
  onAnswer: (allow: boolean, answers: Record<string, string>) => Promise<void>;
  onOption: (option: string) => Promise<void>;
}) {
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const operation = useRef(false);
  async function answer(allow: boolean) {
    if (operation.current) return;
    operation.current = true;
    setBusy(true);
    setError("");
    try {
      await onAnswer(allow, answers);
    } catch (e) {
      setError(errorText(e));
      operation.current = false;
      setBusy(false);
    }
  }
  async function choose(option: string) {
    if (operation.current) return;
    operation.current = true;
    setBusy(true);
    setError("");
    try {
      await onOption(option);
    } catch (e) {
      setError(errorText(e));
      operation.current = false;
      setBusy(false);
    }
  }
  const request = requestSummary(approval.detail);
  return (
    <section
      className="approval-card"
      aria-label={approval.title}
      onKeyDown={(e) => {
        // Shortcuts apply only while focus is inside the card, and never
        // while typing an answer.
        const typing =
          e.target instanceof HTMLInputElement ||
          e.target instanceof HTMLSelectElement;
        if (e.key === "Escape" && !busy) {
          e.preventDefault();
          void answer(false);
        } else if (
          e.key === "Enter" &&
          !typing &&
          !(e.target instanceof HTMLButtonElement) &&
          !busy &&
          !approval.questions.length
        ) {
          e.preventDefault();
          void answer(true);
        }
      }}
    >
      <h3>
        <Icon name="alert" size={17} />
        {approval.title}
      </h3>
      {approval.plan ? (
        // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
        <div className="approval-plan markdown" tabIndex={0} aria-label="Plan">
          <SafeMarkdown text={approval.plan} />
        </div>
      ) : (
        approval.detail && (
          <>
            {request.note && <p className="approval-note">{request.note}</p>}
            {/* Focusable so a long command can be scrolled from the keyboard,
                and so the card's shortcuts can be used without the pointer. */}
            {/* eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex */}
            <pre tabIndex={0} aria-label="Action details">
              {request.text}
            </pre>
          </>
        )
      )}
      {approval.questions.map((question) => (
        <label className="approval-question" key={question.id}>
          <strong>{question.text}</strong>
          {question.options.length > 0 && (
            <select
              aria-label={`Options for ${question.text}`}
              value={
                question.options.includes(answers[question.id])
                  ? answers[question.id]
                  : ""
              }
              onChange={(e) =>
                setAnswers((v) => ({ ...v, [question.id]: e.target.value }))
              }
            >
              <option value="">Choose or write an answer…</option>
              {question.options.map((option) => (
                <option key={option}>{option}</option>
              ))}
            </select>
          )}
          <input
            aria-label={question.text}
            type={question.secret ? "password" : "text"}
            value={answers[question.id] || ""}
            onChange={(e) =>
              setAnswers((v) => ({ ...v, [question.id]: e.target.value }))
            }
          />
        </label>
      ))}
      {error && <p role="alert">{error}</p>}
      <div className="approval-actions">
        <button
          className="button primary"
          disabled={
            busy || approval.questions.some((q) => !answers[q.id]?.trim())
          }
          onClick={() => void answer(true)}
        >
          {busy
            ? "Sending…"
            : approval.questions.length
              ? "Send answers"
              : approval.plan
                ? "Approve plan"
                : "Allow once"}
        </button>
        {approval.options?.map((option) => (
          <button
            key={option.id}
            className="button secondary"
            disabled={busy}
            title={option.detail}
            onClick={() => void choose(option.id)}
          >
            {option.label}
          </button>
        ))}
        <button
          className="button secondary"
          disabled={busy}
          onClick={() => void answer(false)}
        >
          {approval.plan ? "Keep planning" : "Deny"} <kbd>Esc</kbd>
        </button>
      </div>
      {approval.options?.map(
        (option) =>
          option.detail && (
            <p className="approval-option-note" key={option.id}>
              <strong>{option.label}:</strong> {option.detail}
            </p>
          ),
      )}
    </section>
  );
}

function ReviewPreview({
  draft,
  error,
  busy,
  settings,
  onSettingsPending,
  onSettingsSaved,
  onCancel,
  onStart,
}: {
  draft: ReviewDraft;
  settings?: import("./api").ModelSettings;
  onSettingsPending: (value: boolean) => void;
  onSettingsSaved: () => void;
  error: string;
  busy: boolean;
  onCancel: () => void;
  onStart: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [file, setFile] = useState("");
  useEffect(() => {
    const el = dialog.current;
    if (el?.showModal) el.showModal();
    else el?.setAttribute("open", "");
    return () => el?.close?.();
  }, []);
  return (
    <dialog
      className="review-dialog"
      ref={dialog}
      onCancel={(e) => {
        e.preventDefault();
        if (!busy) onCancel();
      }}
      aria-labelledby="review-preview-title"
    >
      <header>
        <div className="eyebrow">REVIEW WITH ANOTHER AGENT</div>
        <h2 id="review-preview-title">
          Send this diff to {toolLabel(draft.tool)}?
        </h2>
        <p>
          All current staged, unstaged and eligible untracked changes in this
          Git working tree. This can include changes made outside this
          conversation.
        </p>
        <small>
          {draft.snapshot.files.length}{" "}
          {draft.snapshot.files.length === 1 ? "file" : "files"} · Base{" "}
          {draft.snapshot.base.slice(0, 10)} · Snapshot{" "}
          {draft.snapshot.hash.slice(0, 12)}
        </small>
      </header>
      <ModelPicker
        id={draft.sourceId}
        settings={settings || draft.settings}
        reviewer
        disabled={busy}
        onPending={onSettingsPending}
        onSaved={onSettingsSaved}
      />
      {!!draft.snapshot.warnings.length && (
        <div className="review-warnings">
          {draft.snapshot.warnings.map((w) => (
            <p key={w}>{w}</p>
          ))}
        </div>
      )}
      <DiffViewer
        snapshot={draft.snapshot}
        selectedFile={file}
        selectedLine={0}
        onFile={setFile}
      />
      {error && (
        <div className="notice error" role="alert">
          {error}
        </div>
      )}
      <footer>
        <span>
          The other agent receives this snapshot for analysis. No fixes are
          applied during review.
        </span>
        <button className="button subtle" disabled={busy} onClick={onCancel}>
          Cancel
        </button>
        <button className="button accent" disabled={busy} onClick={onStart}>
          {busy
            ? "Starting review…"
            : `Send snapshot to ${toolLabel(draft.tool)}`}
        </button>
      </footer>
    </dialog>
  );
}
