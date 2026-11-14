// Agent identity. Claude Code's dot is square and Codex's is round, so the two
// stay distinguishable without color. By default the dot sits on a tinted
// tile; `dot` draws the bare dot for use inside a line of text.
export default function AgentMark({
  tool,
  small = false,
  dot = false,
}: {
  tool: string;
  small?: boolean;
  dot?: boolean;
}) {
  if (dot) return <span className={`agent-dot ${tool}`} aria-hidden="true" />;
  return (
    <span
      className={`agent-mark ${tool}${small ? " small" : ""}`}
      aria-hidden="true"
    >
      <span className="agent-dot" />
    </span>
  );
}
