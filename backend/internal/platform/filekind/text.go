package filekind

// Reading the plain formats: text files in any common encoding, RTF and
// OpenDocument text. Each is bounded: a damaged or hostile file is refused,
// never allowed to use up memory.

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

// Limits on what is read.
const (
	maxText       = 4 << 20  // characters of text kept
	maxODTContent = 64 << 20 // bytes of content.xml once unzipped
	maxZipEntries = 2000
)

// ErrNoText is a file with no readable text in it.
var ErrNoText = errors.New("no readable text was found in the file")

// Text reads the text of a .txt, .rtf or .odt file (see ReadHere).
func Text(contentType string, data []byte) (string, error) {
	var text string
	var err error
	switch contentType {
	case TXT:
		text, err = plainText(data)
	case RTF:
		text, err = rtfText(data)
	case ODT:
		text, err = odtText(data)
	default:
		return "", fmt.Errorf("%s is not read here", contentType)
	}
	if err != nil {
		return "", err
	}
	if text = clean(text); text == "" {
		return "", ErrNoText
	}
	if r := []rune(text); len(r) > maxText {
		text = string(r[:maxText])
	}
	return text, nil
}

// plainText decodes a text file: UTF-8 (with or without its mark), UTF-16
// with its mark or without (told by its zero bytes), else Windows-1252,
// which Word on Windows saves "plain text" in.
func plainText(data []byte) (string, error) {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		data = data[3:]
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}):
		return checked(utf16Text(data[2:], binary.LittleEndian))
	case bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		return checked(utf16Text(data[2:], binary.BigEndian))
	case len(data) >= 4 && data[1] == 0 && data[3] == 0 && data[0] != 0:
		return checked(utf16Text(data, binary.LittleEndian))
	case len(data) >= 4 && data[0] == 0 && data[2] == 0 && data[1] != 0:
		return checked(utf16Text(data, binary.BigEndian))
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return "", errors.New("this is not a text file")
	}
	if utf8.Valid(data) {
		return checked(string(data))
	}
	return checked(cp1252(data))
}

func checked(s string) (string, error) {
	if !validText(s) {
		return "", errors.New("this is not a text file")
	}
	return s, nil
}

func utf16Text(data []byte, order binary.ByteOrder) string {
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = order.Uint16(data[2*i:])
	}
	return string(utf16.Decode(units))
}

// cp1252High is Windows-1252 from 0x80 to 0x9F (the rest is Latin-1).
var cp1252High = [32]rune{
	'€', '\u0081', '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', '\u008d', 'Ž', '\u008f',
	'\u0090', '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', '\u009d', 'ž', 'Ÿ',
}

func cp1252Rune(b byte) rune {
	if b >= 0x80 && b <= 0x9F {
		return cp1252High[b-0x80]
	}
	return rune(b)
}

func cp1252(data []byte) string {
	var sb strings.Builder
	sb.Grow(len(data))
	for _, b := range data {
		sb.WriteRune(cp1252Rune(b))
	}
	return sb.String()
}

// --- RTF ---

// Destinations whose text is not part of the document (fonts, colours,
// styles, file information, pictures, field codes).
var rtfSkip = map[string]bool{
	"fonttbl": true, "colortbl": true, "stylesheet": true, "listtable": true, "listoverridetable": true,
	"revtbl": true, "rsidtbl": true, "generator": true, "info": true, "pict": true, "object": true,
	"themedata": true, "colorschememapping": true, "datastore": true, "latentstyles": true, "xmlnstbl": true,
	"mmathPr": true, "pgdsctbl": true, "filetbl": true, "fldinst": true, "bkmkstart": true, "bkmkend": true,
	"listtext": false, "nonshppict": true, "shpinst": true, "wgrffmtfilter": true, "passwordhash": true,
}

var rtfChar = map[string]string{
	"par": "\n", "line": "\n", "sect": "\n\n", "page": "\n\n", "row": "\n", "cell": "\t", "tab": "\t",
	"emdash": "—", "endash": "–", "bullet": "•", "lquote": "‘", "rquote": "’", "ldblquote": "“", "rdblquote": "”",
	"emspace": " ", "enspace": " ", "qmspace": " ",
}

type rtfState struct {
	skip bool
	uc   int // characters to skip after a \u
}

