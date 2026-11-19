// Tags a claim to the client (a company or contact) or candidate it was for:
// one search over the records the person may see.
import { api, type Permission, type Schemas, unwrap } from "../../api/client";
import { type Picked, SearchPicker } from "../SearchPicker";

type Kind = Schemas["ClaimLinkKind"];
export type LinkValue = { kind: Kind; id: string; name: string };

const KIND_WORD: Record<Kind, string> = { company: "Company", contact: "Contact", candidate: "Candidate" };
const SHOWN = 5;

/** What each kind needs to be searched (and tagged). */
const NEEDS: Record<Kind, Permission> = { company: "companies:read", contact: "contacts:read", candidate: "candidates:read" };

async function search(text: string, kinds: Kind[]) {
  const found = await Promise.all(kinds.map(async (kind) => {
    switch (kind) {
      case "company": {
        const page = unwrap(await api.GET("/v1/companies", { params: { query: { q: text, limit: SHOWN } } }));
        return page.items.map((c) => ({ id: `company:${c.id}`, name: c.name, detail: KIND_WORD.company }));
      }
      case "contact": {
        const page = unwrap(await api.GET("/v1/contacts", { params: { query: { q: text, limit: SHOWN } } }));
        return page.items.map((c) => ({ id: `contact:${c.id}`, name: c.fullName, detail: [KIND_WORD.contact, c.companyName].filter(Boolean).join(" · ") }));
      }
      case "candidate": {
        const page = unwrap(await api.GET("/v1/candidates", { params: { query: { q: text, limit: SHOWN } } }));
        return page.items.map((c) => ({ id: `candidate:${c.id}`, name: c.fullName, detail: [KIND_WORD.candidate, c.currentTitle].filter(Boolean).join(" · ") }));
      }
    }
  }));
  return found.flat();
}

function parse(picked: Picked): LinkValue {
  const [kind, id] = picked.id.split(":") as [Kind, string];
  return { kind, id, name: picked.name };
}

export function LinkPicker({ id, value, permissions, onChange }: {
  id?: string;
  value: LinkValue | null;
  permissions: Permission[];
  onChange: (next: LinkValue | null) => void;
}) {
  const kinds = (Object.keys(NEEDS) as Kind[]).filter((k) => permissions.includes(NEEDS[k]));
  if (kinds.length === 0) return value ? <span>{value.name}</span> : <span className="muted">—</span>;
  return (
    <SearchPicker id={id} value={value ? { id: `${value.kind}:${value.id}`, name: value.name } : null}
      onChange={(p) => { onChange(p ? parse(p) : null); }}
      search={(text) => search(text, kinds)} queryKey={["claim-link", ...kinds]}
      placeholder="Client or candidate" noMatch="No matching client or candidate." />
  );
}

/** A claim's tag in words: "Farah Idris (Acme Berhad)", or the kind when hidden from the reader. */
export function linkWords(link: Schemas["ClaimLink"] | null | undefined): string {
  if (!link) return "";
  if (!link.name) return `A ${KIND_WORD[link.kind].toLowerCase()} you cannot see`;
  return link.detail ? `${link.name} (${link.detail})` : link.name;
}
