import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import type { Backend, ChatState, GitHubIssueForm } from "../api";
import DetailsPanel, { issueStart } from "../chat/DetailsPanel";
import NewIssueDialog from "./NewIssueDialog";

const form: GitHubIssueForm = {
  repository: "me/app",
  url: "https://github.com/me/app",
  labels: [
    { name: "bug", color: "d73a4a", description: "Something isn't working" },
    { name: "docs", color: "not-a-colour", description: "" },
  ],
};
let backend: Record<string, ReturnType<typeof vi.fn>>;
beforeEach(() => {
  backend = {
    GitHubIssueForm: vi.fn().mockResolvedValue(form),
    CreateGitHubIssue: vi.fn().mockResolvedValue({
      number: 13,
      url: "https://github.com/me/app/issues/13",
    }),
    OpenURL: vi.fn().mockResolvedValue(undefined),
  };
  window.go = { main: { App: backend as unknown as Backend } };
});

test("an issue is created only on confirm, with the chosen labels", async () => {
  const created = vi.fn();
  render(
    <NewIssueDialog
      project="/work/app"
      initialTitle="Retry checkout"
      initialBody="Timeouts charge twice."
      onClose={vi.fn()}
      onCreated={created}
    />,
  );
  const dialog = await screen.findByRole("dialog", { name: "New issue" });
  expect(within(dialog).getByText("me/app")).toBeVisible();
  expect(within(dialog).getByText(/Anyone who can see me\/app/)).toBeVisible();
  const title = within(dialog).getByLabelText("Issue title");
  await waitFor(() => expect(title).toHaveFocus());
  expect(title).toHaveValue("Retry checkout");
  expect(within(dialog).getByLabelText("Issue description")).toHaveValue(
    "Timeouts charge twice.",
  );
  // Enter alone in the title does not create anything.
  await userEvent.type(title, " twice{Enter}");
  expect(backend.CreateGitHubIssue).not.toHaveBeenCalled();
  await userEvent.click(within(dialog).getByRole("checkbox", { name: "bug" }));
  await userEvent.keyboard("{Meta>}{Enter}{/Meta}");
  expect(backend.CreateGitHubIssue).not.toHaveBeenCalled();
  title.focus();
  await userEvent.keyboard("{Meta>}{Enter}{/Meta}");
  expect(backend.CreateGitHubIssue).toHaveBeenCalledWith(
    "/work/app",
    "Retry checkout twice",
    "Timeouts charge twice.",
    ["bug"],
  );
  const done = await screen.findByRole("dialog", { name: "Issue created" });
  expect(within(done).getByText("Created issue #13 in me/app.")).toBeVisible();
  expect(created).toHaveBeenCalledWith({
    number: 13,
    url: "https://github.com/me/app/issues/13",
  });
  const open = within(done).getByRole("button", { name: "Open on GitHub" });
  await waitFor(() => expect(open).toHaveFocus());
  await userEvent.click(open);
  expect(backend.OpenURL).toHaveBeenCalledWith(
    "https://github.com/me/app/issues/13",
  );
});

test("nothing can be created without a title, and a refusal is explained", async () => {
  backend.CreateGitHubIssue.mockRejectedValueOnce(
    new Error("GitHub did not create the issue: HTTP 403"),
  );
  render(<NewIssueDialog project="/work/app" onClose={vi.fn()} />);
  const dialog = await screen.findByRole("dialog", { name: "New issue" });
  const create = within(dialog).getByRole("button", { name: /Create issue/ });
  expect(create).toBeDisabled();
  await userEvent.type(
    within(dialog).getByLabelText("Issue title"),
    "x".repeat(257),
  );
  expect(within(dialog).getByText("257 of 256 characters")).toBeVisible();
  expect(create).toBeDisabled();
  await userEvent.clear(within(dialog).getByLabelText("Issue title"));
  await userEvent.type(within(dialog).getByLabelText("Issue title"), "Fine");
  await userEvent.click(create);
  expect(await within(dialog).findByRole("alert")).toHaveTextContent(
    "HTTP 403",
  );
  // The dialog stays, with what you wrote.
  expect(within(dialog).getByLabelText("Issue title")).toHaveValue("Fine");
});

test("a project that is not on GitHub says why", async () => {
  backend.GitHubIssueForm.mockResolvedValue({
    labels: [],
    reason:
      "This project is not a Git repository, so it has no GitHub issues or pull requests.",
  });
  render(<NewIssueDialog project="/work/notes" onClose={vi.fn()} />);
  const dialog = await screen.findByRole("dialog", { name: "New issue" });
  expect(await within(dialog).findByText(/not a Git repository/)).toBeVisible();
  expect(within(dialog).queryByLabelText("Issue title")).toBeNull();
  expect(
    within(dialog).getByRole("button", { name: /Create issue/ }),
  ).toBeDisabled();
});

test("a conversation files an issue starting from the agent's latest reply", async () => {
  const chat = {
    id: "chat",
    tool: "claude",
    project: "/work/app",
    title: "Checkout charges twice",
    status: "idle",
    messages: [
      { id: "u", role: "user", kind: "text", text: "Why twice?" },
      {
        id: "a",
        role: "assistant",
        kind: "text",
        text: "The retry has no key.",
      },
      { id: "t", role: "assistant", kind: "tool", text: "{}" },
    ],
  } as unknown as ChatState;
  expect(issueStart(chat)).toBe("The retry has no key.");
  render(
    <DetailsPanel
      chat={chat}
      summary={{ available: true, files: [], added: 0, removed: 0 }}
      error=""
      drawer={false}
      onRefresh={vi.fn()}
      onClose={vi.fn()}
    />,
  );
  await userEvent.click(screen.getByRole("button", { name: "More actions" }));
  await userEvent.click(
    screen.getByRole("menuitem", { name: /File an issue/ }),
  );
  const dialog = await screen.findByRole("dialog", { name: "New issue" });
  expect(backend.GitHubIssueForm).toHaveBeenCalledWith("/work/app");
  expect(within(dialog).getByLabelText("Issue title")).toHaveValue(
    "Checkout charges twice",
  );
  expect(within(dialog).getByLabelText("Issue description")).toHaveValue(
    "The retry has no key.",
  );
  await userEvent.click(
    within(dialog).getByRole("button", { name: /Create issue/ }),
  );
  expect(await screen.findByText("Created issue #13")).toBeVisible();
});
