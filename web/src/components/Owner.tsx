import { useMutation, useQuery } from "@tanstack/react-query";

import { api, unwrap } from "../api/client";
import { usersKey } from "../api/keys";
import { ErrorMessage } from "./ErrorMessage";
import { Initials } from "./Initials";

/**
 * Who a candidate or a job belongs to, and (with permission) handing it to
 * someone else. Roles that see only their own candidates see those they own.
 */
export function Owner({ what, ownerId, ownerName, canChange, save, chip = false }: {
  what: string;
  /** A small chip with the owner's initials, no "Owner:" (a page header's line). */
  chip?: boolean;
  ownerId: string | null;
  ownerName: string | null;
  canChange: boolean;
  save: (ownerId: string | null) => Promise<unknown>;
}) {
  const users = useQuery({ queryKey: usersKey, queryFn: async () => unwrap(await api.GET("/v1/users")).items, staleTime: 60_000, enabled: canChange });
  const change = useMutation({ mutationFn: save });
  return (
    <span className={chip ? "owner owner-chip" : "owner"} title={chip ? "Owner" : undefined}>
      {chip ? <Initials name={ownerName ?? "Nobody"} /> : "Owner: "}
      {canChange ? (
        <select aria-label={`Owner of this ${what}`} value={ownerId ?? ""} disabled={change.isPending}
          onChange={(e) => { change.mutate(e.target.value || null); }}>
          <option value="">Nobody</option>
          {ownerId && !users.data?.some((u) => u.id === ownerId) && <option value={ownerId}>{ownerName ?? "Someone who left"}</option>}
          {users.data?.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
        </select>
      ) : <strong>{ownerName ?? "Nobody"}</strong>}
      <ErrorMessage error={change.error ?? users.error} />
    </span>
  );
}
