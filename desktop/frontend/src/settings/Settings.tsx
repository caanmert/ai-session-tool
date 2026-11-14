import AgentMark from "../AgentMark";
import { AppearanceSetting, type AppearanceControl } from "../Appearance";
import Icon from "../Icon";
import { toolLabel, type AgentHealth, type Tool } from "../api";

const install: Record<string, string> = {
  claude: "npm install -g @anthropic-ai/claude-code",
  codex: "npm install -g @openai/codex",
};

const shortcuts: [string, string][] = [
  ["⌘N", "New task"],
  ["⌘K", "Search conversations, projects and actions"],
  ["⌘\\", "Show or hide changed files (narrow windows)"],
  ["Enter", "Send the message"],
  ["Shift Enter", "New line in the message"],
  ["↑ ↓", "Move through lists and open conversations"],
  ["Esc", "Close a dialog, or deny a request while its card has focus"],
];

export default function Settings({
  appearance,
  tools,
  health,
  checking,
  onCheck,
}: {
  appearance: AppearanceControl;
  tools: Tool[];
  health: AgentHealth[];
  checking: boolean;
  onCheck: () => void;
}) {
  return (
    <div className="settings" role="region" aria-label="Settings">
      <section>
        {/* The control below carries the "Appearance" label. */}
        <h2>Look</h2>
        <div className="setting-group">
          <AppearanceSetting {...appearance} />
          <p className="setting-note">
            System follows your Mac and changes with it.
          </p>
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
                  <Icon name="check" size={12} /> Found
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
