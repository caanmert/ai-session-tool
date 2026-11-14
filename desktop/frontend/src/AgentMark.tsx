// Agent identity: a tinted tile with a dot. Claude Code's dot is square and
// Codex's is round, so the two stay distinguishable without color.
export default function AgentMark({
  tool,
  small = false,
}: {
  tool: string;
  small?: boolean;
}) {
  return (
    <span
      className={`agent-mark ${tool}${small ? " small" : ""}`}
      aria-hidden="true"
    >
      <span className="agent-dot" />
    </span>
  );
}
