// Two-step sign-in: the code from an authenticator app after the password,
// adding the app (a QR code to scan, or the key to type), recovery codes, and
// your own settings under Your account.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import qrcode from "qrcode-generator";
import { useMemo, useState } from "react";

import { api, ApiError, type Schemas, unwrap } from "../api/client";
import { setToken } from "../auth";
import { CopyButton } from "./CopyButton";
import { ErrorMessage } from "./ErrorMessage";

/** What a sign-in (or a password link) asks for next: a code, or setting the app up first. */
export type NextStep = { kind: "code" | "setup"; challenge: string } | null;

export function nextStepOf(error: unknown): NextStep {
  if (!(error instanceof ApiError)) return null;
  const challenge = typeof error.details.challenge === "string" ? error.details.challenge : "";
  if (!challenge) return null;
  if (error.code === "TWO_STEP_REQUIRED") return { kind: "code", challenge };
  if (error.code === "TWO_STEP_SETUP_REQUIRED") return { kind: "setup", challenge };
  return null;
}

/** A QR code as SVG squares (no script, no image request). */
export function QrCode({ text, label }: { text: string; label: string }) {
  const cells = useMemo(() => {
    const qr = qrcode(0, "M");
    qr.addData(text);
    qr.make();
    const n = qr.getModuleCount();
    const dark: [number, number][] = [];
    for (let r = 0; r < n; r++) for (let c = 0; c < n; c++) if (qr.isDark(r, c)) dark.push([c, r]);
    return { n, dark };
  }, [text]);
  const size = cells.n + 8;
  return (
    <svg className="qr-code" role="img" aria-label={label} viewBox={`-4 -4 ${String(size)} ${String(size)}`} width="180" height="180" shapeRendering="crispEdges">
      <rect x="-4" y="-4" width={size} height={size} fill="#fff" />
      {cells.dark.map(([x, y]) => <rect key={`${String(x)}-${String(y)}`} x={x} y={y} width="1" height="1" fill="#111" />)}
    </svg>
  );
}

function CodeInput({ value, onChange, recovery = false }: { value: string; onChange: (v: string) => void; recovery?: boolean }) {
  return (
    <label className="field full two-step-code">{recovery ? "Code from the app, or a recovery code" : "Code from the app"}
      <input required autoFocus autoComplete="one-time-code" inputMode={recovery ? "text" : "numeric"} maxLength={20}
        placeholder={recovery ? "123456 or abcde-fghjk" : "123456"} value={value} onChange={(e) => { onChange(e.target.value); }} />
    </label>
  );
}

function RecoveryCodes({ codes }: { codes: string[] }) {
  return (
    <div className="recovery-codes">
      <p>Keep these recovery codes somewhere safe (a password manager, or printed). Each signs you in once if you lose your phone. They are shown only now.</p>
      <ol>{codes.map((c) => <li key={c}><code>{c}</code></li>)}</ol>
      <CopyButton text={codes.join("\n")} what="recovery codes" />
    </div>
  );
}

/** Adding the app: scan the QR code (or type the key), then the first code. */
function SetupSteps({ setup, code, onCode }: { setup: Schemas["TwoStepSetup"]; code: string; onCode: (v: string) => void }) {
  return (
    <>
      <ol className="full two-step-how">
        <li>Open an authenticator app on your phone (Microsoft or Google Authenticator, 1Password, Authy).</li>
        <li>Scan this code with it:<QrCode text={setup.uri} label="QR code for your authenticator app" /></li>
        <li>Or type this key: <code className="two-step-key">{setup.secret.replace(/(.{4})/g, "$1 ").trim()}</code></li>
        <li>Enter the 6-digit code the app shows.</li>
      </ol>
      <CodeInput value={code} onChange={onCode} />
    </>
  );
}

