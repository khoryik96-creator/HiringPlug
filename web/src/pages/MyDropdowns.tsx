// My settings › My dropdowns: each person's own favourites in the
// dropdowns (countries, specialization, IT skills, certifications,
// industries). Favourites come first for them only; nothing is removed from
// the lists, and every item can still be found below them or by typing.
import { useState } from "react";

import { ErrorMessage } from "../components/ErrorMessage";
import { type Fav, countryGroups, useFavourites } from "../components/favourites";
import { type Category, categoryLabel, categoryMatches, useIndustries } from "../components/IndustryPicker";
import { useFieldSettings } from "../components/useFieldSettings";
import { countryName, regionName } from "../labels";

const nameOf = (c: string) => regionName(c) ?? c;

export function MyDropdowns() {
  const favs = useFavourites();
  return (
    <main className="my-dropdowns">
      <h2>My dropdowns</h2>
      <p className="muted">
        Your favourites come first in your dropdowns, just for you; colleagues keep their own. Nothing is removed:
        everything else is still there below them, and typing finds any item. You can also click ☆ in a dropdown.
      </p>
      <ErrorMessage error={favs.error} />
      {!favs.loaded ? <p className="muted">Loading…</p> : (
        <>
          <Countries fav={favs.of("country")} set={(v) => { favs.set("country", v); }} />
          <WordList list="specialization" title="Specialization" fav={favs.of("specialization")} clear={() => { favs.set("specialization", []); }} />
          <WordList list="it_skill" title="IT skills" fav={favs.of("itSkill")} clear={() => { favs.set("itSkill", []); }} />
          <WordList list="certification" title="Professional certifications" fav={favs.of("certification")} clear={() => { favs.set("certification", []); }} />
          <Industries fav={favs.of("industry")} clear={() => { favs.set("industry", []); }} />
        </>
      )}
    </main>
  );
}

/** Countries: chosen ones always on top, in the order chosen. */
function Countries({ fav, set }: { fav: Fav; set: (v: string[]) => void }) {
  const g = countryGroups(undefined);
  const left = [...g.top, ...g.rest].filter((c) => !fav.has(c));
  return (
    <section className="panel fav-section" aria-labelledby="fav-countries">
      <h3 id="fav-countries">Countries <small className="muted">always on top</small></h3>
      {fav.list.length === 0 && <p className="muted small">None chosen: your dropdowns start with the usual ones ({g.top.slice(0, 4).map(nameOf).join(", ")}…).</p>}
      <div className="chips">
        {fav.list.map((c, i) => (
          <span key={c} className="chip on">
            <span>{nameOf(c)}</span>
            {i > 0 && <button type="button" className="link" aria-label={`Move ${nameOf(c)} up`} onClick={() => { set(swap(fav.list, i, i - 1)); }}>↑</button>}
            <button type="button" className="link" aria-label={`Remove ${nameOf(c)}`} onClick={() => { fav.toggle(c); }}>×</button>
          </span>
        ))}
        <select aria-label="Add a country" value="" onChange={(e) => { if (e.target.value) fav.toggle(e.target.value); }}>
          <option value="">+ Add a country</option>
          {left.map((c) => <option key={c} value={c}>{countryName(c)}</option>)}
        </select>
      </div>
    </section>
  );
}

const swap = (list: readonly string[], a: number, b: number) => {
  const out = [...list];
  [out[a], out[b]] = [out[b] ?? "", out[a] ?? ""];
  return out;
};

/** A Settings list (specialization, IT skills, certifications): a switch per word. */
function WordList({ list, title, fav, clear }: {
  list: "specialization" | "it_skill" | "certification"; title: string; fav: Fav; clear: () => void;
}) {
  const fs = useFieldSettings();
  const words = fs.options(list).map((o) => o.label);
  return (
    <section className="panel fav-section" aria-label={title}>
      <SectionHead title={title} fav={fav} clear={clear} />
      {words.length === 0 ? <p className="muted small">The list is empty (Settings › Dropdowns &amp; categories).</p> : (
        <div className="fav-switches">{words.map((w) => <FavSwitch key={w} fav={fav} value={w} label={w} />)}</div>
      )}
    </section>
  );
}

function SectionHead({ title, fav, clear }: { title: string; fav: Fav; clear: () => void }) {
  return (
    <div className="fav-head">
      <h3>{title}</h3>
      <span className="muted small">{fav.list.length === 0 ? "No favourites: the whole list shows, in its order" : `${String(fav.list.length)} favourite${fav.list.length === 1 ? "" : "s"} on top`}</span>
      {fav.list.length > 0 && <button type="button" className="link small" onClick={clear}>Clear</button>}
    </div>
  );
}

/** One item, on or off. */
function FavSwitch({ fav, value, label }: { fav: Fav; value: string; label: string }) {
  const on = fav.has(value);
  return (
    <button type="button" role="switch" aria-checked={on} className={`fav-switch${on ? " on" : ""}`} onClick={() => { fav.toggle(value); }}>
      <span className="fav-switch-label">{label}</span><span className="toggle" aria-hidden="true" />
    </button>
  );
}

/** Industries and their subindustries, folded under each industry; a search finds any. */
function Industries({ fav, clear }: { fav: Fav; clear: () => void }) {
  const industries = useIndustries();
  const [search, setSearch] = useState("");
  const items = industries.data?.items ?? [];
  const active = items.filter((i) => !i.retired && !items.find((p) => p.id === i.parentId)?.retired);
  const subsOf = (root: Category) => active.filter((c) => c.parentId === root.id && (!search || categoryMatches(c, search) || categoryMatches(root, search)));
  const roots = active.filter((i) => !i.parentId && (!search || categoryMatches(i, search) || subsOf(i).length > 0));
  return (
    <section className="panel fav-section" aria-label="Industries">
      <SectionHead title="Industries" fav={fav} clear={clear} />
      <ErrorMessage error={industries.error} />
      <input type="search" className="fav-search" placeholder="Find an industry…" aria-label="Find an industry" value={search}
        onChange={(e) => { setSearch(e.target.value); }} />
      {roots.map((root) => {
        const subs = subsOf(root);
        const starred = [root, ...subs].filter((s) => fav.has(s.id)).length;
        return (
          <details key={root.id} className="fav-industry" open={!!search}>
            <summary>{root.label}{starred > 0 && <small className="muted"> · {starred} starred</small>}</summary>
            <div className="fav-switches">
              <FavSwitch fav={fav} value={root.id} label={`${root.label} (all of it)`} />
              {subs.map((s) => <FavSwitch key={s.id} fav={fav} value={s.id} label={categoryLabel(s, items).replace(`${root.label} / `, "")} />)}
            </div>
          </details>
        );
      })}
      {industries.data && roots.length === 0 && <p className="muted small">No industry matches.</p>}
    </section>
  );
}
