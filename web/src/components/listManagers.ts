// Changing the company's lists right from their dropdowns (owners and admins:
// settings:manage): adding a word typed, renaming one, removing one. Each
// change reads the list fresh first, so it never undoes a colleague's.
import { useQueryClient } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../api/client";
import { candidatesKey, fieldSettingsKey } from "../api/keys";
import type { Fav } from "./favourites";
import { industryKey } from "./IndustryPicker";
import { useCan } from "./useCan";

/** What a dropdown offers those who manage the list. */
export type ListManager = {
  /** Adds a word typed (or brings back a hidden one) and gives the value to choose. */
  add: (label: string) => Promise<string>;
  rename: (value: string, label: string) => Promise<void>;
  remove: (value: string) => Promise<void>;
  /** An item's own name to rename (a subindustry without its industry). */
  nameOf?: (value: string) => string;
  /** Said before removing: what happens to the records that have it. */
  removeNote: string;
};

type WordList = "specialization" | "it_skill" | "certification";
type OptionInput = Schemas["FieldOptionInput"];

const tidy = (s: string) => s.trim().replace(/\s+/g, " ");
const same = (a: string, b: string) => a.toLowerCase() === b.toLowerCase();

/** Adding, renaming and removing the specialization, IT skills or certifications words (undefined without settings:manage). */
export function useWordListManager(list: WordList, fav?: Fav): ListManager | undefined {
  const can = useCan("settings:manage");
  const queryClient = useQueryClient();
  if (!can) return undefined;
  const save = async (change: (options: OptionInput[]) => OptionInput[]) => {
    const fresh = unwrap(await api.GET("/v1/settings/fields"));
    const options = (fresh.lists.find((l) => l.name === list)?.options ?? []).map((o) => ({ key: o.key, label: o.label, hidden: o.hidden }));
    unwrap(await api.PUT("/v1/settings/fields/lists/{fieldList}", { params: { path: { fieldList: list } }, body: { options: change(options) } }));
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
        return options.map((o) => (o === had ? { ...o, hidden: false } : o));
      });
      return value;
    },
    rename: async (value, typed) => {
      const label = tidy(typed);
      await save((options) => options.map((o) => (o.key === value ? { ...o, label } : o)));
      // Candidates who had it have the new word now.
      await queryClient.invalidateQueries({ queryKey: candidatesKey });
      await queryClient.invalidateQueries({ queryKey: ["candidate"] });
      if (fav?.has(value)) { fav.toggle(value); fav.toggle(label); }
    },
    remove: async (value) => { await save((options) => options.filter((o) => o.key !== value)); },
    removeNote: "Candidates who have it keep it.",
  };
}

/** Adding, renaming and retiring industries (undefined without settings:manage). */
export function useIndustryManager(): ListManager | undefined {
  const can = useCan("settings:manage");
  const queryClient = useQueryClient();
  if (!can) return undefined;
  const fresh = async () => unwrap(await api.GET("/v1/industries"));
  const done = async () => { await queryClient.invalidateQueries({ queryKey: industryKey }); };
  const items = () => queryClient.getQueryData<Schemas["IndustryCatalogue"]>(industryKey)?.items ?? [];
  return {
    add: async (typed) => {
      const label = tidy(typed);
      const now = await fresh();
      const had = now.items.find((i) => !i.parentId && same(i.label, label));
      if (had && !had.retired) return had.id;
      const saved = had
        ? unwrap(await api.PATCH("/v1/industries/{industryId}", { params: { path: { industryId: had.id } },
          body: { catalogueVersion: now.version, label: had.label, aliases: had.aliases, retired: false } }))
        : unwrap(await api.POST("/v1/industries", { body: { catalogueVersion: now.version, label, aliases: [], retired: false } }));
      await done();
      return had?.id ?? saved.items.find((i) => !i.parentId && same(i.label, label))?.id ?? "";
    },
    rename: async (id, typed) => {
      const now = await fresh();
      const item = now.items.find((i) => i.id === id);
      if (!item) return;
      unwrap(await api.PATCH("/v1/industries/{industryId}", { params: { path: { industryId: id } },
        body: { catalogueVersion: now.version, label: tidy(typed), ...(item.parentId ? { parentId: item.parentId } : {}), aliases: item.aliases, retired: item.retired } }));
      await done();
    },
    remove: async (id) => {
      const now = await fresh();
      unwrap(await api.POST("/v1/industries/{industryId}/remove", { params: { path: { industryId: id } }, body: { catalogueVersion: now.version, action: "retire" } }));
      await done();
    },
    nameOf: (id) => items().find((i) => i.id === id)?.label ?? "",
    removeNote: "It is retired: candidates and companies that have it keep it. Delete it for good in Settings › Industries.",
  };
}