/** After the password: the code, or setting the app up when the company requires it. */
export function TwoStepAtSignIn({ next, onSignedIn, onCancel }: { next: NonNullable<NextStep>; onSignedIn: () => void; onCancel: () => void }) {
  const [code, setCode] = useState("");
  const [codes, setCodes] = useState<string[] | null>(null);
  const [token, setPending] = useState("");
  const setup = useQuery({
    queryKey: ["two-step-setup", next.challenge], enabled: next.kind === "setup", gcTime: 0, staleTime: Infinity, retry: false,
    queryFn: async () => unwrap(await api.POST("/v1/auth/two-step/setup", { body: { challenge: next.challenge } })),
  });
  const send = useMutation({
    mutationFn: async () => next.kind === "code"
      ? { session: unwrap(await api.POST("/v1/auth/two-step", { body: { challenge: next.challenge, code } })), recoveryCodes: null }
      : unwrap(await api.POST("/v1/auth/two-step/setup/confirm", { body: { challenge: next.challenge, code } })),
    onSuccess: (r) => {
      if (r.recoveryCodes) { setPending(r.session.token); setCodes(r.recoveryCodes); return; } // shown before going on
      setToken(r.session.token);
      onSignedIn();
    },
  });
  if (codes) {
    return (
      <section className="form-grid" aria-label="Two-step sign-in is on">
        <p className="full" role="status"><strong>Two-step sign-in is on.</strong></p>
        <RecoveryCodes codes={codes} />
        <div className="row full"><button type="button" onClick={() => { setToken(token); onSignedIn(); }}>Continue</button></div>
      </section>
    );
  }
  return (
    <form className="form-grid" aria-label="Two-step sign-in" onSubmit={(e) => { e.preventDefault(); send.mutate(); }}>
      {next.kind === "code" ? <>
        <p className="full">Enter the code from your authenticator app.</p>
        <CodeInput value={code} onChange={setCode} recovery />
      </> : <>
        <p className="full">Your company requires two-step sign-in: add an authenticator app to finish signing in.</p>
        {setup.data && <SetupSteps setup={setup.data} code={code} onCode={setCode} />}
        <ErrorMessage error={setup.error} />
      </>}
      <ErrorMessage error={send.error} />
      <div className="row full">
        <button type="submit" disabled={send.isPending || (next.kind === "setup" && !setup.data)}>{next.kind === "code" ? "Sign in" : "Turn on and sign in"}</button>
        <button type="button" className="link" onClick={onCancel}>Start again</button>
      </div>
    </form>
  );
}

const statusKey = ["me", "two-step"] as const;

