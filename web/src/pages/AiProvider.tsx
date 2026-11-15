// Settings > AI provider: which AI the company uses (DeepSeek, Claude or
// ChatGPT with its own API key, the server's own, or none), each provider's
// model and key, and whether they work. Keys are never shown back: only their
// last four characters.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { assistantKey } from "../api/keys";
import { ErrorMessage } from "../components/ErrorMessage";
import { formatWhen } from "../labels";

type Providers = Schemas["AiProviders"];
type Name = Schemas["AiProviderName"];
type Entry = Schemas["AiProviderEntry"];

const aiProvidersKey = ["ai-providers"] as const;

/** Each provider: its name, who makes it, the models offered (the first is the default) and where keys come from. */
export const PROVIDERS: Record<Name, { label: string; maker: string; models: string[]; keysAt: string }> = {
  deepseek: { label: "DeepSeek", maker: "DeepSeek", models: ["deepseek-v4-flash"], keysAt: "https://platform.deepseek.com/api_keys" },
  claude: { label: "Claude", maker: "Anthropic", models: ["claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-5-5"], keysAt: "https://platform.claude.com/settings/keys" },
  openai: { label: "ChatGPT", maker: "OpenAI", models: ["gpt-5.5"], keysAt: "https://platform.openai.com/api-keys" },
};

export function AiProvider() {
  const queryClient = useQueryClient();
  const state = useQuery({
    queryKey: aiProvidersKey,
    queryFn: async () => unwrap(await api.GET("/v1/ai-providers")),
    // Follow checks still running.
    refetchInterval: (q) => (q.state.data?.providers.some((p) => p.check?.status === "queued") ? 2000 : false),
  });
  const saved = async (st: Providers) => {
    queryClient.setQueryData(aiProvidersKey, st);
    await queryClient.invalidateQueries({ queryKey: assistantKey }); // the assistant may have switched on or off
  };
  const choose = useMutation({
    mutationFn: async (provider: Schemas["AiProviderChoice"]) => unwrap(await api.PUT("/v1/ai-providers", { body: { provider } })),
    onSuccess: saved,
  });
  const st = state.data;
  return (
    <main className="ai-settings">
      <h2>AI provider</h2>
      <ErrorMessage error={state.error ?? choose.error} />
      {st && (
        <>
          <section className="panel" aria-label="In use">
            <h3>In use</h3>
            <div className="ai-choice" role="radiogroup" aria-label="AI provider in use">
              <Choice value="off" label="Off" st={st} onChoose={choose.mutate} />
              {st.serverAvailable && <Choice value="server" label="Server default" st={st} onChoose={choose.mutate} />}
              {st.providers.map((p) => (
                <Choice key={p.provider} value={p.provider} label={PROVIDERS[p.provider].label} st={st} onChoose={choose.mutate}
                  disabled={p.keyHint === null} />
              ))}
            </div>
            <p className="muted small">Used by the assistant, CV vs job matching, market insights and CV reading.</p>
          </section>
          {!st.keysSupported && (
            <p className="warning" role="alert">This server cannot store API keys yet: HP_AI_KEYS_SECRET must be set.</p>
          )}
          <div className="ai-providers">
            {st.providers.map((p) => <ProviderCard key={p.provider} entry={p} active={st.active === p.provider} disabled={!st.keysSupported} onSaved={saved} />)}
          </div>
        </>
      )}
    </main>
  );
}

function Choice({ value, label, st, onChoose, disabled = false }: {
  value: Schemas["AiProviderChoice"]; label: string; st: Providers; onChoose: (v: Schemas["AiProviderChoice"]) => void; disabled?: boolean;
}) {
  // No choice yet means the server's own, which is "off" when it has none.
  const active = st.active === value || (value === "off" && st.active === "server" && !st.serverAvailable);
  return (
    <label className={`ai-choice-option${active ? " on" : ""}${disabled ? " disabled" : ""}`}>
      <input type="radio" name="ai-provider" value={value} checked={active} disabled={disabled}
        onChange={() => { onChoose(value); }} />
      {label}
    </label>
  );
}

