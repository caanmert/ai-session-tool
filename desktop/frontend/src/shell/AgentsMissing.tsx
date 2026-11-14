import AgentMark from "../AgentMark";
import Icon from "../Icon";
import { toolLabel, type Tool } from "../api";

// Install commands published by each agent's maker.
const install: Record<string, string> = {
  claude: "npm install -g @anthropic-ai/claude-code",
  codex: "npm install -g @openai/codex",
};

/** Shown instead of the workspace when no agent's command-line tool is found. */
export default function AgentsMissing({
  tools,
  checking,
  onCheck,
}: {
  tools: Tool[];
  checking: boolean;
  onCheck: () => void;
}) {
  return (
    <div className="empty-state" role="region" aria-label="Agent setup">
      <h2>No coding agent found</h2>
      <p>
        ais runs the agents already installed on this Mac. It looked in your
        PATH, ~/.local/bin, /opt/homebrew/bin and /usr/local/bin. Saved
        conversations can still be browsed.
      </p>
      <ul>
        {tools.map((tool) => (
          <li key={tool.name}>
            <AgentMark tool={tool.name} small />
            <strong>{toolLabel(tool.name)}</strong>
            {tool.available ? (
              <span className="state">
                <Icon name="check" size={13} /> Found
              </span>
            ) : (
              <span className="state missing">
                <Icon name="alert" size={13} /> Not found
              </span>
            )}
            <code>{tool.available ? tool.path : install[tool.name]}</code>
          </li>
        ))}
      </ul>
      <p>
        Install one, sign in from its own command line, then check again.
        Nothing is sent to an agent until you start a task.
      </p>
      <div className="actions">
        <button
          className="button primary"
          disabled={checking}
          onClick={onCheck}
        >
          {checking ? "Checking…" : "Check again"}
        </button>
      </div>
    </div>
  );
}
