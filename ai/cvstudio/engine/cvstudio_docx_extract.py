"""Exact selected DOCX/outline helpers with a minimal safety adapter.

The original app wrappers are replaced only by the pure ZIP validator.
PDF OCR and Windows .doc conversion are reference adapters, not provided here.
"""
import re
import html
from cvstudio_blind_mask import _BLIND_SUMMARY_MARKER_RE
from cvstudio_document_safety import validate_zip_payload as _validate_zip_payload
from cvstudio_cv_normalize import _CV_LEADING_BULLET_MARKER_RE

# Copied from app.py:12333-12346
def _cv_bullet_match_key(text):
    """Normalize a bullet for source<->parsed matching, mirrored in generate.js.

    Strips the same leading marker/enumerator the formatter strips for display
    (_CV_LEADING_BULLET_MARKER_RE) and lowercases, so a level looked up from the
    raw source bullet matches the rendered (already-stripped) bullet text.
    """
    value = re.sub(r"\s+", " ", str(text or "")).strip()
    for _ in range(5):
        stripped = _CV_LEADING_BULLET_MARKER_RE.sub("", value, count=1).strip()
        if stripped == value:
            break
        value = stripped
    return value.lower()


# Copied from app.py:12349-12349
_ROMAN_RE = re.compile(r"^[ivxlcdm]+$", re.I)


# Copied from app.py:12350-12350
_LABEL_PAREN_RE = re.compile(r"^\((?P<body>[ivxlcdm]{1,7}|[a-z]|\d{1,3})\)", re.I)


# Copied from app.py:12351-12353
_LABEL_NUM_RE = re.compile(
    r"^(?P<body>\d{1,3})(?:[.)](?=\s|[^\W\d_])|-(?=\s))"
)


# Copied from app.py:12354-12356
_LABEL_BARE_RE = re.compile(
    r"^(?P<body>(?:[a-z]|[ivxlcdm]{2,7}))(?P<punct>[.)-])(?=\s)"
)


# Copied from app.py:12359-12364
def _label_body_kind(body):
    if body.isdigit():
        return "digit"
    if len(body) > 1 and _ROMAN_RE.match(body):
        return "roman"
    return "alpha"


# Copied from app.py:12367-12385
def _bullet_label_style(text):
    """Classify a leading outline label into a style key, or None.

    Restricted to the label forms the formatter actually strips
    (_CV_LEADING_BULLET_MARKER_RE). Keeping classification and stripping in
    lockstep means a nested item's label is both removed AND counted -- never one
    without the other. A plain glyph or unlabeled line returns None (base level).
    """
    s = str(text or "").lstrip()
    m = _LABEL_PAREN_RE.match(s)
    if m:
        return ("paren", _label_body_kind(m.group("body")))
    m = _LABEL_NUM_RE.match(s)
    if m:
        return ("num", "digit")
    m = _LABEL_BARE_RE.match(s)
    if m:
        return ("bare", _label_body_kind(m.group("body")))
    return None


# Copied from app.py:12388-12410
def _infer_outline_levels(texts):
    """Infer a nesting level per bullet from the sequence of outline labels.

    Appearance-order stack: an unlabeled line is level 0 and resets the outline;
    the first labelled style below it is level 1; a new deeper style nests one
    level further; returning to a seen style pops back to that level. Mirrors
    cvInferLevelsForSequence in index.html.
    """
    levels = []
    stack = []
    for text in texts:
        style = _bullet_label_style(text)
        if style is None:
            stack = []
            levels.append(0)
        elif style in stack:
            idx = stack.index(style)
            del stack[idx + 1:]
            levels.append(idx + 1)
        else:
            stack.append(style)
            levels.append(len(stack))
    return levels


