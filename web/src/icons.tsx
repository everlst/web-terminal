import type { SVGProps } from "react";

type IconProps = SVGProps<SVGSVGElement> & { size?: number };

const base = (size = 20) => ({
  width: size,
  height: size,
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.8,
  strokeLinecap: "round" as const,
  strokeLinejoin: "round" as const,
  "aria-hidden": true
});

export function TerminalIcon({ size, ...props }: IconProps) {
  return <svg {...base(size)} {...props}><path d="m5 7 4.5 5L5 17"/><path d="M12.5 17H19"/></svg>;
}

export function ContainerIcon({ size, ...props }: IconProps) {
  return <svg {...base(size)} {...props}><path d="m12 2.8 8 4.4v9.6l-8 4.4-8-4.4V7.2Z"/><path d="m4.4 7.4 7.6 4.2 7.6-4.2M12 11.6v9.2"/></svg>;
}

export function CloseIcon({ size, ...props }: IconProps) {
  return <svg {...base(size)} {...props}><path d="M6 6l12 12M18 6 6 18"/></svg>;
}

export function PlusIcon({ size, ...props }: IconProps) {
  return <svg {...base(size)} {...props}><path d="M12 5v14M5 12h14"/></svg>;
}

export function ChevronIcon({ size, ...props }: IconProps) {
  return <svg {...base(size)} {...props}><path d="m9 18 6-6-6-6"/></svg>;
}

export function EyeIcon({ size, ...props }: IconProps) {
  return <svg {...base(size)} {...props}><path d="M2.5 12s3.5-6 9.5-6 9.5 6 9.5 6-3.5 6-9.5 6-9.5-6-9.5-6Z"/><circle cx="12" cy="12" r="2.5"/></svg>;
}

export function EyeOffIcon({ size, ...props }: IconProps) {
  return <svg {...base(size)} {...props}><path d="m3 3 18 18"/><path d="M10.6 6.2c.5-.1.9-.2 1.4-.2 6 0 9.5 6 9.5 6a17 17 0 0 1-2.2 2.8M6.2 6.2C3.8 8 2.5 12 2.5 12s3.5 6 9.5 6c1.4 0 2.6-.3 3.7-.7"/><path d="M9.8 9.8a3 3 0 0 0 4.4 4.4"/></svg>;
}

export function UserIcon({ size, ...props }: IconProps) {
  return <svg {...base(size)} {...props}><circle cx="12" cy="8" r="3.5"/><path d="M4.8 20c.8-4 3.2-6 7.2-6s6.4 2 7.2 6"/></svg>;
}

export function ShieldIcon({ size, ...props }: IconProps) {
  return <svg {...base(size)} {...props}><path d="M12 2.7 19 5v6c0 4.6-2.7 8-7 10.3C7.7 19 5 15.6 5 11V5Z"/><path d="m9 12 2 2 4-4"/></svg>;
}

export function LogoutIcon({ size, ...props }: IconProps) {
  return <svg {...base(size)} {...props}><path d="M10 5H5v14h5M14 8l4 4-4 4M18 12H9"/></svg>;
}
