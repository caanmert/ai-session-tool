import { useEffect, useState } from "react";
import AgentMark from "../AgentMark";
import {
  notificationsEnabled,
  notifyOnlyWhenNeeded,
  setNotificationsEnabled,
  setNotifyOnlyWhenNeeded,
} from "../shell/attention";
import { AppearanceSetting, type AppearanceControl } from "../Appearance";
import Icon from "../Icon";
import SkillsSection from "./SkillsSection";
import SavedPromptsSection from "./SavedPromptsSection";
import SchedulesSection from "./SchedulesSection";
import { type Project, toolLabel, type AgentHealth, type Tool } from "../api";

const install: Record<string, string> = {
  claude: "npm install -g @anthropic-ai/claude-code",
  codex: "npm install -g @openai/codex",
};

const shortcuts: [string, string][] = [
  ["⌘N", "New task"],
  ["⌘K", "Search conversations, projects and actions"],
  ["⌘1 – ⌘9", "Switch to an open conversation by its place in the sidebar"],
  ["⌘,", "Open Settings"],
  ["⌘F", "Find in the open conversation"],
  ["⌘B", "Narrow or expand the sidebar"],
  ["⌘\\", "Show or hide changed files and session details"],
  ["Enter", "Send the message"],
  ["Shift Enter", "New line in the message"],
  ["@", "Find a file in the project and add its path to the message"],
  ["/", "Find a saved prompt or one of the agent's skills"],
  ["↑", "Recall your earlier messages (in an empty message box)"],
  ["↑ ↓", "Move through lists and open conversations"],
  ["Esc", "Close a dialog, or deny a request while its card has focus"],
];

export default function Settings({
  appearance,
  tools,
  health,
  checking,
  onCheck,
  projects = [],
  project = "",
  onOpenChat,
  focusSection,
}: {
  appearance: AppearanceControl;
  tools: Tool[];
  health: AgentHealth[];
  checking: boolean;
  onCheck: () => void;
  /** For the Skills section: which project's skills to list. */
  projects?: Project[];
  project?: string;
  /** Opens a conversation, such as one a scheduled task started. */
  onOpenChat?: (chatId: string) => void;
  /** A section to scroll to, such as "schedules", with a changing key. */
  focusSection?: { id: string; at: number };
}) {
  const [notify, setNotify] = useState(notificationsEnabled);
  const [needsOnly, setNeedsOnly] = useState(notifyOnlyWhenNeeded);
  const [notifyError, setNotifyError] = useState("");
  return (
    <div className="settings" role="region" aria-label="Settings">
      <SettingsIndex focus={focusSection} />
      <section aria-labelledby="settings-look">
        {/* The control below carries the "Appearance" label. */}
        <h2 id="settings-look">Look</h2>
        <div className="setting-group">
          <AppearanceSetting {...appearance} />
          <p className="setting-note">
            System follows your Mac and changes with it.
          </p>
        </div>
      </section>

      <section aria-labelledby="settings-notifications">
        <h2 id="settings-notifications">Notifications</h2>
        <div className="setting-group">
          <label className="setting-check">
            <input
              type="checkbox"
              checked={notify}
              onChange={(e) => {
                setNotify(e.target.checked);
                setNotifyError(
                  setNotificationsEnabled(e.target.checked)
                    ? ""
                    : "This choice could not be saved and applies until you quit.",
                );
              }}
            />
            <span>
              Notify me when an agent needs me or finishes
              <small>
                Only while ais is in the background; clicking one opens its
                conversation. macOS asks for permission the first time. The Dock
                icon shows the count either way.
              </small>
            </span>
          </label>
          <label className="setting-check nested">
            <input
              type="checkbox"
              checked={needsOnly}
              disabled={!notify}
              onChange={(e) => {
                setNeedsOnly(e.target.checked);
                setNotifyError(
                  setNotifyOnlyWhenNeeded(e.target.checked)
                    ? ""
                    : "This choice could not be saved and applies until you quit.",
                );
              }}
            />
            <span>
              Only when an agent needs me
              <small>
                An approval, a question or an error; not when a response just
                finishes.
              </small>
            </span>
          </label>
          {notifyError && (
            <p className="setting-note" role="status">
              {notifyError}
            </p>
          )}
        </div>
      </section>

      <section aria-labelledby="settings-agents">
        <div className="settings-heading">
          <h2 id="settings-agents">Agents</h2>
          <button
            className="button secondary"
            disabled={checking}
            onClick={onCheck}
          >
            <Icon name="refresh" size={14} />
            {checking ? "Checking…" : "Check again"}
          </button>
        </div>
        <p className="setting-note">
          ais uses the agents installed on this Mac and their own sign-in. It
          looks in your PATH, ~/.local/bin, /opt/homebrew/bin and
          /usr/local/bin.
        </p>
        {tools.map((tool) => {
          const state = health.find((item) => item.tool === tool.name);
          return (
            <div
              className={`setting-group agent-card ${tool.available && !state?.problem ? "" : "missing"}`}
              key={tool.name}
            >
              <AgentMark tool={tool.name} />
              <div>
                <strong>{toolLabel(tool.name)}</strong>
                <code>{tool.available ? tool.path : install[tool.name]}</code>
                {!tool.available && (
                  <small>
                    Install it with the command above, sign in from its command
                    line, then check again.
                  </small>
                )}
                {tool.available && state?.version && (
                  <small>Version {state.version}</small>
                )}
                {state?.problem && (
                  <small className="problem" role="alert">
                    {state.problem} Updating ais, or installing version{" "}
                    {state.tested} of this agent, should fix it.
                  </small>
                )}
              </div>
              {tool.available && state?.problem ? (
                <span className="chip status error">
                  <Icon name="alert" size={12} /> Not compatible
                </span>
              ) : tool.available ? (
                <span className="chip status found">
                  <Icon name="check" size={12} /> Installed
                </span>
              ) : (
                <span className="chip status error">
                  <Icon name="alert" size={12} /> Not found
                </span>
              )}
            </div>
          );
        })}
        <p className="setting-note">
          Checking asks each agent for its version and options only. It does not
          contact a model.
        </p>
      </section>

      <SkillsSection tools={tools} projects={projects} project={project} />

      <SavedPromptsSection />

      <SchedulesSection
        tools={tools}
        projects={projects}
        onOpenChat={onOpenChat}
      />

      <section aria-labelledby="settings-shortcuts">
        <h2 id="settings-shortcuts">Keyboard shortcuts</h2>
        <dl className="setting-group shortcut-list">
          {shortcuts.map(([keys, action]) => (
            <div key={keys}>
              <dt>{action}</dt>
              <dd>
                <kbd>{keys}</kbd>
              </dd>
            </div>
          ))}
        </dl>
      </section>

      <section aria-labelledby="settings-data">
        <h2 id="settings-data">Your data</h2>
        <p className="setting-note">
          Conversations opened here, drafts and review snapshots are stored on
          this Mac in the ais data folder. Each agent keeps its own transcripts
          in its own store. Nothing is sent to an agent until you start a task.
        </p>
      </section>
    </div>
  );
}