# Copied from app.py:12413-12439
def _infer_label_bullet_levels(cv_data):
    """Outline-label nesting levels for a parsed CV's role bullets.

    Runs _infer_outline_levels over each role's bullet sequence (raw text, before
    labels are stripped) and returns [{text: match_key, level}] for levels >= 1.
    """
    out = []
    for exp in (cv_data or {}).get("work_experiences") or []:
        if not isinstance(exp, dict):
            continue
        for role in exp.get("roles") or []:
            if not isinstance(role, dict):
                continue
            texts = []
            for item in role.get("bullets") or []:
                if isinstance(item, str):
                    texts.append(item)
                elif isinstance(item, dict):
                    for sub in item.get("bullets") or []:
                        if isinstance(sub, str):
                            texts.append(sub)
            for text, level in zip(texts, _infer_outline_levels(texts)):
                if level >= 1:
                    key = _cv_bullet_match_key(text)
                    if key:
                        out.append({"text": key, "level": min(level, 8)})
    return out


# Copied from app.py:12442-12459
def _merge_bullet_level_lists(*lists):
    """Merge {text, level} lists by key, keeping the deepest level (>= 1)."""
    best = {}
    for entries in lists:
        for entry in entries or []:
            if not isinstance(entry, dict):
                continue
            key = str(entry.get("text") or "")
            if not key:
                continue
            try:
                level = int(entry.get("level") or 0)
            except (TypeError, ValueError):
                continue
            level = max(0, min(level, 8))
            if key not in best or level > best[key]:
                best[key] = level
    return [{"text": k, "level": v} for k, v in best.items() if v >= 1]


# Copied from app.py:12462-12498
def _extract_docx_bullet_levels(file_bytes):
    """Map nested (w:ilvl >= 1) list paragraphs to their indent level.

    Word stores list nesting in w:numPr/w:ilvl, which the plain-text extraction
    discards. This side-channel lets the renderer restore the indent
    deterministically by matching parsed bullet text back to its source level.
    Only levels >= 1 are recorded; every unmatched bullet renders at level 0.
    """
    import io as _io
    import docx as _docx
    from docx.oxml.ns import qn
    out = []
    seen = set()
    try:
        doc = _docx.Document(_io.BytesIO(file_bytes))
    except Exception:
        return out
    for p in doc.element.body.iter(qn("w:p")):
        ilvl_vals = p.xpath("./w:pPr/w:numPr/w:ilvl/@w:val")
        if not ilvl_vals:
            continue
        try:
            level = int(ilvl_vals[0])
        except (TypeError, ValueError):
            continue
        if level < 1:
            continue
        text = "".join(node.text or "" for node in p.iter(qn("w:t"))).strip()
        key = _cv_bullet_match_key(text)
        if not key:
            continue
        dedup = (key, level)
        if dedup in seen:
            continue
        seen.add(dedup)
        out.append({"text": key, "level": min(level, 8)})
    return out


# Copied from app.py:12583-12635
def _extract_docx_textbox_lines(document_xml):
    """Return textbox paragraphs without flattening Word list boundaries.

    CV Studio's summary and its ABOUT HIM / HER label are separate drawing
    shapes. Word stores the content shape before the label shape and duplicates
    both through AlternateContent fallbacks. Keep each ``w:p`` as one line,
    discard only exact duplicate fallback groups, and move the label immediately
    before its numbered summary group so the CV parser sees a real section.
    """
    xml = str(document_xml or "")
    if not xml:
        return []
    groups = []
    seen = set()
    for match in re.finditer(
        r"<w:txbxContent\b[^>]*>(.*?)</w:txbxContent>",
        xml,
        re.S,
    ):
        lines = []
        numbered = False
        for paragraph in re.findall(
            r"<w:p(?=[ />])[^>]*>.*?</w:p>",
            match.group(1),
            re.S,
        ):
            value = _summary_docx_visible_text(paragraph)
            if not value:
                continue
            is_numbered = bool(re.search(r"<w:numPr\b|<w:numId\b", paragraph))
            numbered = numbered or is_numbered
            if is_numbered and not _BLIND_SUMMARY_MARKER_RE.match(value):
                value = "• " + value
            lines.append(value)
        key = tuple(lines)
        if lines and key not in seen:
            seen.add(key)
            groups.append({"lines": lines, "numbered": numbered})

    ordered = []
    about_keys = {"abouthimher", "abouthim", "abouther", "aboutthecandidate"}
    for group in groups:
        key = (
            re.sub(r"[^a-z]+", "", group["lines"][0].casefold())
            if len(group["lines"]) == 1
            else ""
        )
        if key in about_keys and ordered and ordered[-1]["numbered"]:
            summary_group = ordered.pop()
            ordered.extend((group, summary_group))
        else:
            ordered.append(group)
    return [line for group in ordered for line in group["lines"]]


