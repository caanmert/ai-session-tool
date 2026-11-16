import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import DiffViewer from "./DiffViewer";

const snapshot = {
  files: [
    {
      path: "src/a.ts",
      patch:
        "@@ -10,3 +10,4 @@\n keep()\n-retry(all)\n+const key = id()\n+retry(transient)\n done()",
    },
  ],
};

it("comments on a range of lines by clicking and shift-clicking numbers", async () => {
  const onComment = vi.fn();
  render(
    <DiffViewer
      snapshot={snapshot}
      selectedFile="src/a.ts"
      selectedLine={0}
      onFile={vi.fn()}
      onComment={onComment}
    />,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Comment on line 10" }),
  );
  const shift = userEvent.setup();
  await shift.keyboard("{Shift>}");
  await shift.click(screen.getByRole("button", { name: "Comment on line 12" }));
  await shift.keyboard("{/Shift}");
  const box = screen.getByRole("textbox", {
    name: "Comment on src/a.ts:10-12",
  });
  expect(screen.getByRole("button", { name: "Add comment" })).toBeDisabled();
  await userEvent.type(box, "Only retry timeouts.");
  await userEvent.click(screen.getByRole("button", { name: "Add comment" }));
  expect(onComment).toHaveBeenCalledWith({
    path: "src/a.ts",
    side: "new",
    start: 10,
    end: 12,
    quote: ["keep()", "const key = id()", "retry(transient)"],
    note: "Only retry timeouts.",
  });
  // A removed line is commented on by its old number; Escape cancels.
  await userEvent.click(
    screen.getByRole("button", { name: "Comment on removed line 11" }),
  );
  await userEvent.keyboard("{Escape}");
  expect(screen.queryByRole("textbox")).toBeNull();
});

it("shows comments under their lines, and only when they still match", () => {
  const comment = {
    id: "c1",
    path: "src/a.ts",
    side: "new" as const,
    start: 11,
    end: 12,
    quote: ["const key = id()", "retry(transient)"],
    note: "Only retry timeouts.",
  };
  const view = render(
    <DiffViewer
      snapshot={snapshot}
      selectedFile="src/a.ts"
      selectedLine={0}
      onFile={vi.fn()}
      comments={[comment]}
    />,
  );
  expect(screen.getByText("Only retry timeouts.")).toBeVisible();
  // Without onComment, numbers are plain text.
  expect(screen.queryByRole("button", { name: /Comment on line/ })).toBeNull();
  view.rerender(
    <DiffViewer
      snapshot={snapshot}
      selectedFile="src/a.ts"
      selectedLine={0}
      onFile={vi.fn()}
      comments={[{ ...comment, quote: ["something else", "retry(transient)"] }]}
    />,
  );
  expect(screen.queryByText("Only retry timeouts.")).toBeNull();
});
