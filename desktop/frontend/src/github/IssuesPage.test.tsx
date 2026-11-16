import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import type { Backend, GitHubOverview, Project } from "../api";
import IssuesPage, { issueTask } from "./IssuesPage";

const projects: Project[] = [
  {
    path: "/work/app",
    name: "app",
    sessions: 2,
    available: true,
    recent: false,
  },
  {
    path: "/work/site",
    name: "site",
    sessions: 0,
    available: true,
    recent: false,
  },
  { path: "/gone", name: "gone", sessions: 0, available: false, recent: false },
];
const overview: GitHubOverview = {
  repository: "me/app",
  url: "https://github.com/me/app",
  issues: [
    {
      number: 12,
      title: "Retry checkout",
      url: "https://github.com/me/app/issues/12",
      author: "ana",
      labels: ["bug", "payments"],
      updatedAt: "2026-11-15T10:00:00Z",
    },
    {
      number: 15,
      title: "Document the API",
      url: "https://github.com/me/app/issues/15",
      author: "bo",
      labels: [],
      updatedAt: "2026-11-14T10:00:00Z",
    },
  ],
  pulls: [
    {
      number: 7,
      title: "Add retries",
      url: "https://github.com/me/app/pull/7",
      author: "bo",
      branch: "ais/retry",
      isDraft: true,
      updatedAt: "2026-11-15T11:00:00Z",
      review: "CHANGES_REQUESTED",
      checks: "failing",
    },
  ],
};
let backend: Record<string, ReturnType<typeof vi.fn>>;
beforeEach(() => {
  backend = {
    GitHubOverview: vi.fn().mockResolvedValue(overview),
    GitHubIssue: vi.fn().mockResolvedValue({
      number: 12,
      title: "Retry checkout",
      url: "https://github.com/me/app/issues/12",
      body: "Timeouts charge twice.\n\n## Done means\n- one charge",
    }),
    OpenURL: vi.fn().mockResolvedValue(undefined),
  };
  window.go = { main: { App: backend as unknown as Backend } };
});

test("lists open issues and starts a task with the issue's text", async () => {
  const start = vi.fn();
  render(<IssuesPage projects={projects} project="" onStartTask={start} />);
  const list = await screen.findByRole("list", { name: "Open issues" });
  expect(backend.GitHubOverview).toHaveBeenCalledWith("/work/app");
  expect(within(list).getByText("Retry checkout")).toBeVisible();
  expect(within(list).getByText("payments")).toBeVisible();
  // Projects whose folder is missing are not offered.
  expect(
    within(screen.getByLabelText("Project for issues")).queryByText("gone"),
  ).not.toBeInTheDocument();
  const row = within(list).getByText("Retry checkout").closest("li")!;
  await userEvent.click(
    within(row).getByRole("button", { name: "Start task" }),
  );
  expect(backend.GitHubIssue).toHaveBeenCalledWith("/work/app", 12);
  await waitFor(() =>
    expect(start).toHaveBeenCalledWith(
      "/work/app",
      "Work on issue #12: Retry checkout\n\nhttps://github.com/me/app/issues/12\n\nTimeouts charge twice.\n\n## Done means\n- one charge",
    ),
  );
  await userEvent.click(
    within(row).getByRole("button", { name: "Open issue #12 on GitHub" }),
  );
  expect(backend.OpenURL).toHaveBeenCalledWith(
    "https://github.com/me/app/issues/12",
  );
});

test("shows pull request review and check states in words, and filters", async () => {
  render(<IssuesPage projects={projects} project="" onStartTask={vi.fn()} />);
  await screen.findByRole("list", { name: "Open issues" });
  await userEvent.type(
    screen.getByLabelText("Filter issues and pull requests"),
    "payments",
  );
  const issues = screen.getByRole("list", { name: "Open issues" });
  expect(within(issues).getByText("Retry checkout")).toBeVisible();
  expect(
    within(issues).queryByText("Document the API"),
  ).not.toBeInTheDocument();
  await userEvent.clear(
    screen.getByLabelText("Filter issues and pull requests"),
  );
  await userEvent.click(screen.getByRole("button", { name: /Pull requests/ }));
  const pulls = screen.getByRole("list", { name: "Open pull requests" });
  expect(within(pulls).getByText("Draft")).toBeVisible();
  expect(within(pulls).getByText("Checks failing")).toBeVisible();
  expect(within(pulls).getByText("Changes requested")).toBeVisible();
  expect(within(pulls).getByText("ais/retry")).toBeVisible();
});

test("follows the project chosen in the top bar and refreshes on request", async () => {
  const view = render(
    <IssuesPage projects={projects} project="" onStartTask={vi.fn()} />,
  );
  await screen.findByRole("list", { name: "Open issues" });
  view.rerender(
    <IssuesPage
      projects={projects}
      project="/work/site"
      onStartTask={vi.fn()}
    />,
  );
  await waitFor(() =>
    expect(backend.GitHubOverview).toHaveBeenLastCalledWith("/work/site"),
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Refresh from GitHub" }),
  );
  await waitFor(() => expect(backend.GitHubOverview).toHaveBeenCalledTimes(3));
});

test("explains when nothing or only part could be read", async () => {
  backend.GitHubOverview.mockResolvedValueOnce({
    issues: [],
    pulls: [],
    reason:
      "GitHub could not be read through the GitHub CLI: the GitHub CLI (gh) is not installed.",
  });
  const view = render(
    <IssuesPage projects={projects} project="" onStartTask={vi.fn()} />,
  );
  expect(await screen.findByText(/gh\) is not installed/)).toBeVisible();
  expect(screen.queryByRole("list")).not.toBeInTheDocument();
  view.unmount();
  backend.GitHubOverview.mockResolvedValueOnce({
    ...overview,
    pulls: [],
    partial: "Pull requests: HTTP 502",
  });
  render(<IssuesPage projects={projects} project="" onStartTask={vi.fn()} />);
  expect(await screen.findByText(/Pull requests: HTTP 502/)).toBeVisible();
  expect(screen.getByText("Retry checkout")).toBeVisible();
});

test("an issue without a description still makes a task", () => {
  expect(
    issueTask({ number: 3, title: "Fix it", url: "https://x/3", body: "  " }),
  ).toBe("Work on issue #3: Fix it\n\nhttps://x/3");
});

test("New issue creates an issue in the chosen project and reloads the list", async () => {
  backend.GitHubIssueForm = vi.fn().mockResolvedValue({
    repository: "me/app",
    url: "https://github.com/me/app",
    labels: [],
  });
  backend.CreateGitHubIssue = vi.fn().mockResolvedValue({
    number: 16,
    url: "https://github.com/me/app/issues/16",
  });
  render(<IssuesPage projects={projects} project="" onStartTask={vi.fn()} />);
  await screen.findByText("Retry checkout");
  const reads = backend.GitHubOverview.mock.calls.length;
  await userEvent.click(screen.getByRole("button", { name: /New issue/ }));
  const dialog = await screen.findByRole("dialog", { name: "New issue" });
  await userEvent.type(
    within(dialog).getByLabelText("Issue title"),
    "Add a status page",
  );
  await userEvent.click(
    within(dialog).getByRole("button", { name: /Create issue/ }),
  );
  expect(backend.CreateGitHubIssue).toHaveBeenCalledWith(
    "/work/app",
    "Add a status page",
    "",
    [],
  );
  await screen.findByRole("dialog", { name: "Issue created" });
  await waitFor(() =>
    expect(backend.GitHubOverview.mock.calls.length).toBeGreaterThan(reads),
  );
});