# Copied from app.py:12638-12659
def _docx_paragraph_numbering_id(paragraph):
    """Return the live list id, respecting direct zero and inherited styles.

    ``Paragraph.text`` deliberately omits Word's numbering glyph.  Preserve
    that structural signal for the CV parser by checking direct numbering
    and any numbering inherited through the paragraph's style chain.
    """
    try:
        direct = paragraph._p.xpath("./w:pPr/w:numPr/w:numId/@w:val")
        if direct:
            return next((str(value) for value in direct if str(value) != "0"), None)
        style = paragraph.style
        seen = set()
        while style is not None and style.style_id not in seen:
            seen.add(style.style_id)
            inherited = style._element.xpath("./w:pPr/w:numPr/w:numId/@w:val")
            if inherited:
                return next((str(value) for value in inherited if str(value) != "0"), None)
            style = style.base_style
    except Exception:
        return None
    return None


# Copied from app.py:12662-12663
def _docx_paragraph_has_numbering(paragraph):
    return _docx_paragraph_numbering_id(paragraph) is not None


# Copied from app.py:12666-12702
def _docx_cell_content_lines(cell, render_paragraph, depth=0):
    """Read nested layout tables in place, including their anchored textboxes."""
    if depth > 32:
        raise ValueError('The DOCX table layout is too deeply nested. Export it as a simpler DOCX or PDF.')
    from docx.table import Table
    from docx.text.paragraph import Paragraph
    out = []
    def blocks(parent, level):
        if level > 32:
            raise ValueError('The DOCX table layout is too deeply nested. Export it as a simpler DOCX or PDF.')
        for child in parent.iterchildren():
            if child.tag.rsplit('}', 1)[-1] in {'sdt', 'sdtContent', 'customXml', 'ins', 'moveTo'}:
                yield from blocks(child, level+1)
            else:
                yield child, level
    for child, level in blocks(cell._tc, depth):
        tag = child.tag.rsplit('}', 1)[-1]
        if tag == 'p':
            value = render_paragraph(Paragraph(child, cell)).strip()
            if value:
                out.append(value)
            out.extend(_extract_docx_textbox_lines(child.xml))
        elif tag == 'tbl':
            for row in Table(child, cell).rows:
                groups, seen = [], set()
                for nested in row.cells:
                    if nested._tc in seen:
                        continue
                    seen.add(nested._tc)
                    values = _docx_cell_content_lines(nested, render_paragraph, level+1)
                    if values:
                        groups.append(values)
                if groups and all(len(group) == 1 for group in groups):
                    out.append(' | '.join(group[0] for group in groups))
                else:
                    out.extend(value for group in groups for value in group)
    return out