/** Settings' sections, in page order, for the index and for ⌘K. */
export const SETTINGS_SECTIONS: [string, string][] = [
  ["look", "Look"],
  ["notifications", "Notifications"],
  ["agents", "Agents"],
  ["skills", "Skills"],
  ["prompts", "Saved prompts"],
  ["schedules", "Scheduled tasks"],
  ["shortcuts", "Keyboard shortcuts"],
  ["data", "Your data"],
];

function sectionElement(id: string) {
  return document.getElementById(`settings-${id}`)?.closest("section");
}

/**
 * Links to each section of Settings at the top of the page. The section in
 * view is marked as it scrolls, and ⌘K can open a section directly.
 */
function SettingsIndex({ focus }: { focus?: { id: string; at: number } }) {
  const [current, setCurrent] = useState("look");
  useEffect(() => {
    if (!focus) return;
    sectionElement(focus.id)?.scrollIntoView?.({ block: "start" });
    setCurrent(focus.id);
  }, [focus]);
  // The current section is the last one whose heading has reached the
  // top of the page, just under the index.
  useEffect(() => {
    const page = document.querySelector<HTMLElement>(".settings");
    if (!page) return;
    const mark = () => {
      const top = page.getBoundingClientRect().top + 90;
      let at = SETTINGS_SECTIONS[0][0];
      for (const [id] of SETTINGS_SECTIONS) {
        const el = sectionElement(id);
        if (el && el.getBoundingClientRect().top <= top) at = id;
      }
      // At the bottom, the last section counts even if its heading is lower.
      if (page.scrollTop + page.clientHeight >= page.scrollHeight - 2)
        at = SETTINGS_SECTIONS.at(-1)![0];
      setCurrent(at);
    };
    page.addEventListener("scroll", mark, { passive: true });
    return () => page.removeEventListener("scroll", mark);
  }, []);
  return (
    <nav className="settings-index" aria-label="Settings sections">
      {SETTINGS_SECTIONS.map(([id, label]) => (
        <a
          key={id}
          href={`#settings-${id}`}
          aria-current={current === id ? "true" : undefined}
          onClick={(e) => {
            e.preventDefault();
            sectionElement(id)?.scrollIntoView?.({ block: "start" });
            setCurrent(id);
          }}
        >
          {label}
        </a>
      ))}
    </nav>
  );
}
