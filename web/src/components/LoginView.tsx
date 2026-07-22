import { FormEvent, useState } from "react";
import { APIError, api } from "../api";
import { EyeIcon, EyeOffIcon, ShieldIcon } from "../icons";
import { Brand } from "./Brand";

interface LoginViewProps {
  onAuthenticated: () => void;
}

export function LoginView({ onAuthenticated }: LoginViewProps) {
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!password || submitting) return;
    setSubmitting(true);
    setError("");
    try {
      await api.login(password);
      setPassword("");
      onAuthenticated();
    } catch (reason) {
      setError(reason instanceof APIError ? reason.message : "登录失败，请稍后重试");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="login-shell">
      <header className="login-header"><Brand /></header>
      <section className="login-stage" aria-labelledby="login-title">
        <form className="login-form" onSubmit={submit}>
          <div className="login-copy">
            <h1 id="login-title">登录终端</h1>
            <p>输入访问密码以继续</p>
          </div>
          <label className="field-label" htmlFor="access-password">访问密码</label>
          <div className={`password-field${error ? " has-error" : ""}`}>
            <input
              id="access-password"
              autoFocus
              autoComplete="current-password"
              type={showPassword ? "text" : "password"}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              placeholder="请输入密码"
              aria-invalid={Boolean(error)}
              aria-describedby={error ? "login-error" : undefined}
            />
            <button type="button" className="icon-button password-toggle" aria-label={showPassword ? "隐藏密码" : "显示密码"} onClick={() => setShowPassword((value) => !value)}>
              {showPassword ? <EyeOffIcon size={21}/> : <EyeIcon size={21}/>}
            </button>
          </div>
          <div className="login-error" id="login-error" role="alert">{error}</div>
          <button className="login-button" type="submit" disabled={!password || submitting}>
            {submitting ? "正在登录…" : "登录"}
          </button>
        </form>
      </section>
      <footer className="secure-footer"><span /><div><ShieldIcon size={26}/><b>安全连接</b></div><span /></footer>
    </main>
  );
}