# Copied from app.py:12705-12792
def _extract_docx_text_preserve_tables(file_bytes):
    _validate_zip_payload(file_bytes, "DOCX")
    """Extract DOCX text, preserving table-cell paragraph/bullet boundaries.

    Corporate mandates and many CVs place multiple responsibilities inside a
    single table cell.  Joining those paragraphs with spaces destroys bullet
    boundaries and lets the JD term extractor keep only the first few concepts.
    Rows containing only one paragraph per cell remain compact `` | `` lines;
    multi-paragraph cells are emitted as separate lines in document order.
    """
    import io as _io
    import docx as _docx
    from docx.table import Table as _DocxTable
    from docx.text.paragraph import Paragraph as _DocxParagraph
    doc = _docx.Document(_io.BytesIO(file_bytes))
    lines = []

    def clean(text):
        return re.sub(r"\s+", " ", (text or "")).strip()
    def paragraph_line(paragraph):
        value = clean(paragraph.text)
        if not value:
            return ""
        if _docx_paragraph_has_numbering(paragraph) and not re.match(
            r"^[•●▪◦‣⁃∙·‧]\s*", value
        ):
            return "• " + value
        return value

    def cell_lines(cell):
        out = []
        for value in _docx_cell_content_lines(cell, paragraph_line):
            if value and (not out or value != out[-1]):
                out.append(value)
        return out

    def row_lines(row):
        groups = []
        seen_groups = set()
        for cell in row.cells:
            group = cell_lines(cell)
            key = tuple(group)
            # Merged cells are repeated by python-docx; keep the first copy only.
            if group and key not in seen_groups:
                seen_groups.add(key)
                groups.append(group)
        if not groups:
            return []
        if all(len(group) == 1 for group in groups):
            return [" | ".join(group[0] for group in groups)]
        out = []
        for group in groups:
            for value in group:
                if value and (not out or value != out[-1]):
                    out.append(value)
        return out

    seen_headers = set()
    def append_header(value):
        value = clean(value)
        if value and value not in seen_headers:
            seen_headers.add(value)
            lines.append(value)

    for section in doc.sections:
        for hdr in (section.header, section.first_page_header, section.even_page_header):
            try:
                for paragraph in hdr.paragraphs:
                    append_header(paragraph_line(paragraph))
                for table in getattr(hdr, "tables", []) or []:
                    for row in table.rows:
                        for value in row_lines(row):
                            append_header(value)
            except Exception:
                pass

    body = doc.element.body
    for child in body.iterchildren():
        tag = child.tag.rsplit('}', 1)[-1]
        if tag == 'p':
            value = paragraph_line(_DocxParagraph(child, doc))
            if value:
                lines.append(value)
        elif tag == 'tbl':
            table = _DocxTable(child, doc)
            for row in table.rows:
                lines.extend(row_lines(row))
    return "\n".join(lines)


