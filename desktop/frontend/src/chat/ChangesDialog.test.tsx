import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import type { ReviewSnapshot } from "../api";
import ChangesDialog from "./ChangesDialog";

const snapshot = (extra: Partial<ReviewSnapshot>): ReviewSnapshot => ({
  root: "/work/app",
  base: "abc",
  hash: "",
  diff: "",
  files: [],
  warnings: [],
  ...extra,
});

function open(load: () => Promise<ReviewSnapshot>) {
  render(
    <ChangesDialog
      title="Uncommitted changes"
      note="/work/app"
      load={load}
      file=""
      onFile={vi.fn()}
      onClose={vi.fn()}
      empty="No uncommitted changes."
    />,
  );
}

it("shows what fits and, first, what was left out", async () => {
  open(() =>
    Promise.resolve(
      snapshot({
        files: [{ path: "a.ts", patch: "@@ -1 +1 @@\n-a\n+b\n" }],
        warnings: ["Only the first 100 of 200 changed files are shown."],
        total: 200,
        partial: true,
      }),
    ),
  );
  const list = await screen.findByRole("list", {
    hidden: true,
    name: "Not shown",
  });
  expect(list).toHaveTextContent(
    "Only the first 100 of 200 changed files are shown.",
  );
  expect(screen.getAllByText("a.ts", { exact: false }).length).toBeGreaterThan(
    0,
  );
  // The note comes before the diff.
  const diff = document.querySelector(".diff-viewer")!;
  expect(
    list.compareDocumentPosition(diff) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
});

it("does not call changes it could not show 'no changes'", async () => {
  open(() =>
    Promise.resolve(
      snapshot({
        warnings: ["huge.json is larger than 512 KiB and is not shown."],
        total: 1,
        partial: true,
      }),
    ),
  );
  await screen.findByRole("list", { hidden: true, name: "Not shown" });
  expect(screen.queryByText("No uncommitted changes.")).toBeNull();
});

it("says when there are no changes", async () => {
  open(() => Promise.resolve(snapshot({ total: 0 })));
  expect(
    await screen.findByText("No uncommitted changes."),
  ).toBeInTheDocument();
});
