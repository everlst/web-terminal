import { describe, expect, it } from "vitest";
import { terminalSocketURL } from "./api";

describe("terminalSocketURL", () => {
  it("creates a same-origin websocket URL", () => {
    expect(terminalSocketURL("a/b")).toBe("ws://localhost:5173/api/sessions/a%2Fb/stream");
  });
});
