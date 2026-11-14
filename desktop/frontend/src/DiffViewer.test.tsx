import { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import DiffViewer from "./DiffViewer";

it("navigates files with keys, wraps lines and resizes the file panel", async () => {
  const files = [
    { path: "api.ts", patch: "@@ -1 +1 @@\n-before\n+after" },
    { path: "tests.ts", patch: "@@ -0,0 +1,2 @@\n+test one\n+test two" },
  ];
  function Preview() {
    const [file, setFile] = useState("");
    return (
      <DiffViewer
        snapshot={{ files }}
        selectedFile={file}
        selectedLine={1}
        onFile={setFile}
      />
    );
  }
  render(<Preview />);
  screen.getByRole("button", { name: "api.ts" }).focus();
  await userEvent.keyboard("{End}");
  expect(screen.getByRole("button", { name: "tests.ts" })).toHaveFocus();
  expect(screen.getByText("+2")).toBeVisible();
  await userEvent.click(screen.getByLabelText("Wrap lines"));
  expect(document.querySelector(".review-diff")).toHaveClass("wrap-lines");
  fireEvent.change(screen.getByLabelText("File panel width"), {
    target: { value: "260" },
  });
  expect(document.querySelector(".review-diff")).toHaveStyle({
    gridTemplateColumns: "260px minmax(0, 1fr)",
  });
  expect(document.querySelectorAll(".diff-line.focused")).toHaveLength(1);
});
