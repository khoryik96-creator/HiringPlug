// Help: short guides to the main things, searchable, each linking to the
// screens it describes. Only what the person's role can use is shown.
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, unwrap } from "../api/client";
import { preferencesKey } from "../components/candidateColumns";
import { useHideGuide } from "../components/FirstRunGuide";
import { searchTopics, visibleTopics, type HelpTopic } from "../help/help";
import { HELP_TOPICS } from "../help/topics";
import { hashParam } from "../hashQuery";
import "../help/help.css";

export function Help({ permissions, topicId }: { permissions: readonly Permission[]; topicId: string }) {
  const topics = visibleTopics(HELP_TOPICS, permissions);
  if (topicId) {
    const topic = topics.find((t) => t.id === topicId);
    return (
      <main className="help-page">
        <p><a href={backHref()}>← All help topics</a></p>
        {topic ? <TopicView topic={topic} /> : (
          <section className="panel"><h2>Not found</h2><p>There is no help topic here, or it is about something your role does not use.</p></section>
        )}
      </main>
    );
  }
  return <HelpIndex topics={topics} permissions={permissions} />;
}

/** Back to the list, with the search that led here. */
function backHref(): string {
  const q = hashParam("q");
  return q ? `#/help?q=${encodeURIComponent(q)}` : "#/help";
}

function HelpIndex({ topics, permissions }: { topics: HelpTopic[]; permissions: readonly Permission[] }) {
  const [query, setQuery] = useState(() => hashParam("q"));
  const hits = searchTopics(topics, query);
  const searching = query.trim() !== "";
  const change = (q: string) => {
    setQuery(q);
    // Kept in the address, so Back from a topic returns to these results.
    window.history.replaceState(null, "", q.trim() ? `#/help?q=${encodeURIComponent(q)}` : "#/help");
  };
  const topicHref = (id: string) => (searching ? `#/help/${id}?q=${encodeURIComponent(query)}` : `#/help/${id}`);

  return (
    <main className="help-page">
      <h2>Help</h2>
      <p className="muted">Short guides to HiringPlug. Search, or pick a topic. You only see what your role can use.</p>
      <form role="search" aria-label="Help" onSubmit={(e) => {
        e.preventDefault();
        const first = hits[0];
        if (first) window.location.hash = topicHref(first.topic.id);
      }}>
        <label className="help-search">
          <svg aria-hidden="true" width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
            <circle cx="10.5" cy="10.5" r="6.5" /><path d="m16 16 4 4" />
          </svg>
          <input type="search" aria-label="Search help" placeholder="Search help, e.g. import Excel, invite, boolean"
            value={query} onChange={(e) => { change(e.target.value); }}
            onKeyDown={(e) => { if (e.key === "Escape" && query) { e.stopPropagation(); change(""); } }} />
        </label>
      </form>
      {searching ? (
        <section aria-label="Search results">
          <p role="status" className="muted">
            {hits.length ? `${String(hits.length)} ${hits.length === 1 ? "topic" : "topics"}` : `Nothing found for “${query.trim()}”. Try other words, or pick a topic below.`}
          </p>
          <ul className="help-results">
            {hits.map(({ topic, snippet }) => (
              <li key={topic.id}>
                <a href={topicHref(topic.id)}><strong>{topic.title}</strong></a>
                <span className="muted">{snippet ?? topic.summary}</span>
              </li>
            ))}
          </ul>
          {hits.length === 0 && <TopicList topics={topics} href={topicHref} />}
        </section>
      ) : <TopicList topics={topics} href={topicHref} />}
      <ShowGuideAgain permissions={permissions} />
    </main>
  );
}

function TopicList({ topics, href }: { topics: HelpTopic[]; href: (id: string) => string }) {
  return (
    <ul className="help-topics" aria-label="Help topics">
      {topics.map((t) => (
        <li key={t.id}>
          <a href={href(t.id)}>
            <strong>{t.title}</strong>
            <span>{t.summary}</span>
          </a>
        </li>
      ))}
    </ul>
  );
}

function TopicView({ topic }: { topic: HelpTopic }) {
  return (
    <article className="panel help-topic" aria-labelledby="help-topic-title">
      <h2 id="help-topic-title">{topic.title}</h2>
      <p className="help-summary">{topic.summary}</p>
      {topic.links.length > 0 && (
        <p className="help-links">
          {topic.links.map((l) => <a key={l.href} className="button small-button" href={l.href}>{l.label} →</a>)}
        </p>
      )}
      {topic.sections.map((s) => (
        <section key={s.heading} aria-label={s.heading}>
          <h3>{s.heading}</h3>
          <ul>{s.points.map((p) => <li key={p}>{p}</li>)}</ul>
        </section>
      ))}
    </article>
  );
}

/** Brings back the dashboard's checklist for someone who hid it. */
function ShowGuideAgain({ permissions }: { permissions: readonly Permission[] }) {
  const prefs = useQuery({ queryKey: preferencesKey, queryFn: async () => unwrap(await api.GET("/v1/me/preferences")), staleTime: 60_000 });
  const show = useHideGuide();
  if (!permissions.includes("pipeline:read") || prefs.data?.guideDismissed !== true) return null;
  return (
    <p className="help-guide-again">
      <button type="button" className="link" disabled={show.isPending} onClick={() => {
        show.mutate(false, { onSuccess: () => { window.location.hash = "#/"; } });
      }}>Show the getting-started checklist</button>
      <span className="muted"> on the Dashboard again.</span>
    </p>
  );
}
