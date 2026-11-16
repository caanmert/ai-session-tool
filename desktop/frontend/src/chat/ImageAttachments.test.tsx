import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import ChatWorkspace from "../ChatWorkspace";
import type { Backend, ChatState } from "../api";

const chat = {
  id: "chat",
  tool: "codex",
  project: "/project",
  title: "Fix layout",
  status: "idle",
  error: "",
  messages: [
    {
      id: "u1",
      role: "user",
      kind: "text",
      text: "Why is this cut off?",
      images: ["screenshot.png"],
    },
  ],
  approvals: [],
  shortened: false,
  version: 1,
} as unknown as ChatState;
let backend: Record<string, ReturnType<typeof vi.fn>>;
beforeEach(() => {
  window.localStorage.clear();
  let preview = 0;
  URL.createObjectURL = vi.fn(() => `blob:preview-${preview++}`);
  URL.revokeObjectURL = vi.fn();
  backend = {
    ReadChat: vi.fn(async (_id: string, version: number) => ({
      version: 1,
      state: version === 1 ? null : chat,
    })),
    ChatModels: vi
      .fn()
      .mockResolvedValue({ models: [], defaultModel: "", defaultEffort: "" }),
    ChatChanges: vi
      .fn()
      .mockResolvedValue({ available: true, files: [], added: 0, removed: 0 }),
    SendChat: vi.fn().mockResolvedValue(undefined),
    SendChatWithImages: vi.fn().mockResolvedValue(undefined),
  };
  window.go = { main: { App: backend as unknown as Backend } };
});
const show = () =>
  render(
    <ChatWorkspace
      chat={chat}
      active
      reviewerAvailable={false}
      onOpen={vi.fn()}
      onSelect={vi.fn()}
      onState={vi.fn()}
    />,
  );
const png = (name: string, bytes = [137, 80, 78, 71]) =>
  new File([new Uint8Array(bytes)], name, { type: "image/png" });
const paste = (files: File[]) =>
  fireEvent.paste(screen.getByLabelText("Message agent"), {
    clipboardData: { files },
  });

test("a pasted screenshot is attached and sent with the message", async () => {
  show();
  const message = screen.getByLabelText("Message agent");
  await userEvent.type(message, "What is wrong here?");
  paste([png("shot.png")]);
  const list = await screen.findByRole("list", { name: "Attached images" });
  expect(within(list).getByText("shot.png")).toBeVisible();
  // Pasting an image does not put anything into the text.
  expect(message).toHaveValue("What is wrong here?");
  await userEvent.click(screen.getByRole("button", { name: "Send" }));
  await waitFor(() =>
    expect(backend.SendChatWithImages).toHaveBeenCalledWith(
      "chat",
      "What is wrong here?",
      [{ name: "shot.png", data: "iVBORw==" }],
    ),
  );
  expect(backend.SendChat).not.toHaveBeenCalled();
  // Sent attachments are cleared with the text.
  await waitFor(() =>
    expect(
      screen.queryByRole("list", { name: "Attached images" }),
    ).not.toBeInTheDocument(),
  );
  expect(URL.revokeObjectURL).toHaveBeenCalled();
});

test("images can be removed, and a message without them is sent as text", async () => {
  show();
  await userEvent.type(screen.getByLabelText("Message agent"), "Plain");
  paste([png("one.png"), png("two.png")]);
  await screen.findByText("two.png");
  await userEvent.click(screen.getByRole("button", { name: "Remove one.png" }));
  await userEvent.click(screen.getByRole("button", { name: "Remove two.png" }));
  expect(
    screen.queryByRole("list", { name: "Attached images" }),
  ).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Send" }));
  await waitFor(() =>
    expect(backend.SendChat).toHaveBeenCalledWith("chat", "Plain"),
  );
  expect(backend.SendChatWithImages).not.toHaveBeenCalled();
});

test("rejects files that are not images, are too large, or are too many", async () => {
  show();
  paste([new File(["%PDF"], "notes.pdf", { type: "application/pdf" })]);
  // A non-image paste is left to the text box; nothing is attached.
  expect(
    screen.queryByRole("list", { name: "Attached images" }),
  ).not.toBeInTheDocument();
  const picker = screen.getByLabelText("Choose images to attach");
  await userEvent.upload(
    picker,
    new File(["<svg/>"], "icon.svg", { type: "image/svg+xml" }),
    { applyAccept: false },
  );
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "icon.svg is not a PNG, JPEG, GIF or WebP image.",
  );
  const big = png("big.png");
  Object.defineProperty(big, "size", { value: 6 * 1024 * 1024 });
  await userEvent.upload(picker, big);
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "big.png is larger than 5 MB.",
  );
  await userEvent.upload(
    picker,
    Array.from({ length: 7 }, (_, i) => png(`p${i}.png`)),
  );
  await screen.findByText("p5.png");
  expect(screen.queryByText("p6.png")).not.toBeInTheDocument();
  expect(screen.getByRole("alert")).toHaveTextContent(
    "Only 6 images fit in one message.",
  );
});

test("a sent message lists the images that went with it", async () => {
  show();
  const sent = await screen.findByRole("list", {
    name: "Images sent with this message",
  });
  expect(within(sent).getByText("screenshot.png")).toBeVisible();
});