/** Your account › Two-step sign-in: set it up, new recovery codes, or turn it off (with your password, or the app's code if you have none). */
export function MyTwoStep({ hasPassword = true }: { hasPassword?: boolean }) {
  const cache = useQueryClient();
  const status = useQuery({ queryKey: statusKey, queryFn: async () => unwrap(await api.GET("/v1/me/two-step")) });
  const [mode, setMode] = useState<"" | "setup" | "codes" | "off">("");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [codes, setCodes] = useState<string[] | null>(null);
  // Each form starts empty.
  const open = (next: typeof mode) => { setCode(""); setPassword(""); setCodes(null); setMode(next); };
  const start = useMutation({ mutationFn: async () => unwrap(await api.POST("/v1/me/two-step/setup")) });
  const done = async (r: { recoveryCodes: string[] }) => { setCodes(r.recoveryCodes); setMode(""); await cache.invalidateQueries({ queryKey: statusKey }); };
  const confirm = useMutation({ mutationFn: async () => unwrap(await api.POST("/v1/me/two-step/confirm", { body: { code } })), onSuccess: done });
  const renew = useMutation({ mutationFn: async () => unwrap(await api.POST("/v1/me/two-step/recovery-codes", { body: { code } })), onSuccess: done });
  const off = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/me/two-step/disable", { body: hasPassword ? { password } : { code } })),
    onSuccess: async () => { setMode(""); await cache.invalidateQueries({ queryKey: statusKey }); },
  });
  const s = status.data;
  return (
    <section className="panel" aria-label="Two-step sign-in">
      <h3>Two-step sign-in</h3>
      <ErrorMessage error={status.error} />
      {s && <p>{s.enabled
        ? <><span className="badge accepted">On</span> {hasPassword
          ? "A code from your authenticator app is asked after your password."
          : "You sign in with Microsoft, which checks it is you with your work account's own security; the code from your app is asked only if you ever sign in with a password."}
          {" "}{s.recoveryCodesLeft} recovery {s.recoveryCodesLeft === 1 ? "code" : "codes"} left.</>
        : <><span className="badge draft">Off</span> {hasPassword
          ? "Add an authenticator app so a stolen password alone cannot sign in as you."
          : "You sign in with Microsoft, which checks it is you with your work account's own security (HyPlug's code is not asked then). Add an app only if you will also sign in with a password."}</>}
        {s.required && <span className="muted"> Your company requires it.</span>}</p>}
      {codes && <RecoveryCodes codes={codes} />}
      {s && mode === "" && (
        <div className="row">
          <button type="button" onClick={() => { open("setup"); start.mutate(); }}>{s.enabled ? "Move to another app" : "Set up"}</button>
          {s.enabled && <button type="button" className="link" onClick={() => { open("codes"); }}>New recovery codes</button>}
          {s.enabled && !s.required && <button type="button" className="link danger" onClick={() => { open("off"); }}>Turn off</button>}
        </div>
      )}
      {mode === "setup" && (
        <form className="form-grid" onSubmit={(e) => { e.preventDefault(); confirm.mutate(); }}>
          {start.data && <SetupSteps setup={start.data} code={code} onCode={setCode} />}
          <ErrorMessage error={start.error ?? confirm.error} />
          <div className="row full"><button type="submit" disabled={!start.data || confirm.isPending}>Turn on</button>
            <button type="button" className="link" onClick={() => { setMode(""); }}>Cancel</button></div>
        </form>
      )}
      {mode === "codes" && (
        <form className="form-grid" onSubmit={(e) => { e.preventDefault(); renew.mutate(); }}>
          <p className="full muted">Your old recovery codes stop working.</p>
          <CodeInput value={code} onChange={setCode} />
          <ErrorMessage error={renew.error} />
          <div className="row full"><button type="submit" disabled={renew.isPending}>Make new codes</button>
            <button type="button" className="link" onClick={() => { setMode(""); }}>Cancel</button></div>
        </form>
      )}
      {mode === "off" && (
        <form className="form-grid" onSubmit={(e) => { e.preventDefault(); off.mutate(); }}>
          {hasPassword
            ? <label className="field">Your password<input type="password" required autoComplete="current-password" value={password} onChange={(e) => { setPassword(e.target.value); }} /></label>
            : <CodeInput value={code} onChange={setCode} />}
          <ErrorMessage error={off.error} />
          <div className="row full"><button type="submit" className="danger" disabled={off.isPending}>Turn off</button>
            <button type="button" className="link" onClick={() => { setMode(""); }}>Cancel</button></div>
        </form>
      )}
    </section>
  );
}

/** Roles and access › Two-step sign-in for everyone (owners). */
export function RequireTwoStep() {
  const cache = useQueryClient();
  const status = useQuery({ queryKey: statusKey, queryFn: async () => unwrap(await api.GET("/v1/me/two-step")) });
  const set = useMutation({
    mutationFn: async (required: boolean) => unwrap(await api.PUT("/v1/agency/two-step", { body: { required } })),
    onSuccess: async () => { await cache.invalidateQueries({ queryKey: statusKey }); await cache.invalidateQueries({ queryKey: ["team"] }); },
  });
  const s = status.data;
  if (!s) return null;
  return (
    <section className="panel" aria-label="Two-step sign-in for everyone">
      <h3>Two-step sign-in for everyone</h3>
      <label className="row">
        <input type="checkbox" checked={s.required} disabled={set.isPending || (!s.required && !s.enabled)}
          onChange={(e) => {
            if (!e.target.checked || window.confirm("Require two-step sign-in? Everyone without it is signed out and sets it up at their next sign-in.")) set.mutate(e.target.checked);
          }} />
        Require a code from an authenticator app after the password
      </label>
      {!s.enabled && !s.required && <p className="muted small">Set it up for yourself first (<a href="#/account">Your account</a>).</p>}
      <ErrorMessage error={set.error} />
    </section>
  );
}
