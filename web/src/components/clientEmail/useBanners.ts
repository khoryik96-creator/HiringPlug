// The company's email banners, with their images loaded (with the user's
// token) as blob URLs the editor can show.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { api, unwrap } from "../../api/client";
import { emailBannersKey } from "../../api/keys";
import { getToken } from "../../auth";

export function useBanners() {
  const queryClient = useQueryClient();
  const list = useQuery({ queryKey: emailBannersKey, queryFn: async () => unwrap(await api.GET("/v1/email-banners")).items });
  const [urls, setUrls] = useState<Record<string, string>>({});

  useEffect(() => {
    let cancelled = false;
    for (const b of list.data ?? []) {
      if (urls[b.id]) continue;
      void fetch(`/v1/email-banners/${b.id}/file`, { headers: { Authorization: `Bearer ${getToken() ?? ""}` } })
        .then(async (res) => (res.ok ? new Blob([await res.arrayBuffer()], { type: b.contentType }) : null))
        .then((blob) => {
          if (blob && !cancelled) setUrls((u) => ({ ...u, [b.id]: URL.createObjectURL(blob) }));
        });
    }
    return () => { cancelled = true; };
  }, [list.data, urls]);

  const upload = useMutation({
    mutationFn: async (file: File) => {
      const body = new FormData();
      body.append("file", file);
      const res = await fetch("/v1/email-banners", { method: "POST", body, headers: { Authorization: `Bearer ${getToken() ?? ""}` } });
      const data: unknown = await res.json();
      if (!res.ok) unwrap({ error: data, response: res });
      return data as { id: string };
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: emailBannersKey }),
  });

  return { banners: list.data ?? [], urls, upload };
}
