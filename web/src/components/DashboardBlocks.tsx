// The dashboard's blocks below search, in the order each person dragged
// them into (saved with their own settings). A block's handle drags it, or
// moves it with the arrow keys.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { preferencesKey } from "./candidateColumns";

export type BlockId = Schemas["DashboardBlock"];
export interface Block { id: BlockId; label: string; node: ReactNode }

/** The blocks in a saved order; blocks it does not name keep their default place after it. */
export function arrange(blocks: Block[], saved: BlockId[] | null | undefined): Block[] {
  if (!saved) return blocks;
  const byId = new Map(blocks.map((b) => [b.id, b]));
  const placed = saved.flatMap((id) => byId.get(id) ?? []);
  return [...placed, ...blocks.filter((b) => !saved.includes(b.id))];
}

/** order with the block moved to index to. */
export function moved(order: BlockId[], id: BlockId, to: number): BlockId[] {
  const rest = order.filter((x) => x !== id);
  rest.splice(Math.max(0, Math.min(to, rest.length)), 0, id);
  return rest;
}

export function DashboardBlocks({ blocks }: { blocks: Block[] }) {
  const queryClient = useQueryClient();
  const prefs = useQuery({ queryKey: preferencesKey, queryFn: async () => unwrap(await api.GET("/v1/me/preferences")), staleTime: 60_000 });
  const save = useMutation({
    scope: { id: "dashboard-blocks" }, // the last order is the one kept
    mutationFn: async (dashboardBlocks: BlockId[] | null) => unwrap(await api.PUT("/v1/me/preferences", { body: { dashboardBlocks } })),
    onMutate: async (dashboardBlocks) => {
      await queryClient.cancelQueries({ queryKey: preferencesKey });
      queryClient.setQueryData(preferencesKey, (old: Schemas["Preferences"] | undefined) => ({ ...old, dashboardBlocks }));
    },
  });
  const saved = prefs.data?.dashboardBlocks;
  const shown = arrange(blocks, saved);
  const order = shown.map((b) => b.id);
  const [dragging, setDragging] = useState<BlockId | null>(null);
  const [over, setOver] = useState<BlockId | null>(null);
  const put = (next: BlockId[]) => { if (next.join() !== order.join()) save.mutate(next); };

  return (
    <div className="dashboard-blocks">
      {saved && (
        <button type="button" className="link small reset-order" onClick={() => { save.mutate(null); }}>Reset order</button>
      )}
      {shown.map((b, i) => (
        <div key={b.id} className={`dash-block${dragging === b.id ? " dragging" : ""}${over === b.id && dragging !== b.id ? " drop-here" : ""}`}
          onDragOver={(e) => { if (dragging) { e.preventDefault(); setOver(b.id); } }}
          onDragLeave={() => { setOver((o) => (o === b.id ? null : o)); }}
          onDrop={(e) => { e.preventDefault(); if (dragging) put(moved(order, dragging, i)); setDragging(null); setOver(null); }}>
          {/* Not a <button>: browsers do not drag buttons. Dragging shows the whole block. */}
          <span role="button" tabIndex={0} className="drag-handle" draggable aria-label={`Move ${b.label} (arrow keys)`} title="Drag to move"
            onDragStart={(e) => {
              e.dataTransfer.effectAllowed = "move";
              e.dataTransfer.setData("text/plain", b.id);
              const block = e.currentTarget.parentElement;
              if (block) e.dataTransfer.setDragImage(block, 24, 24);
              setDragging(b.id);
            }}
            onDragEnd={() => { setDragging(null); setOver(null); }}
            onKeyDown={(e) => {
              if (e.key === "ArrowUp" && i > 0) { e.preventDefault(); put(moved(order, b.id, i - 1)); }
              if (e.key === "ArrowDown" && i < shown.length - 1) { e.preventDefault(); put(moved(order, b.id, i + 1)); }
            }}>
            <svg viewBox="0 0 24 24" width="14" height="14" aria-hidden="true">
              {[6, 12, 18].flatMap((y) => [9, 15].map((x) => <circle key={`${String(x)}-${String(y)}`} cx={x} cy={y} r="1.6" fill="currentColor" />))}
            </svg>
          </span>
          {b.node}
        </div>
      ))}
    </div>
  );
}
