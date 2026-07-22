import type { Target } from "../types";
import { ChevronIcon, CloseIcon, ContainerIcon, TerminalIcon } from "../icons";

interface NewSessionMenuProps {
  targets: Target[];
  loading: boolean;
  onSelect: (target: Target) => void;
  onClose: () => void;
}

export function NewSessionMenu({ targets, loading, onSelect, onClose }: NewSessionMenuProps) {
  const host = targets.find((target) => target.kind === "host");
  const containers = targets.filter((target) => target.kind === "container");
  return (
    <>
      <button className="menu-backdrop" aria-label="关闭新建终端菜单" onClick={onClose} />
      <section className="session-menu" aria-label="新建终端">
        <div className="sheet-handle" />
        <button className="sheet-close icon-button" aria-label="关闭" onClick={onClose}><CloseIcon size={23}/></button>
        <button className="target-row is-primary" disabled={!host || loading} onClick={() => host && onSelect(host)}>
          <TerminalIcon size={24}/><span>NAS 主机</span>
        </button>
        <div className="target-row target-heading"><ContainerIcon size={24}/><span>Docker 容器</span><ChevronIcon size={20}/></div>
        <div className="container-label">运行中的容器</div>
        {loading && <div className="menu-message">正在读取容器…</div>}
        {!loading && containers.map((target) => (
          <button className="target-row container-row" key={target.id} onClick={() => onSelect(target)}>
            <ContainerIcon size={23}/><span><b>{target.name}</b></span>
          </button>
        ))}
        {!loading && containers.length === 0 && <div className="menu-message">没有可进入的运行中容器</div>}
      </section>
    </>
  );
}
