import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useRef } from "react";
import { expect, test, vi } from "vitest";
import FindBar, { findRanges } from "./FindBar";

function Page({ onClose = vi.fn() }: { onClose?: () => void }) {
  const container = useRef<HTMLDivElement>(null);
  return (
    <>
      <FindBar container={container} version={1} onClose={onClose} />
      <div ref={container}>
        <p>Retry the request. A retry uses the same key.</p>
        <details>
          <summary>npm test retry</summary>
          <pre>hidden retry output</pre>
        </details>
        <details open>
          <summary>Read</summary>
          <pre>open RETRY output</pre>
        </details>
      </div>
    </>
  );
}

test("finds visible matches without case, skipping collapsed tool output", () => {
  const { container } = render(<Page />);
  const ranges = findRanges(
    container.querySelector("div:not(.find-bar)")!,
    "retry",
  );
  // Two in the paragraph, one in the collapsed summary, one in the open block.
  expect(ranges.map((r) => r.toString())).toEqual([
    "Retry",
    "retry",
    "retry",
    "RETRY",
  ]);
  expect(findRanges(container as HTMLElement, "")).toEqual([]);
});

test("counts matches, steps through them and closes with Escape", async () => {
  const scroll = vi.fn();
  Element.prototype.scrollIntoView = scroll;
  const close = vi.fn();
  render(<Page onClose={close} />);
  const input = screen.getByRole("textbox", { name: "Find in conversation" });
  expect(input).toHaveFocus();
  await userEvent.type(input, "retry");
  expect(screen.getByRole("status")).toHaveTextContent("1 of 4");
  await userEvent.keyboard("{Enter}{Enter}");
  expect(screen.getByRole("status")).toHaveTextContent("3 of 4");
  await userEvent.keyboard("{Shift>}{Enter}{/Shift}");
  expect(screen.getByRole("status")).toHaveTextContent("2 of 4");
  await userEvent.click(screen.getByRole("button", { name: "Previous match" }));
  await userEvent.click(screen.getByRole("button", { name: "Previous match" }));
  // Stepping back from the first match wraps to the last.
  expect(screen.getByRole("status")).toHaveTextContent("4 of 4");
  // The view scrolls on the next frame, once folded parts are open.
  await waitFor(() => expect(scroll).toHaveBeenCalled());
  await userEvent.clear(input);
  await userEvent.type(input, "nothing here");
  expect(screen.getByRole("status")).toHaveTextContent("No matches");
  expect(screen.getByRole("button", { name: "Next match" })).toBeDisabled();
  await userEvent.keyboard("{Escape}");
  expect(close).toHaveBeenCalled();
});
