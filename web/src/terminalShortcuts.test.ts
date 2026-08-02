import { describe, expect, it } from "vitest";
import { detectTerminalPlatform, terminalShortcutAction } from "./terminalShortcuts";

function keyboardEvent(key: string, init: KeyboardEventInit = {}): KeyboardEvent {
  return new KeyboardEvent("keydown", { key, ...init });
}

describe("detectTerminalPlatform", () => {
  it.each([
    ["MacIntel", "mac"],
    ["iPad", "mac"],
    ["Win32", "windows"],
    ["Linux x86_64", "linux"],
    ["Android", "linux"],
    ["Unknown", "other"]
  ] as const)("maps %s to %s", (platform, expected) => {
    expect(detectTerminalPlatform(platform)).toBe(expected);
  });
});

describe("terminalShortcutAction", () => {
  it.each([
    ["c", "browser-copy"],
    ["C", "browser-copy"],
    ["v", "browser-paste"],
    ["V", "browser-paste"]
  ] as const)("maps macOS Command+%s to %s", (key, expected) => {
    expect(terminalShortcutAction(keyboardEvent(key, { metaKey: true }), "mac")).toBe(expected);
  });

  it("keeps macOS Control+C available to the terminal", () => {
    expect(terminalShortcutAction(keyboardEvent("c", { ctrlKey: true }), "mac")).toBeNull();
  });

  it("copies with Windows Control+C only when text is selected", () => {
    const event = keyboardEvent("c", { ctrlKey: true });
    expect(terminalShortcutAction(event, "windows", true)).toBe("browser-copy");
    expect(terminalShortcutAction(event, "windows", false)).toBeNull();
  });

  it("maps Windows Control+V to browser paste", () => {
    expect(terminalShortcutAction(keyboardEvent("v", { ctrlKey: true }), "windows")).toBe("browser-paste");
  });

  it.each([
    ["k", {}, "clear"],
    ["=", {}, "font-increase"],
    ["+", { shiftKey: true }, "font-increase"],
    ["-", {}, "font-decrease"],
    ["_", { shiftKey: true }, "font-decrease"],
    ["0", {}, "font-reset"]
  ] as const)("maps macOS Command+%s to %s", (key, modifiers, expected) => {
    expect(terminalShortcutAction(keyboardEvent(key, { metaKey: true, ...modifiers }), "mac")).toBe(expected);
  });

  it.each([
    ["=", {}, "font-increase"],
    ["+", { shiftKey: true }, "font-increase"],
    ["-", {}, "font-decrease"],
    ["0", {}, "font-reset"]
  ] as const)("maps Windows Control+%s to %s", (key, modifiers, expected) => {
    expect(terminalShortcutAction(keyboardEvent(key, { ctrlKey: true, ...modifiers }), "windows")).toBe(expected);
  });

  it("does not override modified Command clipboard shortcuts", () => {
    expect(terminalShortcutAction(keyboardEvent("c", { metaKey: true, shiftKey: true }), "mac")).toBeNull();
  });

  it("does not intercept keyup or IME composition events", () => {
    const event = new KeyboardEvent("keyup", { key: "v", metaKey: true });
    expect(terminalShortcutAction(event, "mac")).toBeNull();
    expect(terminalShortcutAction(keyboardEvent("v", { metaKey: true, isComposing: true }), "mac")).toBeNull();
  });
});
