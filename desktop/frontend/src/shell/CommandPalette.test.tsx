import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import CommandPalette, { type Command } from "./CommandPalette";

const local: Command[] = [
  {
    id: "settings",
    group: "Actions",
    label: "Settings",
    icon: "settings",
    run: vi.fn(),
  },
];

test("finds text inside conversations after a pause, with the match marked", async () => {
  const open = vi.fn();
  const search = vi.fn(async (q: string) =>
    q === "retry key"
      ? [
          {
            id: "found-chat-1",
            group: "Inside conversations",
            label: "Make checkout retries safe",
            hint: "Codex",
            snippet: "…the \u0002retry\u0003 \u0002key\u0003 is reused…",
            icon: "message" as const,
            run: open,
          },
        ]
      : [],
  );
  render(<CommandPalette commands={local} search={search} onClose={vi.fn()} />);
  const box = screen.getByRole("combobox");
  await userEvent.type(box, "r");
  // One letter is not searched.
  expect(search).not.toHaveBeenCalled();
  await userEvent.type(box, "etry key");
  const option = await screen.findByRole("option", {
    name: /Make checkout retries safe/,
  });
  expect(search).toHaveBeenLastCalledWith("retry key");
  expect(within(option).getByText("Inside conversations")).toBeVisible();
  const marks = option.querySelectorAll("mark");
  expect([...marks].map((m) => m.textContent)).toEqual(["retry", "key"]);
  await userEvent.keyboard("{Enter}");
  expect(open).toHaveBeenCalled();
});

test("says nothing matches only after the search answers", async () => {
  let answer: (value: Command[]) => void = () => {};
  const search = vi.fn(
    () => new Promise<Command[]>((resolve) => (answer = resolve)),
  );
  render(<CommandPalette commands={local} search={search} onClose={vi.fn()} />);
  await userEvent.type(screen.getByRole("combobox"), "zzz");
  expect(
    await screen.findByText("Searching inside conversations…"),
  ).toBeVisible();
  expect(screen.queryByText(/Nothing matches/)).toBeNull();
  await waitFor(() => expect(search).toHaveBeenCalledWith("zzz"));
  answer([]);
  expect(await screen.findByText(/Nothing matches “zzz”/)).toBeVisible();
});
