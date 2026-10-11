import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import LiveTerminal from "./LiveTerminal";
import { api, type LiveSession } from "./api";

const terminal = vi.hoisted(() => ({
  construct: vi.fn(),
  dispose: vi.fn(),
  write: vi.fn(),
  fit: vi.fn(),
  focus: vi.fn(),
  osc: vi.fn(),
  input: (_data: string) => {},
}));
vi.mock("@xterm/xterm", () => ({
  Terminal: class {
    cols = 110;
    rows = 35;
    options = { disableStdin: false };
    parser = {
      registerOscHandler: terminal.osc.mockReturnValue({ dispose: vi.fn() }),
    };
    constructor() {
      terminal.construct();
    }
    loadAddon() {}
    open() {}
    reset() {}
    attachCustomKeyEventHandler() {}
    focus = terminal.focus;
    dispose = terminal.dispose;
    onData(handler: (data: string) => void) {
      terminal.input = handler;
      return { dispose: vi.fn() };
    }
    write(data: Uint8Array, callback: () => void) {
      terminal.write(data);
      callback();
    }
  },
}));
vi.mock("@xterm/addon-fit", () => ({
  FitAddon: class {
    fit = terminal.fit;
  },
}));
vi.mock("./api", async (original) => {
  const actual = await original<typeof import("./api")>();
  return {
    ...actual,
    api: {
      ReadTerminal: vi.fn(),
      WriteTerminal: vi.fn(),
      ResizeTerminal: vi.fn(),
      StopTerminal: vi.fn(),
      CloseTerminal: vi.fn(),
    },
  };
});
const session: LiveSession = {
  id: "live-1",
  tool: "codex",
  project: "/demo/api",
  key: "",
  exited: false,
  exitCode: 0,
};
const idle = { data: "", cursor: 0, lost: false, exited: false, exitCode: 0 };
beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
  Object.defineProperty(HTMLElement.prototype, "clientWidth", {
    configurable: true,
    get: () => 900,
  });
  vi.mocked(api.ReadTerminal).mockResolvedValue(idle);
  vi.mocked(api.WriteTerminal).mockResolvedValue();
  vi.mocked(api.ResizeTerminal).mockResolvedValue();
  vi.mocked(api.StopTerminal).mockResolvedValue();
  vi.mocked(api.CloseTerminal).mockResolvedValue();
});

describe("live terminal", () => {
  it("renders raw UTF-8 output before reporting exit and forwards terminal geometry", async () => {
    const bytes = new TextEncoder().encode("λ🙂\x1b[32mready");
    vi.mocked(api.ReadTerminal).mockResolvedValue({
      ...idle,
      data: btoa(String.fromCharCode(...bytes)),
      cursor: bytes.length,
      exited: true,
      exitCode: 7,
    });
    const onExit = vi.fn();
    render(
      <LiveTerminal
        session={session}
        active
        onExit={onExit}
        onClose={vi.fn()}
      />,
    );
    await waitFor(() => expect(onExit).toHaveBeenCalledWith(session.id, 7));
    expect(Array.from(terminal.write.mock.calls[0][0] as Uint8Array)).toEqual(
      Array.from(bytes),
    );
    expect(terminal.write.mock.invocationCallOrder[0]).toBeLessThan(
      onExit.mock.invocationCallOrder[0],
    );
    expect(api.ResizeTerminal).toHaveBeenCalledWith(session.id, 110, 35);
    expect(terminal.osc).toHaveBeenCalledWith(52, expect.any(Function));
  });
  it("preserves the terminal while switching tabs and disposes it when closed", async () => {
    const props = { session, onExit: vi.fn(), onClose: vi.fn() };
    const view = render(<LiveTerminal {...props} active />);
    await waitFor(() => expect(api.ReadTerminal).toHaveBeenCalled());
    view.rerender(<LiveTerminal {...props} active={false} />);
    view.rerender(<LiveTerminal {...props} active />);
    expect(terminal.construct).toHaveBeenCalledTimes(1);
    expect(terminal.dispose).not.toHaveBeenCalled();
    view.unmount();
    expect(terminal.dispose).toHaveBeenCalledTimes(1);
    const calls = vi.mocked(api.ReadTerminal).mock.calls.length;
    await new Promise((resolve) => setTimeout(resolve, 90));
    expect(api.ReadTerminal).toHaveBeenCalledTimes(calls);
  });
  it("sends keystrokes in order even when bridge writes are delayed", async () => {
    let finish!: () => void;
    vi.mocked(api.WriteTerminal).mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        }),
    );
    render(
      <LiveTerminal
        session={session}
        active
        onExit={vi.fn()}
        onClose={vi.fn()}
      />,
    );
    await act(async () => {
      terminal.input("one");
      terminal.input("\r");
    });
    expect(api.WriteTerminal).toHaveBeenCalledTimes(1);
    await act(async () => finish());
    expect(vi.mocked(api.WriteTerminal).mock.calls).toEqual([
      [session.id, "one"],
      [session.id, "\r"],
    ]);
  });
  it("requires a stop confirmation and reports a failed stop", async () => {
    vi.mocked(api.StopTerminal).mockRejectedValue(new Error("stop failed"));
    render(
      <LiveTerminal
        session={session}
        active
        onExit={vi.fn()}
        onClose={vi.fn()}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Stop session" }));
    expect(api.StopTerminal).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Stop now" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("stop failed");
  });
  it("reports buffer loss and offers recovery after a disconnected bridge", async () => {
    vi.mocked(api.ReadTerminal)
      .mockResolvedValueOnce({ ...idle, lost: true })
      .mockRejectedValue(new Error("bridge disconnected"));
    render(
      <LiveTerminal
        session={session}
        active
        onExit={vi.fn()}
        onClose={vi.fn()}
      />,
    );
    expect(await screen.findByRole("status")).toHaveTextContent(
      "Earlier output exceeded",
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "bridge disconnected",
    );
    vi.mocked(api.ReadTerminal).mockResolvedValue({ ...idle, exited: true });
    await userEvent.click(screen.getByRole("button", { name: "Reconnect" }));
    await waitFor(() => expect(terminal.construct).toHaveBeenCalledTimes(2));
    expect(terminal.dispose).toHaveBeenCalledTimes(1);
  });
});
