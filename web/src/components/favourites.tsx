// Each person's favourites in the dropdowns (specialization, industries, IT
// skills, certifications, countries): kept in their own settings and shown
// first, for them only. The lists themselves never change, and every item
// can still be found below the favourites or by typing.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../api/client";
import { countryName } from "../labels";
import { preferencesKey } from "./candidateColumns";
import { ALL_COUNTRIES } from "./countries";
import { COUNTRIES } from "./forms";
import "./favourites.css";

export type FavKind = keyof Schemas["Favourites"];
type Prefs = Schemas["Preferences"];
const saveKey = ["me", "preferences", "favourites"] as const;
const same = (a: string, b: string) => a.toLowerCase() === b.toLowerCase();

/** One kind's favourites, and starring or unstarring one. */
export interface Fav {
  /** In the order they were starred. */
  list: readonly string[];
  has: (value: string) => boolean;
  toggle: (value: string) => void;
}

/** The signed-in person's favourites, and changing them (the screen changes at once). */
export function useFavourites() {
  const queryClient = useQueryClient();
  const prefs = useQuery({ queryKey: preferencesKey, queryFn: async () => unwrap(await api.GET("/v1/me/preferences")), staleTime: 60_000 });
  const save = useMutation({
    mutationKey: saveKey,
    // Saves run one after another, so the last change is the one kept.
    scope: { id: "favourites" },
    mutationFn: async (favourites: Schemas["Favourites"]) => unwrap(await api.PUT("/v1/me/preferences", { body: { favourites } })),
    onMutate: async (favourites) => {
      await queryClient.cancelQueries({ queryKey: preferencesKey });
      queryClient.setQueryData(preferencesKey, (old: Prefs | undefined) => ({ ...old, favourites }));
    },
    onSettled: async () => {
      if (queryClient.isMutating({ mutationKey: saveKey }) <= 1) await queryClient.invalidateQueries({ queryKey: preferencesKey });
    },
  });
  // What is saved (or about to be): a quick second star builds on the first.
  const current = () => queryClient.getQueryData<Prefs>(preferencesKey)?.favourites ?? {};
  const loaded = prefs.data !== undefined;
  const set = (kind: FavKind, values: string[]) => {
    if (loaded) save.mutate({ ...current(), [kind]: values }); // never over what has not loaded
  };
  const of = (kind: FavKind): Fav => {
    const list = prefs.data?.favourites?.[kind] ?? [];
    return {
      list,
      has: (v) => list.some((x) => same(x, v)),
      toggle: (v) => {
        const now = current()[kind] ?? [];
        set(kind, now.some((x) => same(x, v)) ? now.filter((x) => !same(x, v)) : [...now, v]);
      },
    };
  };
  return { of, set, loaded, error: save.error ?? prefs.error };
}

/** The favourites among `items` (in the order starred), and the rest (in the list's order). */
export function splitFavourites<T>(items: readonly T[], value: (t: T) => string, fav?: Fav): { top: T[]; rest: T[] } {
  if (!fav || fav.list.length === 0) return { top: [], rest: [...items] };
  const top = fav.list.flatMap((f) => items.filter((i) => same(value(i), f)).slice(0, 1));
  return { top, rest: items.filter((i) => !fav.has(value(i))) };
}

/**
 * A country dropdown's two groups: the person's favourites (or, until they
 * choose some, the agency's usual countries), then every other country by
 * name. `keep` is a value already chosen, offered even if unknown.
 */
export function countryGroups(fav?: Fav, keep?: string | null): { top: string[]; topLabel: string; rest: string[] } {
  const mine = fav && fav.list.length > 0;
  const top = mine ? [...fav.list] : [...COUNTRIES];
  const known = new Set([...ALL_COUNTRIES, ...top]);
  const rest = [...ALL_COUNTRIES, ...(keep && !known.has(keep) ? [keep] : [])]
    .filter((c) => !top.includes(c))
    .sort((a, b) => countryName(a).localeCompare(countryName(b)));
  return { top, topLabel: mine ? "★ Your favourites" : "Usual countries", rest };
}

/** A native country <select>'s options: favourites on top, then all the others. */
export function CountryOptions({ fav, keep }: { fav?: Fav; keep?: string | null }) {
  const g = countryGroups(fav, keep);
  return (
    <>
      <optgroup label={g.topLabel}>{g.top.map((c) => <option key={c} value={c}>{countryName(c)}</option>)}</optgroup>
      <optgroup label="All countries">{g.rest.map((c) => <option key={c} value={c}>{countryName(c)}</option>)}</optgroup>
    </>
  );
}

/** A country <datalist>'s options (a datalist has no groups: favourites come first). */
export function CountryList({ id, fav }: { id: string; fav?: Fav }) {
  const g = countryGroups(fav);
  return <datalist id={id}>{[...g.top, ...g.rest].map((c) => <option key={c} value={c}>{countryName(c)}</option>)}</datalist>;
}

/** The star beside an item: on, it is a favourite. */
export function Star({ fav, value, label }: { fav: Fav; value: string; label: string }) {
  const on = fav.has(value);
  return (
    <button type="button" className={`fav-star${on ? " on" : ""}`} aria-pressed={on}
      aria-label={on ? `Remove ${label} from your favourites` : `Keep ${label} at the top (favourite)`}
      title={on ? "Your favourite: click to remove" : "Keep at the top for you"}
      onClick={(e) => { e.preventDefault(); e.stopPropagation(); fav.toggle(value); }}>{on ? "★" : "☆"}</button>
  );
}
