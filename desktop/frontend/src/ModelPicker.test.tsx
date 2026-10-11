import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import ModelPicker from "./ModelPicker";
import type { Backend, ModelCatalog, ModelSettings } from "./api";
const catalog: ModelCatalog = {
  defaultModel: "model-a",
  defaultEffort: "medium",
  models: [
    {
      id: "model-a",
      name: "Model A",
      description: "Test model",
      efforts: ["low", "medium", "high"],
      defaultEffort: "medium",
    },
    {
      id: "model-b",
      name: "Model B",
      description: "No effort support",
      efforts: [],
      defaultEffort: "",
    },
  ],
};
let backend: Backend;
const onPending = vi.fn(),
  onSaved = vi.fn();
beforeEach(() => {
  vi.clearAllMocks();
  backend = {
    ChatModels: vi.fn().mockResolvedValue(catalog),
    SetChatSettings: vi.fn().mockResolvedValue(undefined),
  } as unknown as Backend;
  window.go = { main: { App: backend } };
});
function show(settings?: ModelSettings, reviewer = false, disabled = false) {
  render(
    <ModelPicker
      id="chat"
      settings={settings}
      reviewer={reviewer}
      disabled={disabled}
      onPending={onPending}
      onSaved={onSaved}
    />,
  );
}
it("discovers models, saves settings, and resets effort when the model changes", async () => {
  show();
  await screen.findByRole("option", { name: "Model A" });
  expect(backend.ChatModels).toHaveBeenCalledWith("chat", false, false);
  await userEvent.selectOptions(screen.getByLabelText("Model"), "model-a");
  await userEvent.selectOptions(
    screen.getByLabelText("Reasoning effort"),
    "high",
  );
  expect(backend.SetChatSettings).toHaveBeenLastCalledWith(
    "chat",
    { model: "model-a", effort: "high", configured: true },
    false,
  );
  await userEvent.selectOptions(screen.getByLabelText("Model"), "model-b");
  expect(backend.SetChatSettings).toHaveBeenLastCalledWith(
    "chat",
    { model: "model-b", effort: "", configured: true },
    false,
  );
  expect(screen.getByLabelText("Reasoning effort")).toBeDisabled();
  expect(screen.getByLabelText("Reasoning effort")).toHaveValue("");
});
it("keeps unavailable saved models visible until the user chooses a replacement", async () => {
  show({ model: "retired-model", effort: "high", configured: true });
  await screen.findByRole("option", { name: "Model A" });
  expect(screen.getByLabelText("Model")).toHaveValue("retired-model");
  expect(
    screen.getByRole("option", { name: "retired-model · Unavailable" }),
  ).toBeDisabled();
  expect(backend.SetChatSettings).not.toHaveBeenCalled();
  await userEvent.selectOptions(screen.getByLabelText("Model"), "");
  expect(backend.SetChatSettings).toHaveBeenCalledWith(
    "chat",
    { model: "", effort: "", configured: true },
    false,
  );
});
it("uses separate reviewer settings and prevents edits while a response runs", async () => {
  show({ model: "model-a", effort: "medium" }, true);
  await screen.findByRole("option", { name: "Model A" });
  expect(backend.ChatModels).toHaveBeenCalledWith("chat", true, false);
  await userEvent.selectOptions(
    screen.getByLabelText("Reviewer reasoning effort"),
    "high",
  );
  expect(backend.SetChatSettings).toHaveBeenCalledWith(
    "chat",
    { model: "model-a", effort: "high", configured: true },
    true,
  );
});
it("recovers failed discovery with refresh and leaves default selectable", async () => {
  backend.ChatModels = vi
    .fn()
    .mockRejectedValueOnce(new Error("Sign in to the CLI"))
    .mockResolvedValue(catalog);
  show();
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Sign in to the CLI",
  );
  expect(screen.getByLabelText("Model")).not.toBeDisabled();
  await userEvent.click(screen.getByRole("button", { name: "Refresh models" }));
  await screen.findByRole("option", { name: "Model A" });
  expect(backend.ChatModels).toHaveBeenLastCalledWith("chat", false, true);
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});
it("serializes saves and restores the choice when persistence fails", async () => {
  let fail!: (error: Error) => void;
  backend.SetChatSettings = vi.fn(
    () =>
      new Promise<void>((_resolve, reject) => {
        fail = reject;
      }),
  );
  show();
  await screen.findByRole("option", { name: "Model A" });
  fireEvent.change(screen.getByLabelText("Model"), {
    target: { value: "model-a" },
  });
  expect(screen.getByLabelText("Model")).toBeDisabled();
  expect(onPending).toHaveBeenLastCalledWith(true);
  await act(async () => fail(new Error("Cannot save preferences")));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Cannot save preferences",
  );
  expect(screen.getByLabelText("Model")).toHaveValue("");
  expect(onPending).toHaveBeenLastCalledWith(false);
  expect(onSaved).not.toHaveBeenCalled();
});
it("locks both selectors during an active response", async () => {
  show({ model: "model-a", effort: "high" }, false, true);
  await waitFor(() => expect(backend.ChatModels).toHaveBeenCalled());
  expect(screen.getByLabelText("Model")).toBeDisabled();
  expect(screen.getByLabelText("Reasoning effort")).toBeDisabled();
});
