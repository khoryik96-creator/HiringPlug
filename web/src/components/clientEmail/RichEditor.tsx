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
  inserting?: { id: number; html: string } | undefined;
};

export function RichEditor({ html, resetKey, onChange, fields, banners, bannerUrls, onUploadBanner, values, inserting }: Props) {
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
  // A template's fields (and any typed before the values came) show their values.
  useEffect(() => {
    if (box.current && fillTypedFields(box.current, fields, vals)) onChange(box.current.innerHTML);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetKey, vals, fields]);

  const remember = () => { if (box.current) cursor.current = saveRange(box.current) ?? cursor.current; };
  const changed = () => { remember(); if (box.current) onChange(box.current.innerHTML); };
  const typed = () => {
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
    if (inserting) insert(inserting.html);
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
        aria-label="Message" tabIndex={0} onInput={typed} onBlur={changed} onKeyUp={remember} onMouseUp={remember} />
    </div>
  );
}
