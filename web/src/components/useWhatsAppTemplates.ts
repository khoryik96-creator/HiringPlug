// The company's WhatsApp templates and the changes to them, shared by
// Settings › WhatsApp templates and the WhatsApp window. After any change the
// lists (and the Template dropdown) load again.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../api/client";
import { whatsAppDraftsKey, whatsAppTemplatesKey } from "../api/keys";

type Template = Schemas["WhatsAppTemplate"];
export type TemplateValues = { name: string; body: string };

export function useWhatsAppTemplates() {
  return useQuery({ queryKey: whatsAppTemplatesKey, queryFn: async () => unwrap(await api.GET("/v1/whatsapp-templates")) });
}

export function useWhatsAppTemplateChanges() {
  const cache = useQueryClient();
  const reload = () => cache.invalidateQueries({ queryKey: whatsAppTemplatesKey });
  const create = useMutation({
    mutationFn: async (v: TemplateValues) => unwrap(await api.POST("/v1/whatsapp-templates", { body: v })),
    onSuccess: reload,
  });
  const update = useMutation({
    mutationFn: async ({ t, v }: { t: Template; v: TemplateValues }) =>
      unwrap(await api.PUT("/v1/whatsapp-templates/{templateId}", { params: { path: { templateId: t.id } }, body: { version: t.version, ...v } })),
    // A message filled from the changed template (and not typed over) is filled again.
    onSuccess: async () => { await Promise.all([reload(), cache.invalidateQueries({ queryKey: whatsAppDraftsKey })]); },
  });
  const remove = useMutation({
    mutationFn: async (t: Template) => { unwrap(await api.DELETE("/v1/whatsapp-templates/{templateId}", { params: { path: { templateId: t.id } } })); },
    onSuccess: reload,
  });
  return { create, update, remove };
}