// rtfText reads an RTF file's text, following its groups and control words.
// Bytes written as \'hh (or raw) are decoded in the file's code page
// (\ansicpg: Windows-1252 unless it says otherwise, e.g. 936 for Chinese).
func rtfText(data []byte) (string, error) {
	var out strings.Builder
	stack := []rtfState{{uc: 1}}
	state := &stack[0]
	pendingSkip := 0 // fallback characters still to drop after a \u
	depth := 0
	var raw []byte // 8-bit text waiting to be decoded (a DBCS character is two bytes)
	decode := cp1252
	flush := func() {
		if len(raw) > 0 {
			if !state.skip && out.Len() < maxText*4 {
				out.WriteString(decode(raw))
			}
			raw = raw[:0]
		}
	}
	emit := func(s string) {
		flush()
		if !state.skip && out.Len() < maxText*4 {
			out.WriteString(s)
		}
	}
	byteText := func(b byte) {
		if pendingSkip > 0 {
			pendingSkip--
		} else if !state.skip {
			raw = append(raw, b)
		}
	}
	for i := 0; i < len(data); {
		c := data[i]
		switch c {
		case '{':
			flush()
			depth++
			if depth > 10000 {
				return "", errors.New("the RTF file is damaged (groups nested too deep)")
			}
			stack = append(stack, *state)
			state = &stack[len(stack)-1]
			pendingSkip = 0
			i++
		case '}':
			flush()
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
				state = &stack[len(stack)-1]
			}
			depth = max(depth-1, 0)
			pendingSkip = 0
			i++
		case '\\':
			i++
			if i >= len(data) {
				break
			}
			n := data[i]
			switch {
			case n == '\'' && i+2 < len(data):
				if v, err := strconv.ParseUint(string(data[i+1:i+3]), 16, 8); err == nil {
					byteText(byte(v))
				}
				i += 3
			case isLetter(n):
				j := i
				for j < len(data) && isLetter(data[j]) && j-i < 32 {
					j++
				}
				word := string(data[i:j])
				k := j
				if k < len(data) && (data[k] == '-' || isDigit(data[k])) {
					k++
					for k < len(data) && isDigit(data[k]) && k-j < 12 {
						k++
					}
				}
				param, hasParam := 0, k > j
				if hasParam {
					param, _ = strconv.Atoi(string(data[j:k]))
				}
				if k < len(data) && data[k] == ' ' {
					k++ // the space ending a control word is not text
				}
				i = k
				switch {
				case word == "bin" && hasParam: // raw binary (a picture): skipped whole, never read as RTF
					i = min(i+max(param, 0), len(data))
				case word == "ansicpg" && hasParam:
					flush()
					decode = codePage(param)
				case word == "u" && hasParam:
					if param < 0 {
						param += 65536
					}
					emit(string(rune(param)))
					pendingSkip = state.uc
				case word == "uc" && hasParam:
					state.uc = max(param, 0)
				case rtfSkip[word]:
					flush()
					state.skip = true
				default:
					if s, ok := rtfChar[word]; ok {
						emit(s)
					}
				}
			case n == '*': // this group is a destination to skip when not understood
				flush()
				state.skip = true
				i++
			case n == '~':
				emit("\u00a0")
				i++
			case n == '_':
				emit("-")
				i++
			case n == '\n' || n == '\r':
				emit("\n")
				i++
			default: // \\ \{ \} and the rest stand for themselves
				if n != '-' && n != ':' && n != '|' {
					emit(string(rune(n)))
				}
				i++
			}
		case '\r', '\n':
			i++
		default:
			if c >= 0x80 { // raw 8-bit text, in the code page
				byteText(c)
			} else if pendingSkip > 0 {
				pendingSkip--
			} else {
				emit(string(rune(c)))
			}
			i++
		}
	}
	flush()
	if out.Len() == 0 {
		return "", ErrNoText
	}
	return strings.ReplaceAll(out.String(), "\u00a0", " "), nil
}

