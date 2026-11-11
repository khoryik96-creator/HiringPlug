// The message box with its toolbar: bold, italic, underline, strike, lists,
// indent, alignment, link, clear format, font, size, colour, insert a field
// ([Expected salary]) or a banner. CV Studio's email client, with indent.
import { useEffect, useRef } from "react";

import type { Schemas } from "../../api/client";
import { exec, fillTypedFields, insertHtml, insertValue, restoreRange, saveRange, setFontSize, showBanners } from "./richText";

const NONE: Record<string, string> = {};
const FONTS = ["Calibri", "Arial", "Georgia", "Times New Roman", "Verdana", "Tahoma"];
const SIZES = ["10", "11", "12", "14", "16", "18", "24"];

type Props = {
  invalid?: boolean;
  /** What the box starts with; changing `resetKey` puts it back in. */
  html: string;
  resetKey: number;
  onChange: (html: string) => void;
  fields: Schemas["EmailField"][];
  banners: Schemas["EmailBanner"][];
  bannerUrls: Record<string, string>;
  onUploadBanner?: (file: File) => void;
  /**
   * The candidate's value for each field, by token. A field is put in as its
   * value (highlighted, editable; the candidate's record is not changed);
   * a field without one stays as [Field] and is reported when sending.
   */
  values?: Record<string, string>;
  /** Something to put in where the cursor was (a booking link); a new id puts it in. */
  inserting?: { id: number; html: string; managed?: boolean; removeIds?:string[];order?:string[] } | undefined;
  /**
   * A field picked while the cursor was last outside the message (in the
   * subject): put it there and say true; false puts it in the message.
   */
  insertElsewhere?: (field: Schemas["EmailField"], value: string | undefined) => boolean;
  /** The message got the focus (so fields go in it again). */
  onFocus?: () => void;
};

