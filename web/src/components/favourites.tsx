// Each person's favourites in the dropdowns (specialization, industries, IT
// skills, certifications, countries): kept in their own settings and shown
// first, for them only. The lists themselves never change, and every item
// can still be found below the favourites or by typing.
import { type QueryClient, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../api/client";
import { countryName } from "../labels";
import { preferencesKey } from "./candidateColumns";
import { ALL_COUNTRIES } from "./countries";
import { COUNTRIES } from "./forms";
import "./favouriteStars.css";

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
  /** Loaded, so a star can be changed (no star until then). */
  ready: boolean;
  /** The last change could not be saved. */
  failed: boolean;
}

/**
 * Changes sent but not yet saved, by kind, for each query client: a second
 * star builds on the first even if a refetch brings back the older list
 * meanwhile.
 */
const unsaved = new WeakMap<QueryClient, Map<FavKind, string[]>>();

/** The signed-in person's favourites, and changing them (the screen changes at once). */
export function useFavourites() {
  const queryClient = useQueryClient();
  const prefs = useQuery({ queryKey: preferencesKey, queryFn: async () => unwrap(await api.GET("/v1/me/preferences")), staleTime: 60_000 });
  const pending = unsaved.get(queryClient) ?? new Map<FavKind, string[]>();
  unsaved.set(queryClient, pending);
  const save = useMutation({
    mutationKey: saveKey,
    // Saves run one after another; each sends one kind (the server keeps the others).
    scope: { id: "favourites" },
    mutationFn: async (change: Partial<Record<FavKind, string[]>>) => unwrap(await api.PUT("/v1/me/preferences", { body: { favourites: change } })),
    onMutate: async (change) => {
      await queryClient.cancelQueries({ queryKey: preferencesKey });
      queryClient.setQueryData(preferencesKey, (old: Prefs | undefined) => ({ ...old, favourites: { ...old?.favourites, ...change } }));
    },
    onSettled: async (_data, _error, change) => {
      for (const [kind, values] of Object.entries(change) as [FavKind, string[]][]) {
        if (pending.get(kind) === values) pending.delete(kind); // a later change of that kind stays
      }
      if (queryClient.isMutating({ mutationKey: saveKey }) <= 1) await queryClient.invalidateQueries({ queryKey: preferencesKey });
    },
  });
  const loaded = prefs.data !== undefined;
  const listOf = (kind: FavKind): string[] => pending.get(kind) ?? prefs.data?.favourites?.[kind] ?? [];
  const set = (kind: FavKind, values: string[]) => {
    if (!loaded) return; // never over what has not loaded
    pending.set(kind, values);
    save.mutate({ [kind]: values });
  };
  const of = (kind: FavKind): Fav => {
    const list = listOf(kind);
    return {
      list,
      has: (v) => list.some((x) => same(x, v)),
      toggle: (v) => {
        const now = listOf(kind);
        set(kind, now.some((x) => same(x, v)) ? now.filter((x) => !same(x, v)) : [...now, v]);
      },
      ready: loaded,
      failed: save.isError,
    };
  };
  return { of, set, loaded, error: save.error ?? prefs.error };
}

/** The favourites as they were when a menu opened: starring then shows at once, but nothing moves under the mouse. */
export function frozen(fav: Fav | undefined, order: readonly string[]): Fav | undefined {
  return fav && { ...fav, list: order, has: (v) => order.some((o) => same(o, v)) };
}

/** A line under a menu's items when a star could not be saved. */
export function StarFailed({ fav }: { fav?: Fav }) {
  return fav?.failed ? <p className="warning small fav-failed" role="alert">That favourite could not be saved: try again.</p> : null;
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
  const unknown = keep && !ALL_COUNTRIES.includes(keep) && !top.includes(keep) ? [keep] : [];
  const rest = [...byName().filter((c) => !top.includes(c)), ...unknown];
  return { top, topLabel: mine ? "★ Your favourites" : "Usual countries", rest };
}

/** Every country by its name, sorted once (names come from the browser, so not at load). */
let sorted: readonly string[] | undefined;
function byName(): readonly string[] {
  if (!sorted) {
    const names = new Map(ALL_COUNTRIES.map((c) => [c, countryName(c)]));
    sorted = [...ALL_COUNTRIES].sort((a, b) => (names.get(a) ?? a).localeCompare(names.get(b) ?? b));
  }
  return sorted;
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
  if (!fav.ready) return null;
  const on = fav.has(value);
  return (
    <button type="button" className={`fav-star${on ? " on" : ""}`} aria-pressed={on}
      aria-label={on ? `Remove ${label} from your favourites` : `Keep ${label} at the top (favourite)`}
      title={on ? "Your favourite: click to remove" : "Keep at the top for you"}
      onClick={(e) => { e.preventDefault(); e.stopPropagation(); fav.toggle(value); }}>{on ? "★" : "☆"}</button>
  );
}
