package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

var sourceLabel = regexp.MustCompile(`^[a-zA-Z0-9_. -]{0,80}$`)

// A decoder can read ahead, so each reset permits at most another 16 MiB.
// Arrays/envelopes stream; one enormous object is flagged without losing it.
type recordBudget struct {
	reader io.Reader
	left   int64
}

func (r *recordBudget) Read(p []byte) (int, error) {
	if r.left <= 0 {
		return 0, errors.New("source record exceeds indexing bound")
	}
	if int64(len(p)) > r.left {
		p = p[:r.left]
	}
	n, err := r.reader.Read(p)
	r.left -= int64(n)
	return n, err
}

func (s *Store) indexSourceRows(ctx context.Context, member int64, input io.Reader, kind, entity, idField string) (int64, error) {
	if entity == "" {
		entity = "unmapped"
	}
	if !sourceLabel.MatchString(entity) || len(idField) > 128 {
		return 0, errors.New("invalid source mapping")
	}
	r, err := sourceText(input)
	if err != nil {
		return 0, err
	}
	var count int64
	// Batches bound SQLite overhead without retaining row values in memory.
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	insert := func(value any, pointer string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		id := ""
		if object, ok := value.(map[string]any); ok {
			if idField != "" {
				id = scalarID(object[idField])
			} else {
				for key, e := range sourceEntities {
					if e == entity {
						if value := scalarID(object[key]); value != "" {
							id = value
							break
						}
					}
				}
			}
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO source_rows(member_id,entity,source_id,pointer) VALUES(?,?,?,?)`, member, entity, id, pointer); err != nil {
			return err
		}
		count++
		if count%1000 == 0 {
			if err := tx.Commit(); err != nil {
				return err
			}
			var err error
			tx, err = s.db.Begin()
			if err != nil {
				return err
			}
		}
		return nil
	}
	if kind == ".csv" {
		budget := &recordBudget{r, 16 << 20}
		reader := csv.NewReader(budget)
		reader.ReuseRecord = true
		headers, err := reader.Read()
		if err != nil {
			return count, err
		}
		headers = append([]string(nil), headers...)
		seen := map[string]bool{}
		for _, header := range headers {
			if seen[header] {
				return count, errors.New("duplicate CSV columns require explicit mapping")
			}
			seen[header] = true
		}
		for {
			budget.left = 16 << 20
			row, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return count, err
			}
			object := map[string]any{}
			for i, key := range headers {
				object[key] = row[i]
			}
			if err := insert(object, "row:"+strconv.FormatInt(count+2, 10)); err != nil {
				return count, err
			}
		}
	} else if kind == ".jsonl" || kind == ".ndjson" {
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 64<<10), 16<<20)
		var line int64
		for scanner.Scan() {
			line++
			if strings.TrimSpace(scanner.Text()) == "" {
				continue
			}
			value, err := decodePage(scanner.Bytes())
			if err != nil {
				return count, err
			}
			if err := insert(value, "line:"+strconv.FormatInt(line, 10)); err != nil {
				return count, err
			}
		}
		if err := scanner.Err(); err != nil {
			return count, err
		}
	} else {
		budget := &recordBudget{r, 16 << 20}
		d := json.NewDecoder(budget)
		d.UseNumber()
		start, err := d.Token()
		if err != nil {
			return count, err
		}
		array := func(prefix string) error {
			for d.More() {
				budget.left = 16 << 20
				var v any
				if err := d.Decode(&v); err != nil {
					return err
				}
				if err := insert(v, prefix+strconv.FormatInt(count, 10)); err != nil {
					return err
				}
			}
			_, err := d.Token()
			return err
		}
		switch start {
		case json.Delim('['):
			err = array("/")
		case json.Delim('{'):
			// Logical exports may be {items:[...]} or a single record. Values
			// remain in the original; no arbitrary JSON is executed or normalized.
			object := map[string]any{}
			hadItems := false
			for d.More() {
				budget.left = 16 << 20
				key, e := d.Token()
				if e != nil {
					return count, e
				}
				name, ok := key.(string)
				if !ok {
					return count, errors.New("invalid object key")
				}
				if name == "items" {
					token, e := d.Token()
					if e != nil || token != json.Delim('[') {
						return count, errors.New("items must be an array")
					}
					if err = array("/items/"); err != nil {
						return count, err
					}
					hadItems = true
				} else {
					var v any
					if err = d.Decode(&v); err != nil {
						return count, err
					}
					if _, known := sourceEntities[name]; known || name == idField {
						object[name] = v
					}
				}
			}
			if _, err = d.Token(); err == nil && !hadItems {
				err = insert(object, "")
			}
		default:
			err = errors.New("expected a JSON object or array")
		}
		if err != nil {
			return count, err
		}
		budget.left = 16 << 20
		if d.Decode(new(any)) != io.EOF {
			return count, errors.New("trailing source data")
		}
	}
	if err := tx.Commit(); err != nil {
		return count, err
	}
	return count, nil
}

func sourceText(r io.Reader) (io.Reader, error) {
	b := bufio.NewReader(r)
	prefix, _ := b.Peek(3)
	if len(prefix) >= 3 && string(prefix[:3]) == "\xef\xbb\xbf" {
		_, _ = b.Discard(3)
		return b, nil
	}
	if len(prefix) >= 2 && (string(prefix[:2]) == "\xff\xfe" || string(prefix[:2]) == "\xfe\xff") {
		little := prefix[0] == 0xff
		_, _ = b.Discard(2)
		return &utf16Reader{r: b, little: little}, nil
	}
	return b, nil
}

type utf16Reader struct {
	r       *bufio.Reader
	little  bool
	pending []byte
}

func (r *utf16Reader) unit() (uint16, error) {
	a, err := r.r.ReadByte()
	if err != nil {
		return 0, err
	}
	b, err := r.r.ReadByte()
	if err != nil {
		return 0, errors.New("truncated UTF-16")
	}
	if r.little {
		return uint16(a) | uint16(b)<<8, nil
	}
	return uint16(a)<<8 | uint16(b), nil
}
func (r *utf16Reader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(r.pending) == 0 {
		a, err := r.unit()
		if err != nil {
			return 0, err
		}
		char := rune(a)
		if utf16.IsSurrogate(char) {
			b, err := r.unit()
			if err != nil {
				return 0, errors.New("truncated UTF-16 surrogate")
			}
			char = utf16.DecodeRune(char, rune(b))
			if char == '\ufffd' {
				return 0, errors.New("invalid UTF-16 surrogate")
			}
		}
		r.pending = []byte(string(char))
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}