// codePage decodes text in a Windows code page RTF names (\ansicpg).
func codePage(n int) func([]byte) string {
	var enc encoding.Encoding
	switch n {
	case 936:
		enc = simplifiedchinese.GBK
	case 950:
		enc = traditionalchinese.Big5
	case 932:
		enc = japanese.ShiftJIS
	case 949:
		enc = korean.EUCKR
	case 874:
		enc = charmap.Windows874
	case 1250, 1251, 1253, 1254, 1255, 1256, 1257, 1258:
		enc = []encoding.Encoding{charmap.Windows1250, charmap.Windows1251, nil, charmap.Windows1253, charmap.Windows1254,
			charmap.Windows1255, charmap.Windows1256, charmap.Windows1257, charmap.Windows1258}[n-1250]
	default:
		return cp1252
	}
	return func(b []byte) string {
		s, err := enc.NewDecoder().Bytes(b)
		if err != nil {
			return cp1252(b)
		}
		return string(s)
	}
}

func isLetter(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }
func isDigit(b byte) bool  { return b >= '0' && b <= '9' }

// --- OpenDocument text ---

// odtText reads an .odt file's text: its paragraphs and headings on their
// own lines, tabs, line breaks, repeated spaces and table cells.
func odtText(data []byte) (string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", errors.New("the ODT file is damaged")
	}
	if len(r.File) > maxZipEntries {
		return "", errors.New("the ODT file has too many parts")
	}
	parts := map[string]string{}
	for _, f := range r.File {
		if f.Name != "content.xml" && f.Name != "styles.xml" {
			continue
		}
		content, err := readEntry(f, maxODTContent)
		if err != nil {
			return "", err
		}
		if parts[f.Name], err = odtContent(content); err != nil {
			return "", err
		}
	}
	body, ok := parts["content.xml"]
	if !ok {
		return "", errors.New("the ODT file has no content")
	}
	// The page header and footer (where many CVs put the name and contacts)
	// are in styles.xml; its only paragraphs are theirs.
	return strings.TrimSpace(strings.TrimSpace(parts["styles.xml"]) + "\n" + body), nil
}

func odtContent(content []byte) (string, error) {
	const textNS = "urn:oasis:names:tc:opendocument:xmlns:text:1.0"
	const tableNS = "urn:oasis:names:tc:opendocument:xmlns:table:1.0"
	d := xml.NewDecoder(bytes.NewReader(content))
	var out strings.Builder
	inPara := 0 // text is only ever inside a paragraph or heading
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", errors.New("the ODT file is damaged")
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space == textNS && (t.Name.Local == "p" || t.Name.Local == "h") {
				inPara++
			}
			switch {
			case t.Name.Space == textNS && t.Name.Local == "tab":
				out.WriteString("\t")
			case t.Name.Space == textNS && t.Name.Local == "line-break":
				out.WriteString("\n")
			case t.Name.Space == textNS && t.Name.Local == "s":
				n := 1
				for _, a := range t.Attr {
					if a.Name.Local == "c" {
						if v, err := strconv.Atoi(a.Value); err == nil && v > 0 && v < 1000 {
							n = v
						}
					}
				}
				out.WriteString(strings.Repeat(" ", n))
			case t.Name.Space == textNS && t.Name.Local == "list-item":
				out.WriteString("• ")
			}
		case xml.EndElement:
			switch {
			case t.Name.Space == textNS && (t.Name.Local == "p" || t.Name.Local == "h"):
				inPara = max(inPara-1, 0)
				out.WriteString("\n")
			case t.Name.Space == tableNS && t.Name.Local == "table-cell":
				out.WriteString("\t")
			case t.Name.Space == tableNS && t.Name.Local == "table-row":
				out.WriteString("\n")
			}
		case xml.CharData:
			if inPara > 0 {
				out.Write(t)
			}
		}
		if out.Len() > maxText*4 {
			break
		}
	}
	text := strings.ReplaceAll(out.String(), "• \n", "") // a list item's own paragraph closes it
	return text, nil
}

// readEntry reads a zip entry, refusing one larger than limit once unzipped.
func readEntry(f *zip.File, limit int64) ([]byte, error) {
	if f.UncompressedSize64 > uint64(limit) { //nolint:gosec // limit is positive
		return nil, errors.New("a part of the file is too large")
	}
	rc, err := f.Open()
	if err != nil {
		return nil, errors.New("the file is damaged")
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, errors.New("the file is damaged")
	}
	if int64(len(b)) > limit {
		return nil, errors.New("a part of the file is too large")
	}
	return b, nil
}
