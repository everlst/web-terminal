import { describe, expect, it } from "vitest";
import { isMacPlatform, shouldProcessTerminalKeyEvent } from "./terminalShortcuts";

function keyboardEvent(key: string, init: KeyboardEventInit = {}): KeyboardEvent {
  return new KeyboardEvent("keydown", { key, ...init });
}

describe("isMacPlatform", () => {
  it("recognizes macOS and iPad platforms", () => {
    expect(isMacPlatform("MacIntel")).toBe(true);
    expect(isMacPlatform("iPad")).toBe(true);
  });

  it("does not treat Windows as macOS", () => {
    expect(isMacPlatform("Win32")).toBe(false);
  });
});

describe("shouldProcessTerminalKeyEvent", () => {
  it.each(["c", "C", "v", "V"])("leaves macOS Command+%s to the browser clipboard", (key) => {
    expect(shouldProcessTerminalKeyEvent(keyboardEvent(key, { metaKey: true }), true)).toBe(false);
  });

  it("keeps Control+C available to the terminal on macOS", () => {
    expect(shouldProcessTerminalKeyEvent(keyboardEvent("c", { ctrlKey: true }), true)).toBe(true);
  });

  it("keeps Control+C and Control+V available to the terminal on Windows", () => {
    expect(shouldProcessTerminalKeyEvent(keyboardEvent("c", { ctrlKey: true }), false)).toBe(true);
    expect(shouldProcessTerminalKeyEvent(keyboardEvent("v", { ctrlKey: true }), false)).toBe(true);
  });

  it("does not override modified Command shortcuts", () => {
    expect(shouldProcessTerminalKeyEvent(keyboardEvent("c", { metaKey: true, shiftKey: true }), true)).toBe(true);
  });

  it("does not intercept keyup events", () => {
    const event = new KeyboardEvent("keyup", { key: "v", metaKey: true });
    expect(shouldProcessTerminalKeyEvent(event, true)).toBe(true);
  });
});
