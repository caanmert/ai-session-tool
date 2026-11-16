import { useEffect, useRef, useState } from "react";
import Icon from "../Icon";
import {
  api,
  errorText,
  type ProjectCommand,
  type ProjectRunState,
} from "../api";

/** How a finished run ended, in words. */
export function runOutcome(run: ProjectRunState) {
  if (run.running) return "Running…";
  if (run.stopped) return "Stopped";
  if (run.exitCode === 0) return "Finished";
  if (run.exitCode < 0) return "Could not finish";
  return `Failed with exit code ${run.exitCode}`;
}

/**
 * Runs one of the project's commands beside the conversation: a script the
 * project offers or a typed command. Nothing runs until Run is pressed; one
 * command runs at a time and can be stopped.
 */
export default function RunPanel({ chatId }: { chatId: string }) {
  const [commands, setCommands] = useState<ProjectCommand[]>([]);
  const [command, setCommand] = useState("");
  const [run, setRun] = useState<ProjectRunState>();
  const [error, setError] = useState("");
  const [starting, setStarting] = useState(false);
  const output = useRef<HTMLPreElement>(null);
  useEffect(() => {
    let active = true;
    setRun(undefined);
    setCommand("");
    api
      .ProjectCommands(chatId)
      .then((found) => active && setCommands(found || []))
      .catch(() => active && setCommands([]));
    // A run started earlier in this conversation is shown again.
    api
      .ProjectRun(chatId)
      .then((last) => {
        if (!active) return;
        setRun(last);
        setCommand(last.command);
      })
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [chatId]);
  const running = !!run?.running;
  // Follow the output while it runs.
  useEffect(() => {
    if (!running) return;
    const timer = setInterval(() => {
      api
        .ProjectRun(chatId)
        .then(setRun)
        .catch(() => {});
    }, 400);
    return () => clearInterval(timer);
  }, [chatId, running]);
  useEffect(() => {
    const box = output.current;
    if (box) box.scrollTop = box.scrollHeight;
  }, [run?.output]);
  async function start() {
    const text = command.trim();
    if (!text || running || starting) return;
    setStarting(true);
    setError("");
    try {
      setRun(await api.StartProjectCommand(chatId, text));
    } catch (e) {
      setError(errorText(e));
    } finally {
      setStarting(false);
    }
  }
  async function stop() {
    try {
      await api.StopProjectCommand(chatId);
      setRun(await api.ProjectRun(chatId));
    } catch (e) {
      setError(errorText(e));
    }
  }
  const outcome = run && runOutcome(run);
  return (
    <section className="run-panel" aria-label="Run a command">
      <h3 className="details-section">Run</h3>
      {commands.length > 0 && (
        <div
          className="run-commands"
          role="group"
          aria-label="Project commands"
        >
          {commands.map((c) => (
            <button
              key={c.source + c.command}
              className="run-chip"
              title={`${c.command} (from ${c.source})`}
              aria-pressed={command === c.command}
              disabled={running}
              onClick={() => setCommand(c.command)}
            >
              {c.label}
            </button>
          ))}
        </div>
      )}
      <form
        className="run-form"
        onSubmit={(e) => {
          e.preventDefault();
          void start();
        }}
      >
        <input
          aria-label="Command to run in the project folder"
          placeholder={
            commands.length
              ? "Choose above or type a command"
              : "Type a command"
          }
          value={command}
          spellCheck={false}
          disabled={running}
          onChange={(e) => setCommand(e.target.value)}
        />
        {running ? (
          <button
            type="button"
            className="button secondary"
            onClick={() => void stop()}
          >
            <Icon name="stop" size={13} /> Stop
          </button>
        ) : (
          <button
            type="submit"
            className="button secondary"
            disabled={!command.trim() || starting}
          >
            <Icon name="play" size={13} /> Run
          </button>
        )}
      </form>
      {error && (
        <p className="details-note error" role="alert">
          {error}
        </p>
      )}
      {run && (
        <div className="run-result">
          <p
            className={`run-status ${run.running ? "running" : run.exitCode === 0 && !run.stopped ? "ok" : "failed"}`}
            role="status"
          >
            <code>{run.command}</code> · {outcome}
          </p>
          <pre ref={output} className="run-output" aria-label="Output">
            {run.shortened && "[Earlier output is not kept]\n"}
            {run.output || (run.running ? "" : "No output.")}
          </pre>
        </div>
      )}
    </section>
  );
}
