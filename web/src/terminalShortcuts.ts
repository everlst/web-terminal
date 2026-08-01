const macPlatformPattern = /Mac|iPhone|iPad|iPod/;

export function isMacPlatform(platform = navigator.platform): boolean {
  return macPlatformPattern.test(platform);
}

export function shouldProcessTerminalKeyEvent(event: KeyboardEvent, mac = isMacPlatform()): boolean {
  if (event.type !== "keydown" || !mac) return true;

  const isPlainCommandShortcut = event.metaKey && !event.ctrlKey && !event.altKey && !event.shiftKey;
  if (!isPlainCommandShortcut) return true;

  const key = event.key.toLowerCase();
  // Let the browser dispatch xterm's native copy/paste events for Command+C/V.
  return key !== "c" && key !== "v";
}
