import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SavedWorkspace from "./SavedWorkspace";
import type { Backend, ChatState, ChatSummary } from "./api";

const saved: ChatSummary[] = [
  {
    id: "review",
    tool: "claude",
    project: "/projects/api",
    title: "Checkout review",
    status: "idle",
    updatedAt: "2026-11-14T08:00:00Z",
    closed: true,
    isReview: true,
    complete: true,
    findingCount: 2,
    sentCount: 1,
    fileCount: 3,
  },
  {
    id: "draft",
    tool: "codex",
    project: "/projects/api",
    title: "New conversation",
    status: "idle",
    updatedAt: "2026-11-14T07:00:00Z",
    closed: true,
    isReview: false,
    complete: false,
    findingCount: 0,
    sentCount: 0,
    fileCount: 0,
  },
  {
    id: "other",
    tool: "codex",
    project: "/projects/docs",
    title: "Documentation",
    status: "idle",
    updatedAt: "2026-11-13T07:00:00Z",
    closed: false,
    isReview: false,
    complete: false,
    findingCount: 0,
    sentCount: 0,
    fileCount: 0,
  },
];
let backend: Backend;
const onOpen = vi.fn(),
  onClose = vi.fn();
beforeEach(() => {
  vi.clearAllMocks();
  backend = {
    ListSavedChats: vi.fn().mockResolvedValue(saved),
    ReopenChat: vi
      .fn()
      .mockResolvedValue({ id: "review", review: { sourceId: "source" } }),
    SendChat: vi.fn(),
  } as unknown as Backend;
  window.go = { main: { App: backend } };
});
const show = (project = "") =>
  render(
    <SavedWorkspace project={project} onOpen={onOpen} onClose={onClose} />,
  );

describe("saved workspace", () => {
  it("filters saved conversations and reviews by project, type and search", async () => {
    show("/projects/api");
    await screen.findByRole("heading", { name: "Checkout review" });
    expect(
      screen.queryByRole("heading", { name: "Documentation" }),
    ).not.toBeInTheDocument();
    expect(screen.getByText("2 findings · 1 sent · 3 files")).toBeVisible();
    await userEvent.selectOptions(
      screen.getByLabelText("Saved work type"),
      "reviews",
    );
    expect(
      screen.queryByRole("heading", { name: "New conversation" }),
    ).not.toBeInTheDocument();
    await userEvent.type(
      screen.getByLabelText("Search saved work"),
      "no match",
    );
    await screen.findByRole("heading", { name: "No matching saved work" });
    await userEvent.click(
      screen.getByRole("button", { name: "Clear saved-work filters" }),
    );
    expect(
      screen.getByRole("heading", { name: "Documentation" }),
    ).toBeVisible();
    await userEvent.selectOptions(
      screen.getByLabelText("Saved work type"),
      "conversations",
    );
    expect(
      screen.queryByRole("heading", { name: "Checkout review" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Reopen New conversation" }),
    ).toBeEnabled();
    expect(backend.SendChat).not.toHaveBeenCalled();
  });
  it("reopens the existing record once without sending a message", async () => {
    let finish!: (state: ChatState) => void;
    backend.ReopenChat = vi.fn(
      () =>
        new Promise<ChatState>((resolve) => {
          finish = resolve;
        }),
    );
    show();
    const button = await screen.findByRole("button", {
      name: "Reopen Checkout review",
    });
    fireEvent.click(button);
    fireEvent.click(button);
    const cancel = new Event("cancel", { cancelable: true });
    fireEvent(screen.getByRole("dialog"), cancel);
    expect(cancel.defaultPrevented).toBe(true);
    expect(onClose).not.toHaveBeenCalled();
    expect(backend.ReopenChat).toHaveBeenCalledTimes(1);
    await act(async () => finish({ id: "review" } as ChatState));
    expect(onOpen).toHaveBeenCalledWith({ id: "review" });
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(backend.SendChat).not.toHaveBeenCalled();
  });
  it("shows load and reopen failures and allows retry", async () => {
    backend.ListSavedChats = vi
      .fn()
      .mockRejectedValueOnce(new Error("Records unavailable"))
      .mockResolvedValue(saved);
    backend.ReopenChat = vi
      .fn()
      .mockRejectedValue(new Error("Close an older tab"));
    show();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Records unavailable",
    );
    await userEvent.click(screen.getByRole("button", { name: "Retry" }));
    await userEvent.click(
      await screen.findByRole("button", { name: "Reopen Checkout review" }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Close an older tab",
    );
    expect(onOpen).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });
  it("handles empty records and restores focus on close", async () => {
    backend.ListSavedChats = vi.fn().mockResolvedValue([]);
    const button = document.createElement("button");
    document.body.append(button);
    button.focus();
    const view = show();
    expect(screen.getByLabelText("Search saved work")).toHaveFocus();
    await screen.findByRole("heading", {
      name: "Your saved work will be here",
    });
    fireEvent(
      screen.getByRole("dialog"),
      new Event("cancel", { cancelable: true }),
    );
    expect(onClose).toHaveBeenCalled();
    view.unmount();
    await waitFor(() => expect(button).toHaveFocus());
    button.remove();
  });
});
