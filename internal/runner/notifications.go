package runner

import "github.com/caanmert/ai-session-tool/internal/model"

// Claude captures hook stdout, and hooks may run without a controlling tty.
// Resolve the originating pane through its inherited tmux socket/pane identity
// and ring that pane's terminal. Never fall back to tmux's current pane.
// The hook emits no model-visible output or approval decision. A closed pane
// is harmless: notification failures must not interrupt the agent.
const claudeNotificationSettings = `{"hooks":{"Notification":[{"matcher":"^(permission_prompt|idle_prompt|elicitation_dialog|elicitation_url_dialog)$","hooks":[{"type":"command","command":"{ [ -n \"$TMUX\" ] && [ -n \"$TMUX_PANE\" ] && pane_tty=$(tmux display-message -p -t \"$TMUX_PANE\" '#{pane_tty}') && [ -c \"$pane_tty\" ] && printf '\\007' > \"$pane_tty\"; } 2>/dev/null || true","timeout":5}]}]}}`

// Only use events the agent reports itself. In particular, do not treat a
// quiet terminal, a tool call, or a stoppable Stop hook as confirmed idle state.
// CLI arguments scope configuration to this process; user files stay intact.
func notificationArgs(tool model.Tool, enabled bool) []string {
	if !enabled {
		return nil
	}
	switch tool {
	case model.ToolCodex:
		return []string{
			"-c", `tui.notifications=["agent-turn-complete","approval-requested"]`,
			"-c", `tui.notification_method="bel"`,
			"-c", `tui.notification_condition="always"`,
		}
	case model.ToolClaude:
		return []string{"--settings", claudeNotificationSettings}
	default:
		return nil
	}
}

func onOff(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}