function ProviderCard({ entry, active, disabled, onSaved }: {
  entry: Entry; active: boolean; disabled: boolean; onSaved: (st: Providers) => Promise<void>;
}) {
  const info = PROVIDERS[entry.provider];
  const [model, setModel] = useState(entry.model ?? info.models[0] ?? "");
  const [key, setKey] = useState("");
  const path = { params: { path: { aiProvider: entry.provider } } };
  const save = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/v1/ai-providers/{aiProvider}", {
      ...path, body: { model: model.trim(), ...(key.trim() ? { apiKey: key.trim() } : {}) },
    })),
    onSuccess: async (st) => { setKey(""); await onSaved(st); },
  });
  const check = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/ai-providers/{aiProvider}/check", path)),
    onSuccess: onSaved,
  });
  const remove = useMutation({
    mutationFn: async () => unwrap(await api.DELETE("/v1/ai-providers/{aiProvider}", path)),
    onSuccess: async (st) => { setKey(""); await onSaved(st); },
  });
  const hasKey = entry.keyHint !== null;
  const listId = `ai-models-${entry.provider}`;
  return (
    <section className={`panel ai-provider${active ? " active" : ""}`} aria-label={info.label}>
      <div className="row spread">
        <h3>{info.label}{info.maker !== info.label && <span className="muted small"> {info.maker}</span>}</h3>
        <Status entry={entry} active={active} />
      </div>
      {entry.check?.status === "failed" && entry.check.message && <p className="warning small">{sentence(entry.check.message)}</p>}
      <form onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
        <div className="field">
          <label htmlFor={`${listId}-model`}>Model</label>
          <input id={`${listId}-model`} required list={listId} value={model} disabled={disabled} onChange={(e) => { setModel(e.target.value); }} />
          <datalist id={listId}>{info.models.map((m) => <option key={m} value={m} />)}</datalist>
        </div>
        <div className="field">
          <label htmlFor={`${listId}-key`}>{hasKey ? `API key (saved: ${entry.keyHint ?? ""})` : "API key"}</label>
          <input id={`${listId}-key`} type="password" required={!hasKey} autoComplete="off" spellCheck={false} value={key} disabled={disabled}
            onChange={(e) => { setKey(e.target.value); }} />
        </div>
        <div className="row spread">
          <a href={info.keysAt} target="_blank" rel="noreferrer">Get a key</a>
          <span className="row">
            {hasKey && (
              <>
                <button type="button" className="link" disabled={check.isPending || entry.check?.status === "queued"}
                  onClick={() => { check.mutate(); }}>Check</button>
                <button type="button" className="link" disabled={remove.isPending}
                  onClick={() => { if (window.confirm(`Remove the ${info.label} key?${active ? " The AI switches off." : ""}`)) remove.mutate(); }}>
                  Remove key
                </button>
              </>
            )}
            <button type="submit" className="primary" disabled={disabled || save.isPending}>
              Save
            </button>
          </span>
        </div>
      </form>
      <ErrorMessage error={save.error ?? check.error ?? remove.error} />
    </section>
  );
}

/** The worker's messages start in lower case (they also end up mid-sentence elsewhere). */
const sentence = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

function Status({ entry, active }: { entry: Entry; active: boolean }) {
  if (entry.keyHint === null) return <span className="badge">No key</span>;
  const c = entry.check;
  return (
    <span className="row">
      {active && <span className="badge in-use">In use</span>}
      {c?.status === "queued" && <span className="badge queued" role="status">Checking…</span>}
      {c?.status === "ok" && <span className="badge valid" title={`Checked ${formatWhen(c.checkedAt)}`}>Working</span>}
      {c?.status === "failed" && <span className="badge invalid" title={c.message ?? ""}>Not working</span>}
    </span>
  );
}
