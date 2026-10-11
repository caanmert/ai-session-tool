import { useEffect, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { api, errorText, toolLabel, type LiveSession } from "./api";
import "@xterm/xterm/css/xterm.css";

type Props = {
  session: LiveSession;
  active: boolean;
  onExit: (id: string, code: number) => void;
  onClose: (id: string) => void;
};

// Keep visited terminals mounted while switching tabs: their screen, scrollback,
// and escape-sequence parser state belong to the running process.
export default function LiveTerminal(props: Props) {
  const [visited, setVisited] = useState(props.active);
  useEffect(() => {
    if (props.active) setVisited(true);
  }, [props.active]);
  return (
    <section
      className="live-panel"
      hidden={!props.active}
      aria-label={`${toolLabel(props.session.tool)} live terminal`}
    >
      {visited && <TerminalScreen {...props} />}
    </section>
  );
}

function TerminalScreen({ session, active, onExit, onClose }: Props) {
  const container = useRef<HTMLDivElement>(null);
  const controls = useRef<{ term: Terminal; fit: () => void } | null>(null);
  const activeRef = useRef(active);
  activeRef.current = active;
  const exitCallback = useRef(onExit);
  exitCallback.current = onExit;
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  const [confirmStop, setConfirmStop] = useState(false);
  const [stopping, setStopping] = useState(false);
  const [lost, setLost] = useState(false);

  useEffect(() => {
    if (!container.current) return;
    let disposed = false;
    let timer: ReturnType<typeof setTimeout>;
    let resizeTimer: ReturnType<typeof setTimeout>;
    let cursor = 0;
    let exited = false;
    let writes = Promise.resolve();
    const term = new Terminal({
      cursorBlink: true,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      fontSize: 13,
      scrollback: 5000,
      cols: 100,
      rows: 30,
      allowProposedApi: false,
      theme: {
        background: "#151c19",
        foreground: "#e5ebe3",
        cursor: "#b6d2a4",
        selectionBackground: "#426047",
      },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    // Do not let terminal output set the clipboard or invoke external links.
    const clipboard = term.parser.registerOscHandler(52, () => true);
    term.open(container.current);
    const report = (e: unknown) => {
      if (!disposed) setError(errorText(e));
    };
    const fitTerminal = () => {
      if (disposed || !activeRef.current || !container.current?.clientWidth)
        return;
      fit.fit();
      if (!exited)
        void api.ResizeTerminal(session.id, term.cols, term.rows).catch(report);
    };
    controls.current = { term, fit: fitTerminal };
    const observer = new ResizeObserver(() => {
      clearTimeout(resizeTimer);
      resizeTimer = setTimeout(fitTerminal, 80);
    });
    observer.observe(container.current);
    fitTerminal();
    if (activeRef.current) term.focus();
    const input = term.onData((data) => {
      // Serialize bridge calls to preserve keystroke and paste order. Keep each
      // request below the backend limit without splitting UTF-16 surrogate pairs.
      while (data.length) {
        let end = Math.min(data.length, 16000);
        if (end < data.length && /[\uD800-\uDBFF]/.test(data[end - 1])) end--;
        const part = data.slice(0, end);
        data = data.slice(end);
        writes = writes
          .then(async () => {
            if (!disposed && !exited) await api.WriteTerminal(session.id, part);
          })
          .catch(report);
      }
    });
    // Preserve normal terminal Ctrl+C. Copy/paste remain the native Cmd+C/V.
    term.attachCustomKeyEventHandler(
      (event) =>
        !(event.metaKey && ["k", "n"].includes(event.key.toLowerCase())),
    );
    async function poll() {
      try {
        const chunk = await api.ReadTerminal(session.id, cursor);
        if (disposed) return;
        if (chunk.lost) {
          setLost(true);
          term.reset();
        }
        if (chunk.data) {
          const bytes = Uint8Array.from(atob(chunk.data), (c) =>
            c.charCodeAt(0),
          );
          // Wait for xterm to consume output before reading another chunk.
          await new Promise<void>((resolve) => term.write(bytes, resolve));
        }
        if (disposed) return;
        cursor = chunk.cursor;
        if (chunk.exited) {
          exited = true;
          term.options.disableStdin = true;
          exitCallback.current(session.id, chunk.exitCode);
          return;
        }
        timer = setTimeout(() => void poll(), chunk.data ? 0 : 50);
      } catch (e) {
        report(e);
      }
    }
    void poll();
    return () => {
      disposed = true;
      clearTimeout(timer);
      clearTimeout(resizeTimer);
      observer.disconnect();
      input.dispose();
      clipboard.dispose();
      controls.current = null;
      term.dispose();
    };
  }, [session.id, retry]);

  useEffect(() => {
    if (active) {
      controls.current?.fit();
      controls.current?.term.focus();
    }
  }, [active]);

  async function stop() {
    setStopping(true);
    setError("");
    try {
      await api.StopTerminal(session.id);
      setConfirmStop(false);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setStopping(false);
    }
  }
  async function close() {
    try {
      await api.CloseTerminal(session.id);
      onClose(session.id);
    } catch (e) {
      setError(errorText(e));
    }
  }
  return (
    <>
      <header className="live-header">
        <div>
          <strong>{toolLabel(session.tool)}</strong>
          <span title={session.project}>{session.project}</span>
        </div>
        <div className="live-controls">
          <span className={`live-state ${session.exited ? "ended" : ""}`}>
            {session.exited ? `Exited (${session.exitCode})` : "Running"}
          </span>
          {session.exited ? (
            <button className="button subtle" onClick={() => void close()}>
              Close tab
            </button>
          ) : confirmStop ? (
            <>
              <span>Stop this agent and its jobs?</span>
              <button
                className="button subtle"
                disabled={stopping}
                onClick={() => setConfirmStop(false)}
              >
                Cancel
              </button>
              <button
                className="button danger"
                disabled={stopping}
                onClick={() => void stop()}
              >
                {stopping ? "Stopping…" : "Stop now"}
              </button>
            </>
          ) : (
            <button
              className="button subtle"
              onClick={() => setConfirmStop(true)}
            >
              Stop session
            </button>
          )}
        </div>
      </header>
      {error && (
        <div className="notice error" role="alert">
          <span>{error}</span>
          <button
            className="text-button"
            onClick={() => {
              setError("");
              setRetry((n) => n + 1);
            }}
          >
            Reconnect
          </button>
        </div>
      )}
      {lost && (
        <p className="terminal-warning" role="status">
          Earlier output exceeded the live buffer. Showing the latest output;
          saved agent conversations are unchanged.
        </p>
      )}
      <div className="terminal-host" ref={container} />
      <footer className="live-footer">
        {session.exited
          ? "Saved conversations can be continued from History."
          : "Type directly here · Ctrl+C interrupts · ⌘C / ⌘V copies and pastes"}
      </footer>
    </>
  );
}