export function RichEditor({ invalid = false, html, resetKey, onChange, fields, banners, bannerUrls, onUploadBanner, values, inserting, insertElsewhere, onFocus }: Props) {
  const vals = values ?? NONE;
  const box = useRef<HTMLDivElement>(null);
  const cursor = useRef<Range | null>(null); // where it was before a toolbar dropdown took the focus

  useEffect(() => {
    if (!box.current) return;
    box.current.innerHTML = html;
    showBanners(box.current, bannerUrls);
    // Only when the message is replaced (a template), not on every change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey]);
  useEffect(() => { showBanners(box.current, bannerUrls); }, [bannerUrls]);
  // Wherever the cursor goes in the message (a click, the keyboard, a field
  // just put in), it is remembered, so the next field goes exactly there.
  useEffect(() => {
    const track = () => { if (box.current) cursor.current = saveRange(box.current) ?? cursor.current; };
    document.addEventListener("selectionchange", track);
    return () => { document.removeEventListener("selectionchange", track); };
  }, []);
  // A template's fields (and any typed before the values came) show their values.
  useEffect(() => {
    if (box.current && fillTypedFields(box.current, fields, vals)) onChange(box.current.innerHTML);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey, vals, fields]);

  const remember = () => { if (box.current) cursor.current = saveRange(box.current) ?? cursor.current; };
  const changed = () => { remember(); if (box.current) onChange(box.current.innerHTML); };
  const typed = () => {
    box.current?.querySelectorAll<HTMLElement>("[data-missing-detail]").forEach((node)=>{
      const text=node.textContent.trim();
      if(text && text!=="—" && !text.includes("(unit not specified)")){delete node.dataset.missingDetail;node.removeAttribute("title");}
    });
    if (box.current) fillTypedFields(box.current, fields, vals); // "[Expected salary]" typed in full becomes its value
    changed();
  };
  const back = () => { if (box.current) restoreRange(box.current, cursor.current); };
  const run = (command: string, value?: string) => {
    back();
    exec(command, value);
    changed();
  };
  const insert = (fragment: string) => {
    if (!box.current) return;
    back();
    insertHtml(box.current, fragment);
    showBanners(box.current, bannerUrls);
    changed();
  };
  useEffect(() => {
    if (!inserting || !box.current) return;
    if (inserting.removeIds || inserting.order) {
      for(const section of box.current.querySelectorAll('[data-candidate-summary="true"]')) {
        for(const row of section.querySelectorAll('[data-candidate-id]')) if(inserting.removeIds?.includes(row.getAttribute("data-candidate-id") ?? "")) row.remove();
        const rows=[...section.querySelectorAll<HTMLElement>('[data-candidate-id]')];
        if(rows.length===0){section.remove();continue;}
        const parents=[...new Set(rows.map((r)=>r.parentElement))];
        for(const parent of parents) for(const id of inserting.order ?? []) {const row=rows.find((r)=>r.parentElement===parent && r.dataset.candidateId===id);if(row)parent?.appendChild(row);}
        rows.forEach((r)=>{const at=inserting.order?.indexOf(r.dataset.candidateId ?? "") ?? -1;const title=r.tagName==="P"?r.querySelector("b"):null;const first=title?.firstChild;if(at>=0 && first?.nodeType===Node.TEXT_NODE)first.textContent=first.textContent?.replace(/^\d+\./,`${String(at+1)}.`) ?? "";});
      }
      cursor.current=null;changed();return;
    }
    if (inserting.managed) {
      const nodes=[...box.current.querySelectorAll('[data-candidate-summary="true"]')];
      if (nodes.length>0) {
        const first=nodes.shift();if(first) first.outerHTML=inserting.html;
        nodes.forEach((n)=>{n.remove();});cursor.current=null;changed();return;
      }
      if (!inserting.html) return;
      // Insert a standalone block. Browser insertHTML can merge its first row
      // into the introduction, which would delete that writing with the candidate.
      box.current.insertAdjacentHTML("beforeend", inserting.html);
      cursor.current=null;changed();return;
    }
    insert(inserting.html);
    // Once per request, not when insert is made again.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [inserting?.id]);
  // Toolbar buttons keep the cursor where it is in the message.
  const keep = (e: React.MouseEvent) => { e.preventDefault(); };
  const button = (label: string, command: string, text = label) => (
    <button type="button" className="tool" aria-label={label} onMouseDown={keep} onClick={() => { run(command); }}>{text}</button>
  );

  return (
    <div className="rich">
      <div className="toolbar" role="toolbar" aria-label="Formatting">
        {button("Bold", "bold", "B")}
        {button("Italic", "italic", "I")}
        {button("Underline", "underline", "U")}
        {button("Strike", "strikeThrough", "Strike")}
        {button("Bullets", "insertUnorderedList")}
        {button("Numbered list", "insertOrderedList")}
        {button("Indent", "indent", "Indent →")}
        {button("Outdent", "outdent", "← Outdent")}
        {button("Left", "justifyLeft")}
        {button("Center", "justifyCenter")}
        {button("Right", "justifyRight")}
        <button type="button" className="tool" onMouseDown={keep} onClick={() => {
          const url = window.prompt("Link to (https://…)");
          if (url && /^(https?:\/\/|mailto:)/i.test(url)) run("createLink", url);
        }}>Link</button>
        {button("Clear format", "removeFormat")}
        <select aria-label="Font" defaultValue="Calibri" onChange={(e) => { run("fontName", e.target.value); }}>
          {FONTS.map((f) => <option key={f} value={f}>{f}</option>)}
        </select>
        <select aria-label="Size" defaultValue="12" onChange={(e) => { if (box.current) { back(); setFontSize(box.current, e.target.value); changed(); } }}>
          {SIZES.map((s) => <option key={s} value={s}>{s} pt</option>)}
        </select>
        <input type="color" aria-label="Text colour" defaultValue="#0b57a4" onChange={(e) => { run("foreColor", e.target.value); }} />
        <select aria-label="Insert field" value="" onChange={(e) => {
          const field = fields.find((f) => f.token === e.target.value);
          const value = field && vals[field.token];
          if (!field || !box.current) return;
          if (insertElsewhere?.(field, value || undefined)) return;
          if (!value) { insert(field.token); return; } // no value: the field stays, and sending says so
          back();
          insertValue(box.current, field.key, value);
          changed();
        }}>
          <option value="">Insert field…</option>
          {fields.map((f) => <option key={f.key} value={f.token}>{f.label}{f.salary ? " (salary)" : ""}{values && !values[f.token] ? " — no value" : ""}</option>)}
        </select>
        <select aria-label="Insert banner" value="" onChange={(e) => {
          if (e.target.value) insert(`<img data-banner-id="${e.target.value}" alt="banner"><br>`);
        }}>
          <option value="">Insert banner…</option>
          {banners.map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
        </select>
        {onUploadBanner && (
          <label className="tool">
            Upload banner
            <input type="file" accept="image/png,image/jpeg,image/gif" hidden onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) onUploadBanner(f);
              e.target.value = "";
            }} />
          </label>
        )}
      </div>
      <div ref={box} className="rich-box" contentEditable suppressContentEditableWarning role="textbox" aria-multiline="true"
        aria-label="Message" aria-invalid={invalid || undefined} tabIndex={0} onInput={typed} onBlur={changed} onKeyUp={remember} onMouseUp={remember} onFocus={onFocus} />
    </div>
  );
}
