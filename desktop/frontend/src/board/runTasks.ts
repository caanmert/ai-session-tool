import { useEffect, useState } from "react";
import { api, errorText, type Run, type RunTaskStatus } from "../api";

/** One task of a parallel run, as the board shows it. */
export type BoardRunTask = {
  run: Run;
  status: RunTaskStatus;
};

/** How many of the newest runs the board inspects. Each inspection reads Git
 * and tmux state, so older runs are left to the Parallel runs page. */
export const BOARD_RUN_LIMIT = 4;
const REFRESH_MS = 10000;

/** Which board column a run task belongs in. An agent event wins: it means
 * someone should look, whatever the process is doing. */
export function runTaskColumn(status: RunTaskStatus) {
  if (status.attention || status.quiet) return "needs";
  return status.state === "running" ? "working" : "ready";
}

/** The process state in words. This is separate from attention on purpose. */
export function runTaskState(status: RunTaskStatus) {
  if (status.state === "exited")
    return status.exitCode === undefined
      ? "Ended"
      : status.exitCode === 0
        ? "Finished"
        : `Ended with an error (${status.exitCode})`;
  if (status.state === "running") return "Running";
  if (status.state === "missing") return "Not running";
  return "State unknown";
}

/**
 * Tasks of the newest parallel runs, refreshed while the board is visible.
 * Reading run state never starts, stops or changes a run.
 */
export function useRunTasks(active: boolean) {
  const [tasks, setTasks] = useState<BoardRunTask[]>([]);
  const [error, setError] = useState("");
  useEffect(() => {
    if (!active) return;
    let live = true;
    let timer: ReturnType<typeof setTimeout>;
    async function load() {
      try {
        const runs = (await api.ListRuns())
          .slice()
          .sort((a, b) => b.createdAt.localeCompare(a.createdAt))
          .slice(0, BOARD_RUN_LIMIT);
        const found: BoardRunTask[] = [];
        const problems: string[] = [];
        for (const run of runs) {
          try {
            const status = await api.RunStatus(run.id);
            for (const task of status.tasks) found.push({ run, status: task });
          } catch (e) {
            // One run that cannot be inspected does not hide the others.
            problems.push(`${run.id}: ${errorText(e)}`);
          }
        }
        if (!live) return;
        setTasks(found);
        setError(problems.join(" "));
      } catch (e) {
        if (live) setError(errorText(e));
      }
      if (live) timer = setTimeout(() => void load(), REFRESH_MS);
    }
    void load();
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [active]);
  return { tasks, error };
}
