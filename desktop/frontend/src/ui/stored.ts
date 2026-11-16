import { useCallback, useState } from "react";

export const isBoolean = (value: unknown): value is boolean =>
  typeof value === "boolean";
export const isStringList = (value: unknown): value is string[] =>
  Array.isArray(value) && value.every((item) => typeof item === "string");

/**
 * A layout choice kept on this Mac, such as a folded sidebar. A convenience
 * only: when storage is unavailable or holds something unexpected, the
 * fallback is used and the choice lasts until the app quits.
 */
export function useStored<T>(
  key: string,
  fallback: T,
  valid: (value: unknown) => value is T,
) {
  const [value, setValue] = useState<T>(() => {
    try {
      const saved = window.localStorage.getItem(key);
      if (saved !== null) {
        const parsed: unknown = JSON.parse(saved);
        if (valid(parsed)) return parsed;
      }
    } catch {
      // Use the fallback.
    }
    return fallback;
  });
  const update = useCallback(
    (next: T | ((current: T) => T)) =>
      setValue((current) => {
        const resolved =
          typeof next === "function"
            ? (next as (current: T) => T)(current)
            : next;
        try {
          window.localStorage.setItem(key, JSON.stringify(resolved));
        } catch {
          // Kept for this session only.
        }
        return resolved;
      }),
    [key],
  );
  return [value, update] as const;
}
