import type { Schemas } from "../api/client";

type Export = Schemas["CandidateDataExport"];

/** A copy of a candidate's data as plain text: what is sent to them for an access request. */
export function exportText(e: Export): string {
  const lines = [
    `Personal data held about ${e.candidateName} by ${e.company}`,
    `As of ${e.exportedAt.slice(0, 10)}`,
    "",
  ];
  for (const s of e.sections) {
    lines.push(s.title.toUpperCase(), "-".repeat(s.title.length));
    if (s.items.length === 0) lines.push("(none)");
    s.items.forEach((item, i) => {
      if (i > 0) lines.push("");
      for (const f of item.fields) {
        const [first = "", ...rest] = f.value.split("\n");
        lines.push(`${f.label}: ${first}`, ...rest.map((l) => `    ${l}`));
      }
    });
    lines.push("");
  }
  if (e.notIncluded.length > 0) lines.push("NOT INCLUDED IN THIS COPY", ...e.notIncluded.map((n) => `- ${n}`), "");
  return lines.join("\n");
}

/** Hands text to the browser to save as a file. */
export function saveText(text: string, fileName: string, type: string): void {
  const url = URL.createObjectURL(new Blob([text], { type }));
  const a = Object.assign(window.document.createElement("a"), { href: url, download: fileName });
  a.click();
  setTimeout(() => { URL.revokeObjectURL(url); }, 10_000);
}
