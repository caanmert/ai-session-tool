import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { isBoolean, isStringList, useStored } from "./stored";

beforeEach(() => window.localStorage.clear());
afterEach(() => vi.restoreAllMocks());

test("remembers a choice and reads it back", () => {
  const first = renderHook(() => useStored("k", false, isBoolean));
  act(() => first.result.current[1]((on) => !on));
  expect(first.result.current[0]).toBe(true);
  expect(window.localStorage.getItem("k")).toBe("true");
  const again = renderHook(() => useStored("k", false, isBoolean));
  expect(again.result.current[0]).toBe(true);
});

test("ignores a stored value of the wrong shape", () => {
  window.localStorage.setItem("folded", JSON.stringify(["a", 2]));
  const { result } = renderHook(() =>
    useStored<string[]>("folded", [], isStringList),
  );
  expect(result.current[0]).toEqual([]);
  window.localStorage.setItem("broken", "{not json");
  expect(
    renderHook(() => useStored("broken", true, isBoolean)).result.current[0],
  ).toBe(true);
});

test("still works when storage is unavailable", () => {
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
    throw new Error("blocked");
  });
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("blocked");
  });
  const { result } = renderHook(() => useStored("k", false, isBoolean));
  expect(result.current[0]).toBe(false);
  act(() => result.current[1](true));
  expect(result.current[0]).toBe(true);
});
