import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import type { Backend, ChatState, CommitPlan, PullRequestPlan } from "../api";
import DetailsPanel from "./DetailsPanel";
import { CommitDialog, PullRequestDialog } from "./GitDialogs";

let backend: Record<string, ReturnType<typeof vi.fn>>;
const commitPlan: CommitPlan = {
  branch: "main",
  defaultBranch: "main",
  onDefault: true,
  files: [
    { path: "src/a.ts", status: "M", added: 3, removed: 1 },
    { path: "src/new.ts", status: "?", added: 10, removed: 0 },
    { path: "old.ts", status: "D", added: 0, removed: 4 },
  ],
  added: 13,
  removed: 5,
  message: "Add retries to checkout",
  newBranch: "ais/add-retries-to-checkout",
};
beforeEach(() => {
  backend = {
    CommitPlan: vi.fn().mockResolvedValue(commitPlan),
    Commit: vi.fn().mockResolvedValue({
      commit: "abc1234",
      branch: "ais/add-retries-to-checkout",
      subject: "Add retries to checkout",
      files: 2,
    }),
    PullRequestPlan: vi.fn(),
    CreatePullRequest: vi.fn(),
    OpenURL: vi.fn().mockResolvedValue(undefined),
  };
  window.go = { main: { App: backend as unknown as Backend } };
});

test("commits the chosen files on a new branch when on the default branch", async () => {
  const committed = vi.fn();
  render(
    <CommitDialog chatId="chat" onClose={vi.fn()} onCommitted={committed} />,
  );
  const dialog = await screen.findByRole("dialog", { name: "Commit changes" });
  const message = await within(dialog).findByLabelText("Commit message");
  expect(message).toHaveValue("Add retries to checkout");
  // On main, a new branch is suggested and chosen by default.
  expect(
    within(dialog).getByRole("checkbox", { name: /Commit on a new branch/ }),
  ).toBeChecked();
  expect(within(dialog).getByLabelText("New branch name")).toHaveValue(
    "ais/add-retries-to-checkout",
  );
  expect(within(dialog).getByText("3 of 3 files")).toBeVisible();
  expect(
    within(dialog).getByText(
      "Creates a local commit only. Nothing is pushed or merged.",
    ),
  ).toBeVisible();
  await userEvent.click(
    within(dialog).getByRole("checkbox", { name: /old\.ts/ }),
  );
  expect(within(dialog).getByText("2 of 3 files")).toBeVisible();
  // Enter adds a line to the message; it does not commit.
  await userEvent.type(message, "{Enter}Keeps the key.");
  expect(backend.Commit).not.toHaveBeenCalled();
  fireEvent.keyDown(message, { key: "Enter", metaKey: true });
  await vi.waitFor(() =>
    expect(backend.Commit).toHaveBeenCalledWith("chat", {
      message: "Add retries to checkout\nKeeps the key.",
      files: ["src/a.ts", "src/new.ts"],
      newBranch: "ais/add-retries-to-checkout",
    }),
  );
  await vi.waitFor(() => expect(committed).toHaveBeenCalled());
});

test("a refused commit is explained and the dialog stays open", async () => {
  backend.Commit.mockRejectedValue(
    new Error("the commit was not made: lint failed: a.ts"),
  );
  backend.CommitPlan.mockResolvedValue({
    ...commitPlan,
    onDefault: false,
    branch: "feature",
  });
  const close = vi.fn();
  render(<CommitDialog chatId="chat" onClose={close} onCommitted={vi.fn()} />);
  const dialog = await screen.findByRole("dialog", { name: "Commit changes" });
  await within(dialog).findByText("feature");
  expect(
    within(dialog).queryByRole("checkbox", { name: /Commit on a new branch/ }),
  ).not.toBeInTheDocument();
  await userEvent.click(
    within(dialog).getByRole("button", { name: /Commit 3 files/ }),
  );
  expect(await within(dialog).findByRole("alert")).toHaveTextContent(
    "lint failed: a.ts",
  );
  expect(backend.Commit.mock.calls[0][1].newBranch).toBe("");
  // Nothing chosen: nothing to commit.
  await userEvent.click(
    within(dialog).getByRole("checkbox", { name: "All files" }),
  );
  expect(
    within(dialog).getByRole("button", { name: /Commit 0 files/ }),
  ).toBeDisabled();
  await userEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
  expect(close).toHaveBeenCalled();
});

const prPlan: PullRequestPlan = {
  repository: "me/app",
  remote: "origin",
  branch: "ais/retry",
  base: "main",
  commits: ["Retry checkout"],
  unpushed: 1,
  uncommitted: 2,
  title: "Retry checkout",
  body: "Keeps the request key.",
};

