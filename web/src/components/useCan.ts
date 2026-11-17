import { useQuery } from "@tanstack/react-query";

import { api, type Permission, unwrap } from "../api/client";

/** Whether the signed-in person has a permission (from /v1/me, loaded with the app). */
export function useCan(permission: Permission): boolean {
  const me = useQuery({ queryKey: ["me"], queryFn: async () => unwrap(await api.GET("/v1/me")), staleTime: 60_000 });
  const perms: unknown = me.data?.permissions; // not loaded yet: no
  return Array.isArray(perms) && perms.includes(permission);
}
