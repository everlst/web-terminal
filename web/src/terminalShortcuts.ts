export type TerminalPlatform = "mac" | "windows" | "linux" | "other";

export type TerminalShortcutAction =
  | "browser-copy"
  | "browser-paste"
  | "clear"
  | "font-increase"
  | "font-decrease"
  | "font-reset";

interface NavigatorWithUserAgentData extends Navigator {
  userAgentData?: { platform?: string };
}

function browserPlatform(): string {
  const browserNavigator = navigator as NavigatorWithUserAgentData;
  return browserNavigator.userAgentData?.platform || browserNavigator.platform;
}

export function detectTerminalPlatform(platform = browserPlatform()): TerminalPlatform {
  if (/Mac|iPhone|iPad|iPod/i.test(platform)) return "mac";
  if (/Win/i.test(platform)) return "windows";
  if (/Linux|Android|CrOS/i.test(platform)) return "linux";
  return "other";
}

export function terminalShortcutAction(
  event: KeyboardEvent,
  platform = detectTerminalPlatform(),
  hasSelection = false
): TerminalShortcutAction | null {
  if (event.type !== "keydown" || event.isComposing) return null;

  const key = event.key.toLowerCase();
  if (platform === "mac") {
    if (!event.metaKey || event.ctrlKey || event.altKey) return null;
    if (key === "c" && !event.shiftKey) return "browser-copy";
    if (key === "v" && !event.shiftKey) return "browser-paste";
    if (key === "k" && !event.shiftKey) return "clear";
    if ((key === "+" || key === "=") && (!event.shiftKey || key === "+")) return "font-increase";
    if ((key === "-" || key === "_") && (!event.shiftKey || key === "_")) return "font-decrease";
    if (key === "0" && !event.shiftKey) return "font-reset";
    return null;
  }

  if (platform === "windows") {
    if (!event.ctrlKey || event.metaKey || event.altKey) return null;
    if (key === "c" && !event.shiftKey) return hasSelection ? "browser-copy" : null;
    if (key === "v" && !event.shiftKey) return "browser-paste";
    if ((key === "+" || key === "=") && (!event.shiftKey || key === "+")) return "font-increase";
    if ((key === "-" || key === "_") && (!event.shiftKey || key === "_")) return "font-decrease";
    if (key === "0" && !event.shiftKey) return "font-reset";
  }

  return null;
}
