// Folders of candidates, contacts or companies: the signed-in person's own and
// those shared with them, and the changes made to them. Changing what is in a
// folder refreshes the list it filters.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type React from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { candidatesKey, companiesKey, contactsKey, foldersKey, usersKey } from "../../api/keys";

export type Folder = Schemas["Folder"];
export type FolderKind = Schemas["FolderKind"];

/** The most records put in or taken out of a folder at once (the API's limit). */
export const MAX_AT_ONCE = 500;

/** "candidate" → "candidates" (and the like), for labels. */
export const plural = (kind: FolderKind, n = 2) => (n === 1 ? kind : kind === "company" ? "companies" : `${kind}s`);

const listKey = { candidate: candidatesKey, contact: contactsKey, company: companiesKey } as const;

export function useFolderList(kind: FolderKind, enabled = true) {
  return useQuery({
    queryKey: [...foldersKey, kind],
    queryFn: async () => unwrap(await api.GET("/v1/folders", { params: { query: { kind } } })).items,
    enabled,
  });
}

/** The agency's people (to share with, and to name a folder's owner). */
export function useUsers() {
  return useQuery({ queryKey: usersKey, queryFn: async () => unwrap(await api.GET("/v1/users")).items, staleTime: 60_000 });
}

/** Everything one can do to folders; each refreshes the folders (and the list, when records move). */
export function useFolderChanges(kind: FolderKind) {
  const queryClient = useQueryClient();
  const refresh = (items = false) => {
    void queryClient.invalidateQueries({ queryKey: foldersKey });
    if (items) void queryClient.invalidateQueries({ queryKey: listKey[kind] });
  };
  return {
    create: useMutation({
      mutationFn: async (name: string) => unwrap(await api.POST("/v1/folders", { body: { kind, name } })),
      onSuccess: () => { refresh(); },
    }),
    rename: useMutation({
      mutationFn: async ({ folder, name }: { folder: Folder; name: string }) =>
        unwrap(await api.PATCH("/v1/folders/{folderId}", { params: { path: { folderId: folder.id } }, body: { name, version: folder.version } })),
      onSuccess: () => { refresh(); },
    }),
    remove: useMutation({
      mutationFn: async (folderId: string) => { unwrap(await api.DELETE("/v1/folders/{folderId}", { params: { path: { folderId } } })); },
      onSuccess: () => { refresh(); },
    }),
    share: useMutation({
      mutationFn: async ({ folderId, userId }: { folderId: string; userId: string }) =>
        unwrap(await api.PUT("/v1/folders/{folderId}/shares/{userId}", { params: { path: { folderId, userId } } })),
      onSuccess: () => { refresh(); },
    }),
    unshare: useMutation({
      mutationFn: async ({ folderId, userId }: { folderId: string; userId: string }) =>
        unwrap(await api.DELETE("/v1/folders/{folderId}/shares/{userId}", { params: { path: { folderId, userId } } })),
      onSuccess: () => { refresh(); },
    }),
    add: useMutation({
      mutationFn: async ({ folderId, itemIds }: { folderId: string; itemIds: string[] }) =>
        unwrap(await api.POST("/v1/folders/{folderId}/items", { params: { path: { folderId } }, body: { itemIds } })),
      onSuccess: () => { refresh(true); },
    }),
    take: useMutation({
      mutationFn: async ({ folderId, itemIds }: { folderId: string; itemIds: string[] }) =>
        unwrap(await api.POST("/v1/folders/{folderId}/items/remove", { params: { path: { folderId } }, body: { itemIds } })),
      onSuccess: () => { refresh(true); },
    }),
  };
}

/** Which of the person's folders hold each record: record id → folder ids. */
export function useMemberships(kind: FolderKind, ids: string[], enabled = true) {
  const sorted = [...new Set(ids)].sort();
  return useQuery({
    queryKey: [...foldersKey, "memberships", kind, sorted],
    enabled: enabled && sorted.length > 0,
    queryFn: async () => {
      const out = new Map<string, string[]>();
      for (let i = 0; i < sorted.length; i += MAX_AT_ONCE) { // the most one call takes
        const got = unwrap(await api.POST("/v1/folders/memberships", { body: { kind, itemIds: sorted.slice(i, i + MAX_AT_ONCE) } }));
        for (const { folderId, itemIds } of got.items) {
          for (const id of itemIds) out.set(id, [...(out.get(id) ?? []), folderId]);
        }
      }
      return out;
    },
    placeholderData: (before) => before, // no flicker while more rows load
  });
}

// Dragging rows onto a folder carries their ids.
const DRAG_TYPE = "application/x-hiringplug-records";

/** Lets a row (or the chosen rows, when it is one of them) be dragged onto a folder. */
export function dragRows(ids: () => string[]): Pick<React.HTMLAttributes<HTMLElement>, "draggable" | "onDragStart"> {
  return {
    draggable: true,
    onDragStart: (e) => {
      e.dataTransfer.setData(DRAG_TYPE, JSON.stringify(ids()));
      e.dataTransfer.effectAllowed = "copy";
    },
  };
}

export const carriesRows = (e: React.DragEvent) => e.dataTransfer.types.includes(DRAG_TYPE);

export function droppedRows(e: React.DragEvent): string[] {
  try {
    const ids: unknown = JSON.parse(e.dataTransfer.getData(DRAG_TYPE));
    return Array.isArray(ids) ? ids.filter((x): x is string => typeof x === "string") : [];
  } catch {
    return [];
  }
}

// The folder last opened on each list is kept on this device (a convenience only).
const OPEN_KEY = "hiringplug.folder.";
export function readOpenFolder(kind: FolderKind): string | null {
  try { return window.localStorage.getItem(OPEN_KEY + kind); } catch { return null; }
}
export function keepOpenFolder(kind: FolderKind, id: string | null) {
  try {
    if (id) window.localStorage.setItem(OPEN_KEY + kind, id);
    else window.localStorage.removeItem(OPEN_KEY + kind);
  } catch { /* only a convenience */ }
}
