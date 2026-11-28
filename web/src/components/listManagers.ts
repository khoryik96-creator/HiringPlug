// Changing the company's lists right from their dropdowns (owners and admins:
// settings:manage): adding a word typed, renaming one, removing one. Each
// change reads the list fresh first, so it never undoes a colleague's. The
// server carries a rename over to every candidate and everyone's favourites.
import { type QueryClient, useQueryClient } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../api/client";
import { candidatesKey, fieldSettingsKey } from "../api/keys";
import { industryKey } from "./IndustryPicker";
import { useCan } from "./useCan";

/** What a dropdown offers those who manage the list. */
export type ListManager = {
  /** Adds a word typed (or brings back a hidden or retired one) and gives the value to choose. */
  add: (label: string) => Promise<string>;
  /** Left out where renaming would leave an open form behind (Edit profile). */
  rename?: (value: string, label: string) => Promise<void>;
  remove?: (value: string) => Promise<void>;
  /** Whether an item shown is in the list (a record's own word may not be): only those get ✎ and ×. */
  manages: (value: string) => boolean;
  /** Whether a word typed is an item already, so there is nothing to add. */
  knows: (typed: string) => boolean;
  /** An item's own name to rename (a subindustry without its industry). */
  nameOf?: (value: string) => string;
  /** Said before removing: what happens to the records that have it. */
  removeNote: string;
};

type WordList = "specialization" | "it_skill" | "certification";
type OptionInput = Schemas["FieldOptionInput"];
type Catalogue = Schemas["IndustryCatalogue"];

const tidy = (s: string) => s.trim().replace(/\s+/g, " ");
const same = (a: string, b: string) => a.toLowerCase() === b.toLowerCase();

/** Records that had the old name (and their versions) are read again. */
async function refreshRecords(queryClient: QueryClient) {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: candidatesKey }),
    queryClient.invalidateQueries({ queryKey: ["candidate"] }),
    queryClient.invalidateQueries({ queryKey: ["companies"] }),
    queryClient.invalidateQueries({ queryKey: ["company"] }),
  ]);
}

/** A change refused because the item is not (or no longer) in the list. */
const notInList = (value: string) => new Error(`“${value}” is not in the list (it is only on this record), so there is nothing to change there.`);

/** Adding, renaming and removing the specialization, IT skills or certifications words (undefined without settings:manage). */
export function useWordListManager(list: WordList): ListManager | undefined {
  const can = useCan("settings:manage");
  const queryClient = useQueryClient();
  if (!can) return undefined;
  const cached = () => queryClient.getQueryData<Schemas["FieldSettings"]>(fieldSettingsKey)?.lists.find((l) => l.name === list)?.options ?? [];
  /** Saves the list as change makes it from the fresh one (nothing when change gives null). */
  const save = async (change: (options: OptionInput[]) => OptionInput[] | null) => {
    const fresh = unwrap(await api.GET("/v1/settings/fields"));
    const options = (fresh.lists.find((l) => l.name === list)?.options ?? []).map((o) => ({ key: o.key, label: o.label, hidden: o.hidden }));
    const next = change(options);
    if (!next) return;
    unwrap(await api.PUT("/v1/settings/fields/lists/{fieldList}", { params: { path: { fieldList: list } }, body: { options: next } }));
    await queryClient.invalidateQueries({ queryKey: fieldSettingsKey });
  };
  return {
    add: async (typed) => {
      const label = tidy(typed);
      let value = label;
      await save((options) => {
        const had = options.find((o) => same(o.label, label));
        if (!had) return [...options, { label, hidden: false }];
        value = had.label;
        return had.hidden ? options.map((o) => (o === had ? { ...o, hidden: false } : o)) : null; // there already
      });
      return value;
    },
    rename: async (value, typed) => {
      const label = tidy(typed);
      await save((options) => {
        if (!options.some((o) => o.key === value)) throw notInList(value);
        return options.map((o) => (o.key === value ? { ...o, label } : o));
      });
      await refreshRecords(queryClient); // candidates who had it have the new word now
    },
    remove: async (value) => {
      await save((options) => {
        if (!options.some((o) => o.key === value)) throw notInList(value);
        return options.filter((o) => o.key !== value);
      });
    },
    manages: (value) => cached().some((o) => o.key === value),
    knows: (typed) => cached().some((o) => same(o.label, tidy(typed)) && !o.hidden),
    removeNote: "Candidates who have it keep it.",
  };
}

/** Adding, renaming and retiring industries (undefined without settings:manage). */
export function useIndustryManager(): ListManager | undefined {
  const can = useCan("settings:manage");
  const queryClient = useQueryClient();
  if (!can) return undefined;
  const fresh = async () => unwrap(await api.GET("/v1/industries"));
  const items = () => queryClient.getQueryData<Catalogue>(industryKey)?.items ?? [];
  /** The catalogue saved, at once: a choice made right after uses its version. */
  const saved = async (catalogue: Catalogue) => {
    queryClient.setQueryData(industryKey, catalogue);
    await queryClient.invalidateQueries({ queryKey: industryKey });
  };
  /** An item called this (its own name or another name for it), at any level. */
  const named = (all: Catalogue["items"], label: string) =>
    all.find((i) => same(i.label, label)) ?? all.find((i) => i.aliases.some((a) => same(a, label)));
  return {
    add: async (typed) => {
      const label = tidy(typed);
      let now = await fresh();
      const had = named(now.items, label);
      if (had && !had.retired && !now.items.find((p) => p.id === had.parentId)?.retired) return had.id;
      if (!had) {
        now = unwrap(await api.POST("/v1/industries", { body: { catalogueVersion: now.version, label, aliases: [], retired: false } }));
        await saved(now);
        return named(now.items, label)?.id ?? "";
      }
      // Brought back: its industry first (a subindustry cannot be in use under a retired one).
      for (const item of [now.items.find((p) => p.id === had.parentId), had]) {
        if (!item?.retired) continue;
        now = unwrap(await api.PATCH("/v1/industries/{industryId}", { params: { path: { industryId: item.id } },
          body: { catalogueVersion: now.version, label: item.label, ...(item.parentId ? { parentId: item.parentId } : {}), aliases: item.aliases, retired: false } }));
      }
      await saved(now);
      return had.id;
    },
    rename: async (id, typed) => {
      const now = await fresh();
      const item = now.items.find((i) => i.id === id);
      if (!item) throw notInList(typed);
      await saved(unwrap(await api.PATCH("/v1/industries/{industryId}", { params: { path: { industryId: id } },
        body: { catalogueVersion: now.version, label: tidy(typed), ...(item.parentId ? { parentId: item.parentId } : {}), aliases: item.aliases, retired: item.retired } })));
      await refreshRecords(queryClient); // their industry summaries (and versions) changed
    },
    remove: async (id) => {
      const now = await fresh();
      await saved(unwrap(await api.POST("/v1/industries/{industryId}/remove", { params: { path: { industryId: id } }, body: { catalogueVersion: now.version, action: "retire" } })));
      await refreshRecords(queryClient);
    },
    manages: (id) => items().some((i) => i.id === id),
    knows: (typed) => {
      const all = items();
      const item = named(all, tidy(typed));
      return !!item && !item.retired && !all.find((p) => p.id === item.parentId)?.retired;
    },
    nameOf: (id) => items().find((i) => i.id === id)?.label ?? "",
    removeNote: "It is retired, with its subindustries: candidates and companies that have them keep them. Delete it for good in Settings › Industries.",
  };
}