# Copied from app.py:12794-12879
def _normalize_extracted_text_artifacts(text):
    """Clean common PDF/OCR font-encoding artefacts from extracted JD/CV text.

    Some PDFs embed fonts where common ligatures are exposed by pdfplumber as
    placeholders such as ``(cid:415)`` instead of real text. Replacing them with
    spaces gives broken JD text like ``Accoun ng`` / ``Loca on``. Keep this
    cleanup conservative and focused on known extraction artefacts so genuine
    wording and tech stack terms are preserved.
    """
    if not text:
        return text
    import re as _re
    t = str(text)

    # Known glyph-code mappings seen in PDFs generated from Word/Canva-like
    # layouts. These map to common ligature fragments, not company names.
    _cid_map = {
        '332': 'ft',   # so(cid:332)ware -> software, shi(cid:332) -> shift
        '414': 'tf',   # pla(cid:414)orm -> platform
        '415': 'ti',   # Accoun(cid:415)ng -> Accounting, Loca(cid:415)on -> Location
        '425': 'tt',   # Commi(cid:425)ed -> Committed, wri(cid:425)en -> written
        '427': 'tti',  # a(cid:427)tude -> attitude
    }
    def _cid_repl(m):
        return _cid_map.get(m.group(1), ' ')
    t = _re.sub(r'\(?cid:(\d+)\)?', _cid_repl, t, flags=_re.I)

    # Same artefacts sometimes arrive as odd Unicode glyphs instead of cid tags.
    t = t.translate(str.maketrans({
        '\u019f': 'ti',  # Ɵ
        '\u014c': 'ft',  # Ō
        '\u019e': 'tf',  # ƞ
        '\u01a9': 'tt',  # Ʃ
        '\u01ab': 'tti', # ƫ
        '\uf0b7': '•',   # private-use bullet
        '\ufffe': '-',   # malformed soft-hyphen separator seen as buy￾ins
    }))

    # If a previous extraction already replaced cid tags with spaces, recover
    # the most common JD words. Only exact broken-word patterns are touched.
    _word_fixes = {
        r'\bAccoun\s+ng\b': 'Accounting', r'\baccoun\s+ng\b': 'accounting',
        r'\bLoca\s+on\b': 'Location', r'\bloca\s+on\b': 'location',
        r'\bFull\s+me\b': 'Full time', r'\bfull\s+me\b': 'full time',
        r'\bso\s+ware\b': 'software', r'\bSo\s+ware\b': 'Software',
        r'\bpla\s+orm\b': 'platform', r'\bPla\s+orm\b': 'Platform',
        r'\bwri\s+en\b': 'written', r'\bWri\s+en\b': 'Written',
        r'\bCommi\s+ed\b': 'Committed', r'\bcommi\s+ed\b': 'committed',
        r'\ba\s+tude\b': 'attitude', r'\bA\s+tude\b': 'Attitude',
        r'\bcri\s+cal\b': 'critical', r'\bCri\s+cal\b': 'Critical',
        r'\bpoten\s+al\b': 'potential', r'\bPoten\s+al\b': 'Potential',
        r'\bop\s+mize\b': 'optimize', r'\bOp\s+mize\b': 'Optimize',
        r'\bcollec\s+on\b': 'collection', r'\bCollec\s+on\b': 'Collection',
        r'\bsolu\s+on\b': 'solution', r'\bSolu\s+on\b': 'Solution',
        r'\bsolu\s+ons\b': 'solutions', r'\bSolu\s+ons\b': 'Solutions',
        r'\bopera\s+on\b': 'operation', r'\bOpera\s+on\b': 'Operation',
        r'\bopera\s+ons\b': 'operations', r'\bOpera\s+ons\b': 'Operations',
        r'\bini\s+a\s+ves\b': 'initiatives', r'\bIni\s+a\s+ves\b': 'Initiatives',
        r'\biden\s+fy\b': 'identify', r'\bIden\s+fy\b': 'Identify',
        r'\beffec\s+vely\b': 'effectively', r'\bEffec\s+vely\b': 'Effectively',
        r'\bCollabora\s+ve\b': 'Collaborative', r'\bcollabora\s+ve\b': 'collaborative',
        r'\bac\s+vely\b': 'actively', r'\bAc\s+vely\b': 'Actively',
        r'\bconstruc\s+ve\b': 'constructive', r'\bConstruc\s+ve\b': 'Constructive',
        r'\bcon\s+nuous\b': 'continuous', r'\bCon\s+nuous\b': 'Continuous',
        r'\bsuppor\s+ve\b': 'supportive', r'\bSuppor\s+ve\b': 'Supportive',
        r'\bmo\s+vates\b': 'motivates', r'\bMo\s+vates\b': 'Motivates',
        r'\bCoordina\s+on\b': 'Coordination', r'\bcoordina\s+on\b': 'coordination',
        r'\bme\s+culous\b': 'meticulous', r'\bMe\s+culous\b': 'Meticulous',
        r'\bproac\s+ve\b': 'proactive', r'\bProac\s+ve\b': 'Proactive',
        r'\bmul\s+ple\b': 'multiple', r'\bMul\s+ple\b': 'Multiple',
        r'\bpriori\s+es\b': 'priorities', r'\bPriori\s+es\b': 'Priorities',
        r'\bshi\s+ing\b': 'shifting', r'\bShi\s+ing\b': 'Shifting',
        r'\bnight\s+shi\s+\b': 'night shift ', r'\bNight\s+shi\s+\b': 'Night shift ',
        r'\b\s+me\b': ' time',
        r'\bcke\s+ng\b': 'ticketing', r'\bCke\s+ng\b': 'Ticketing',
        r'\bma\s+ers\b': 'matters', r'\bMa\s+ers\b': 'Matters',
        r'\bsa\s+sfac\s+on\b': 'satisfaction', r'\bSa\s+sfac\s+on\b': 'Satisfaction',
    }
    for _pat, _rep in _word_fixes.items():
        t = _re.sub(_pat, _rep, t)

    # Clean spacing without flattening paragraphs.
    t = _re.sub(r'[ \t]{2,}', ' ', t)
    t = _re.sub(r' *\n *', '\n', t)
    t = _re.sub(r'\n{3,}', '\n\n', t).strip()
    return t



# Copied from app.py:11505-11511
def _summary_docx_visible_text(paragraph_xml):
    values = re.findall(r"<w:t\b[^>]*>(.*?)</w:t>", paragraph_xml, re.S)
    return re.sub(
        r"\s+",
        " ",
        "".join(html.unescape(value) for value in values),
    ).strip()
