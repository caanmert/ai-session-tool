import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";
afterEach(cleanup);

// Node 25+ defines its own localStorage and sessionStorage, which replace
// jsdom's and do not work without a storage file. Use the browser-like ones.
const dom = (globalThis as { jsdom?: { window: Window } }).jsdom;
if (dom)
  for (const name of ["localStorage", "sessionStorage"] as const)
    Object.defineProperty(globalThis, name, {
      configurable: true,
      get: () => dom.window[name],
    });

// jsdom does not implement the native dialog lifecycle. Browser fixture checks
// exercise the real top-layer focus and Escape behavior.
HTMLDialogElement.prototype.showModal = function () {
  this.setAttribute("open", "");
};
HTMLDialogElement.prototype.close = function () {
  this.removeAttribute("open");
};
