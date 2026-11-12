// A call note's lines, "RFL: wants regional scope", as label and answer.
/** A call's lines "RFL: wants regional scope" as label and answer; its "Call on … by …" line is in the header. */
export function callLines(body: string): { label: string | null; text: string }[] {
  return body.split("\n").filter((l) => l.trim() && !/^Call on .+ by .+$/.test(l.trim())).map((l) => {
    const m = /^([^:\n]{1,40}):\s+(.+)$/.exec(l.trim());
    return m ? { label: m[1] ?? null, text: m[2] ?? "" } : { label: null, text: l.trim() };
  });
}

/** A call note as a short question-and-answer list. */
export function CallQA({ body }: { body: string }) {
  return (
    <dl className="call-qa">
      {callLines(body).map((l, i) => (
        l.label ? <div key={i}><dt>{l.label}</dt><dd>{l.text}</dd></div> : <div key={i} className="plain"><dd>{l.text}</dd></div>
      ))}
    </dl>
  );
}
