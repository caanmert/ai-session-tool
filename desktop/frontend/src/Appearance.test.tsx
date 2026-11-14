import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Appearance from "./Appearance";
import type { Backend } from "./api";

let dark: boolean, listeners: Set<() => void>;
const native = vi.fn();
beforeEach(() => {
  vi.restoreAllMocks();
  window.localStorage.clear();
  dark = false;
  listeners = new Set();
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    value: vi.fn(() => ({
      get matches() {
        return dark;
      },
      addEventListener: (_event: string, listener: () => void) =>
        listeners.add(listener),
      removeEventListener: (_event: string, listener: () => void) =>
        listeners.delete(listener),
    })),
  });
  native.mockReset().mockResolvedValue(undefined);
  window.go = {
    main: { App: { SetAppearance: native } as unknown as Backend },
  };
});
describe("appearance", () => {
  it("follows system changes and keeps explicit choices until changed", async () => {
    render(<Appearance />);
    expect(screen.getByLabelText("Appearance")).toHaveValue("system");
    act(() => {
      dark = true;
      listeners.forEach((listener) => listener());
    });
    expect(document.documentElement.dataset.theme).toBe("dark");
    fireEvent.change(screen.getByLabelText("Appearance"), {
      target: { value: "light" },
    });
    expect(document.documentElement.dataset.theme).toBe("light");
    act(() => {
      dark = false;
      listeners.forEach((listener) => listener());
      dark = true;
      listeners.forEach((listener) => listener());
    });
    expect(document.documentElement.dataset.theme).toBe("light");
    expect(window.localStorage.getItem("ais:appearance:v1")).toBe("light");
    await waitFor(() => expect(native).toHaveBeenLastCalledWith("light"));
  });
  it("restores a saved dark preference after remount and cleans up listeners", () => {
    const first = render(<Appearance />);
    fireEvent.change(screen.getByLabelText("Appearance"), {
      target: { value: "dark" },
    });
    first.unmount();
    expect(listeners.size).toBe(0);
    render(<Appearance />);
    expect(screen.getByLabelText("Appearance")).toHaveValue("dark");
    expect(document.documentElement.style.colorScheme).toBe("dark");
  });
  it("still changes appearance if storage is unavailable", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw Error("Unavailable");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw Error("Unavailable");
    });
    render(<Appearance />);
    fireEvent.change(screen.getByLabelText("Appearance"), {
      target: { value: "dark" },
    });
    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(screen.getByRole("status")).toHaveTextContent("could not be saved");
  });
  it("reports native window errors without losing the page theme", async () => {
    native.mockRejectedValue(new Error("Native connection unavailable"));
    render(<Appearance />);
    expect(await screen.findByRole("status")).toHaveTextContent(
      "page theme is active",
    );
  });
});
