import { useState } from "react";
import Icon from "../Icon";
import {
  api,
  errorText,
  toolLabel,
  type AgentServer,
  type AgentTools,
} from "../api";

const STATE_ICON: Record<AgentServer["state"], "check" | "alert" | "clock"> = {
  connected: "check",
  failed: "alert",
  "needs-auth": "alert",
  pending: "clock",
  off: "clock",
  unknown: "clock",
};

function stateOrder(state: AgentServer["state"]) {
  return [
    "failed",
    "needs-auth",
    "pending",
    "connected",
    "unknown",
    "off",
  ].indexOf(state);
}

/**
 * The MCP servers the conversation's agent has in this project, and whether
 * they work, read from the agent when asked. Checking starts the servers, so
 * it never runs on its own.
 */
export default function AgentToolsSection({
  chatId,
  tool,
}: {
  chatId: string;
  tool: string;
}) {
  const [tools, setTools] = useState<AgentTools>();
  const [checking, setChecking] = useState(false);
  const [error, setError] = useState("");
  const agent = toolLabel(tool);
  async function check() {
    setChecking(true);
    setError("");
    try {
      setTools(await api.AgentTools(chatId));
    } catch (e) {
      setError(errorText(e));
    } finally {
      setChecking(false);
    }
  }
  const servers = [...(tools?.servers || [])].sort(
    (a, b) => stateOrder(a.state) - stateOrder(b.state),
  );
  const problems = servers.filter(
    (s) => s.state === "failed" || s.state === "needs-auth",
  ).length;
  return (
    <section className="agent-tools" aria-label="Agent tools">
      <h3 className="details-section">
        Agent tools
        <button
          type="button"
          className="text-button"
          disabled={checking}
          title={`Ask ${agent} which MCP servers it has in this project. This starts them, as a message would; nothing is changed.`}
          onClick={check}
        >
          {checking ? "Checking…" : tools ? "Check again" : "Check tools"}
        </button>
      </h3>
      {error && (
        <p className="details-note error" role="alert">
          {error}
        </p>
      )}
      {!tools && !error && (
        <p className="details-note">
          The MCP servers {agent} can use here, and whether they work.
        </p>
      )}
      {tools && !servers.length && (
        <p className="details-note" role="status">
          {agent} has no MCP servers in this project.
        </p>
      )}
      {!!servers.length && (
        <>
          <p className="details-note" role="status">
            {servers.length} {servers.length === 1 ? "server" : "servers"}
            {problems ? `, ${problems} not working` : ""}.
          </p>
          <ul className="agent-servers">
            {servers.map((server) => (
              <li
                key={server.name}
                className={`${server.state} ${(server.detail || "").length > 24 ? "long" : ""}`}
              >
                <span className="agent-server-name" title={server.name}>
                  {server.name}
                </span>
                <span className="agent-server-state" title={server.detail}>
                  <Icon name={STATE_ICON[server.state]} size={12} />
                  {server.detail || server.state}
                </span>
                {!!server.tools?.length && (
                  <details>
                    <summary>Tools</summary>
                    <p>
                      {server.tools.join(", ")}
                      {server.more ? `, and ${server.more} more` : ""}
                    </p>
                  </details>
                )}
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  );
}
