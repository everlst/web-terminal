import { TerminalIcon } from "../icons";

export function Brand() {
  return (
    <div className="brand" aria-label="Web Terminal">
      <span className="brand-mark"><TerminalIcon size={28}/><i /></span>
      <span className="brand-name">Web Terminal</span>
    </div>
  );
}
