// A template as one candidate would get it: pick a candidate and the fields
// (name, expected salary...) show that candidate's values as you write. The
// template itself keeps its fields; nothing is sent or changed.
import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { api, unwrap } from "../../api/client";
import { ErrorMessage } from "../ErrorMessage";
import { showBanners, withoutCaretMarks } from "./richText";

const timeZone = () => Intl.DateTimeFormat().resolvedOptions().timeZone;

/** The value after it has stopped changing for a moment (a string: compared by value). */
function useSettled(value: string, ms = 300): string {
  const [settled, setSettled] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => { setSettled(value); }, ms);
    return () => { clearTimeout(t); };
  }, [value, ms]);
  return settled;
}

type Props = { subject: string; html: string; bannerUrls: Record<string, string> };

export function TemplatePreview({ subject, html, bannerUrls }: Props) {
  const [q, setQ] = useState("");
  const [who, setWho] = useState<{ id: string; name: string } | null>(null);
  const search = useSettled(q.trim());
  const found = useQuery({
    queryKey: ["template-preview-candidates", search],
    queryFn: async () => unwrap(await api.GET("/v1/candidates", { params: { query: { q: search, limit: 5 } } })).items,
    enabled: search.length > 1 && !who,
  });
  const settledSubject = useSettled(subject);
  const settledHtml = useSettled(withoutCaretMarks(html));
  const preview = useQuery({
    queryKey: ["template-preview", who?.id, settledSubject, settledHtml],
    queryFn: async () => unwrap(await api.POST("/v1/email-previews", {
      body: { subject: settledSubject, bodyHtml: settledHtml, candidateId: who?.id, timeZone: timeZone() },
    })),
    enabled: !!who,
    placeholderData: (previous) => previous,
  });
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => { showBanners(box.current, bannerUrls); }, [preview.data, bannerUrls]);

  return (
    <section className="template-preview full" aria-label="Preview with a candidate">
      <div className="row">
        <strong>Preview with a candidate</strong>
        {who ? (
          <>
            <span>{who.name}</span>
            <button type="button" className="link" onClick={() => { setWho(null); setQ(""); }}>Change</button>
          </>
        ) : (
          <input aria-label="Candidate to preview with" placeholder="Search a candidate by name" value={q} onChange={(e) => { setQ(e.target.value); }} />
        )}
      </div>
      {!who && found.data && found.data.length > 0 && (
        <ul className="preview-hits">
          {found.data.map((c) => (
            <li key={c.id}>
              <button type="button" className="link" onClick={() => { setWho({ id: c.id, name: c.fullName }); }}>{c.fullName}</button>
              {c.currentTitle && <span className="muted small"> · {c.currentTitle}</span>}
            </li>
          ))}
        </ul>
      )}
      {!who && <p className="muted small">The fields you put in show this candidate&apos;s values here as you write; the template keeps the fields.</p>}
      <ErrorMessage error={who ? preview.error : found.error} />
      {who && preview.data && (
        <div className="email-preview">
          <p><span className="muted">Subject:</span> {preview.data.subject}</p>
          <div ref={box} className="email-body formatted" dangerouslySetInnerHTML={{ __html: preview.data.bodyHtml }} />
          {preview.data.missing.length > 0 && (
            <p className="warning small">No value for this candidate: {preview.data.missing.join(", ")}</p>
          )}
        </div>
      )}
    </section>
  );
}
