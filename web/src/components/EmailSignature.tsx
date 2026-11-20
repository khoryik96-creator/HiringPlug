// Settings > Your account: the person's email signature (formatted, with a
// banner if they like), added to the emails they write in Outlook Mail.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { RichEditor } from "./clientEmail/RichEditor";
import { useBanners } from "./clientEmail/useBanners";
import { ErrorMessage } from "./ErrorMessage";
import { emailSignatureKey } from "./outlook/OutlookComposeTools";

export function EmailSignature({ canUploadBanners = false }: { canUploadBanners?: boolean }) {
  const client = useQueryClient();
  const saved = useQuery({ queryKey: emailSignatureKey, queryFn: async () => unwrap(await api.GET("/v1/email-signature")) });
  return (
    <section className="panel email-signature" aria-label="Email signature">
      <h3>Email signature</h3>
      <p className="muted small">Added to the emails you write in Outlook Mail: new messages, replies and forwards, above the quoted message. Delete it from any one email as you write it.</p>
      <ErrorMessage error={saved.error} />
      {saved.data && <SignatureForm key={saved.data.version} saved={saved.data} canUploadBanners={canUploadBanners}
        onSaved={(s) => { client.setQueryData(emailSignatureKey, s); }} />}
    </section>
  );
}

function SignatureForm({ saved, canUploadBanners, onSaved }: { saved: Schemas["EmailSignature"]; canUploadBanners: boolean; onSaved: (s: Schemas["EmailSignature"]) => void }) {
  const { banners, urls, upload } = useBanners();
  const [html, setHtml] = useState(saved.bodyHtml);
  const save = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/v1/email-signature", { body: { bodyHtml: html, version: saved.version } })),
    onSuccess: onSaved,
  });
  return (
    <>
      <RichEditor compact html={saved.bodyHtml} resetKey={saved.version} onChange={setHtml} banners={banners} bannerUrls={urls}
        onUploadBanner={canUploadBanners ? (f) => { upload.mutate(f); } : undefined} />
      <div className="row">
        <button type="button" className="primary" disabled={save.isPending || html === saved.bodyHtml} onClick={() => { save.mutate(); }}>Save signature</button>
        {save.isSuccess && <span className="muted small" role="status">Saved.</span>}
      </div>
      <ErrorMessage error={save.error ?? upload.error} />
    </>
  );
}