test("says exactly what opening a pull request does, then opens it", async () => {
  backend.PullRequestPlan.mockResolvedValue(prPlan);
  backend.CreatePullRequest.mockResolvedValue({
    number: 7,
    url: "https://github.com/me/app/pull/7",
    created: true,
    pushed: 1,
  });
  render(<PullRequestDialog chatId="chat" onClose={vi.fn()} />);
  const dialog = await screen.findByRole("dialog", {
    name: "Open a pull request",
  });
  expect(
    await within(dialog).findByText(
      /Pushes ais\/retry to origin and opens a pull request into main on me\/app\. Nothing is merged\./,
    ),
  ).toBeVisible();
  expect(within(dialog).getByLabelText("Title")).toHaveValue("Retry checkout");
  expect(within(dialog).getByLabelText("Description")).toHaveValue(
    "Keeps the request key.",
  );
  expect(
    within(dialog).getByText(/2 uncommitted files are not included/),
  ).toBeVisible();
  await userEvent.click(
    within(dialog).getByRole("checkbox", { name: "Open as a draft" }),
  );
  await userEvent.click(
    within(dialog).getByRole("button", { name: "Push and open pull request" }),
  );
  expect(backend.CreatePullRequest).toHaveBeenCalledWith("chat", {
    title: "Retry checkout",
    body: "Keeps the request key.",
    draft: true,
  });
  await screen.findByRole("dialog", { name: "Pull request #7 opened" });
  await userEvent.click(screen.getByRole("button", { name: "Open on GitHub" }));
  expect(backend.OpenURL).toHaveBeenCalledWith(
    "https://github.com/me/app/pull/7",
  );
});

test("pushes to a pull request that is already open instead of opening another", async () => {
  backend.PullRequestPlan.mockResolvedValue({
    ...prPlan,
    unpushed: 2,
    uncommitted: 0,
    existing: {
      number: 7,
      url: "https://github.com/me/app/pull/7",
      title: "Retry checkout",
    },
  });
  backend.CreatePullRequest.mockResolvedValue({
    number: 7,
    url: "https://github.com/me/app/pull/7",
    created: false,
    pushed: 2,
  });
  render(<PullRequestDialog chatId="chat" onClose={vi.fn()} />);
  const dialog = await screen.findByRole("dialog", { name: "Pull request #7" });
  expect(within(dialog).queryByLabelText("Title")).not.toBeInTheDocument();
  await userEvent.click(
    within(dialog).getByRole("button", { name: "Push 2 commits" }),
  );
  expect(
    await screen.findByRole("dialog", { name: "Pushed to pull request #7" }),
  ).toBeVisible();
});

test("explains why no pull request can be opened, without an action", async () => {
  backend.PullRequestPlan.mockResolvedValue({
    commits: [],
    unpushed: 0,
    uncommitted: 0,
    reason:
      "You are on main, the branch pull requests go into. Commit on a new branch first; the commit dialog can create one.",
  });
  render(<PullRequestDialog chatId="chat" onClose={vi.fn()} />);
  const dialog = await screen.findByRole("dialog");
  expect(await within(dialog).findByText(/You are on main/)).toBeVisible();
  expect(
    within(dialog)
      .getAllByRole("button")
      .map((b) => b.textContent),
  ).toEqual(["Close"]);
});

test("the details panel offers commit only when there is something to commit and the agent is idle", async () => {
  const chat = {
    id: "chat",
    tool: "codex",
    project: "/p",
    status: "idle",
    messages: [],
    title: "T",
  } as unknown as ChatState;
  const summary = {
    available: true,
    files: [{ path: "a.ts", status: "M", added: 1, removed: 0 }],
    added: 1,
    removed: 0,
  };
  const panel = (state: ChatState, changes = summary) => (
    <DetailsPanel
      chat={state}
      summary={changes}
      error=""
      drawer={false}
      onRefresh={vi.fn()}
      onClose={vi.fn()}
    />
  );
  const view = render(panel(chat));
  expect(screen.getByRole("button", { name: "Commit…" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "Pull request…" })).toBeEnabled();
  view.rerender(panel({ ...chat, status: "running" }));
  expect(screen.getByRole("button", { name: "Commit…" })).toBeDisabled();
  view.rerender(panel(chat, { ...summary, files: [] }));
  expect(screen.getByRole("button", { name: "Commit…" })).toBeDisabled();
  // Outside a Git repository neither is offered.
  view.rerender(panel(chat, { ...summary, available: false, files: [] }));
  expect(
    screen.queryByRole("button", { name: "Commit…" }),
  ).not.toBeInTheDocument();
});
