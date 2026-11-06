package ical

// Reading busy times from a calendar's address: Google Calendar's "secret
// address in iCal format" and an Outlook published calendar both serve an
// .ics file of the calendar's events. Only when someone is busy is kept:
// never what the events are.

import (
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/teambition/rrule-go"
)

// Interval is a busy time.
type Interval struct{ Start, End time.Time }

// maxIntervals bounds what one calendar can produce.
const maxIntervals = 5000

// ErrNotCalendar is returned for a file that is not an iCalendar.
var ErrNotCalendar = errors.New("this is not a calendar (.ics) file")

type prop struct {
	params map[string]string
	value  string
}

type event map[string][]prop

func (e event) one(name string) (prop, bool) {
	p, ok := e[name]
	if !ok || len(p) == 0 {
		return prop{}, false
	}
	return p[0], true
}

// Busy returns the busy times between from and to, sorted. Events marked
// free (TRANSP:TRANSPARENT, Outlook's BUSYSTATUS FREE) or cancelled are left
// out; repeating events are expanded (RRULE, RDATE, EXDATE and moved
// occurrences). Times without a zone are in loc.
func Busy(data []byte, from, to time.Time, loc *time.Location) ([]Interval, error) {
	text := string(data)
	if !strings.Contains(text, "BEGIN:VCALENDAR") {
		return nil, ErrNotCalendar
	}
	events, zones := parse(unfold(text))
	// Moved or changed occurrences of a repeating event replace the one at
	// their RECURRENCE-ID.
	moved := map[string]map[int64]bool{}
	for _, e := range events {
		if rid, ok := e.one("RECURRENCE-ID"); ok {
			uid, _ := e.one("UID")
			if t, _, err := parseTime(rid, loc, zones); err == nil {
				if moved[uid.value] == nil {
					moved[uid.value] = map[int64]bool{}
				}
				moved[uid.value][t.Unix()] = true
			}
		}
	}
	var out []Interval
	for _, e := range events {
		if !busy(e) {
			continue
		}
		start, allDay, err := dtstart(e, loc, zones)
		if err != nil {
			continue // an event we cannot place is not guessed at
		}
		length := duration(e, start, allDay, loc, zones)
		if length <= 0 {
			continue
		}
		_, isOverride := e.one("RECURRENCE-ID")
		uid, _ := e.one("UID")
		for _, s := range occurrences(e, start, length, from, to, zones) {
			if !isOverride && moved[uid.value][s.Unix()] {
				continue
			}
			end := s.Add(length)
			if allDay { // whole days in the calendar's own time
				end = s.AddDate(0, 0, int(length/(24*time.Hour)))
			}
			if end.After(from) && s.Before(to) {
				out = append(out, Interval{Start: s, End: end})
				if len(out) >= maxIntervals {
					break
				}
			}
		}
		if len(out) >= maxIntervals {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out, nil
}

func busy(e event) bool {
	if p, ok := e.one("STATUS"); ok && strings.EqualFold(p.value, "CANCELLED") {
		return false
	}
	if p, ok := e.one("TRANSP"); ok && strings.EqualFold(p.value, "TRANSPARENT") {
		return false
	}
	if p, ok := e.one("X-MICROSOFT-CDO-BUSYSTATUS"); ok && strings.EqualFold(p.value, "FREE") {
		return false
	}
	return true
}

func unfold(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if (strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")) && len(lines) > 0 {
			lines[len(lines)-1] += l[1:]
			continue
		}
		lines = append(lines, l)
	}
	return lines
}

// parse returns the events, and each time zone's standard offset (a fallback
// for zone names Go does not know, such as Outlook's Windows names).
func parse(lines []string) ([]event, map[string]*time.Location) {
	var events []event
	zones := map[string]*time.Location{}
	var cur event
	var zoneID string
	inStandard := false
	for _, line := range lines {
		name, params, value := split(line)
		switch {
		case name == "BEGIN" && value == "VEVENT":
			cur = event{}
		case name == "END" && value == "VEVENT":
			if cur != nil {
				events = append(events, cur)
			}
			cur = nil
		case name == "BEGIN" && value == "VTIMEZONE":
			zoneID = ""
		case name == "TZID" && cur == nil:
			zoneID = value
		case name == "BEGIN" && value == "STANDARD":
			inStandard = true
		case name == "END" && value == "STANDARD":
			inStandard = false
		case name == "TZOFFSETTO" && inStandard && zoneID != "":
			if off, ok := offset(value); ok {
				zones[zoneID] = time.FixedZone(zoneID, off)
			}
		case cur != nil && name != "":
			cur[name] = append(cur[name], prop{params: params, value: value})
		}
	}
	return events, zones
}

// split reads "NAME;P1=a;P2="b:c":value".
func split(line string) (string, map[string]string, string) {
	inQuote := false
	colon := -1
	for i, r := range line {
		if r == '"' {
			inQuote = !inQuote
		}
		if r == ':' && !inQuote {
			colon = i
			break
		}
	}
	if colon < 0 {
		return "", nil, ""
	}
	head, value := line[:colon], line[colon+1:]
	parts := strings.Split(head, ";")
	params := map[string]string{}
	for _, p := range parts[1:] {
		if k, v, ok := strings.Cut(p, "="); ok {
			params[strings.ToUpper(k)] = strings.Trim(v, `"`)
		}
	}
	return strings.ToUpper(parts[0]), params, strings.TrimSpace(value)
}

var offsetRE = regexp.MustCompile(`^([+-])(\d{2})(\d{2})(\d{2})?$`)

func offset(s string) (int, bool) {
	m := offsetRE.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	h, _ := strconv.Atoi(m[2])
	mi, _ := strconv.Atoi(m[3])
	secs := h*3600 + mi*60
	if m[1] == "-" {
		secs = -secs
	}
	return secs, true
}

// windowsZones maps the Windows zone names Outlook writes to IANA ones (the
// common ones: others fall back to the file's own VTIMEZONE).
var windowsZones = map[string]string{
	"Singapore Standard Time": "Asia/Singapore", "Malay Peninsula Standard Time": "Asia/Kuala_Lumpur",
	"China Standard Time": "Asia/Shanghai", "Tokyo Standard Time": "Asia/Tokyo", "India Standard Time": "Asia/Kolkata",
	"SE Asia Standard Time": "Asia/Bangkok", "W. Australia Standard Time": "Australia/Perth",
	"AUS Eastern Standard Time": "Australia/Sydney", "GMT Standard Time": "Europe/London",
	"W. Europe Standard Time": "Europe/Berlin", "Romance Standard Time": "Europe/Paris",
	"Eastern Standard Time": "America/New_York", "Central Standard Time": "America/Chicago",
	"Pacific Standard Time": "America/Los_Angeles", "UTC": "UTC", "Arabian Standard Time": "Asia/Dubai",
	"Taipei Standard Time": "Asia/Taipei", "Korea Standard Time": "Asia/Seoul", "Philippines Standard Time": "Asia/Manila",
}

func zone(id string, zones map[string]*time.Location) *time.Location {
	if loc, err := time.LoadLocation(id); err == nil && id != "" && id != "Local" {
		return loc
	}
	if name, ok := windowsZones[id]; ok {
		if loc, err := time.LoadLocation(name); err == nil {
			return loc
		}
	}
	return zones[id]
}

// parseTime reads a DATE-TIME (UTC, with a TZID, or floating) or a DATE.
func parseTime(p prop, loc *time.Location, zones map[string]*time.Location) (time.Time, bool, error) {
	v := p.value
	if p.params["VALUE"] == "DATE" || len(v) == 8 {
		t, err := time.ParseInLocation("20060102", v, loc)
		return t, true, err
	}
	if strings.HasSuffix(v, "Z") {
		t, err := time.Parse("20060102T150405Z", v)
		return t, false, err
	}
	in := loc
	if id := p.params["TZID"]; id != "" {
		if z := zone(id, zones); z != nil {
			in = z
		}
	}
	t, err := time.ParseInLocation("20060102T150405", v, in)
	return t, false, err
}

func dtstart(e event, loc *time.Location, zones map[string]*time.Location) (time.Time, bool, error) {
	p, ok := e.one("DTSTART")
	if !ok {
		return time.Time{}, false, errors.New("no start")
	}
	return parseTime(p, loc, zones)
}

func duration(e event, start time.Time, allDay bool, loc *time.Location, zones map[string]*time.Location) time.Duration {
	if p, ok := e.one("DTEND"); ok {
		if end, _, err := parseTime(p, loc, zones); err == nil {
			return end.Sub(start)
		}
	}
	if p, ok := e.one("DURATION"); ok {
		if d, ok := isoDuration(p.value); ok {
			return d
		}
	}
	if allDay {
		return 24 * time.Hour
	}
	return 0
}

var durationRE = regexp.MustCompile(`^([+-])?P(?:(\d+)W)?(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$`)

func isoDuration(s string) (time.Duration, bool) {
	m := durationRE.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	n := func(i int) time.Duration { v, _ := strconv.Atoi(m[i]); return time.Duration(v) }
	d := n(2)*7*24*time.Hour + n(3)*24*time.Hour + n(4)*time.Hour + n(5)*time.Minute + n(6)*time.Second
	if m[1] == "-" {
		d = -d
	}
	return d, true
}

// occurrences are the event's starts that may overlap [from, to).
func occurrences(e event, start time.Time, length time.Duration, from, to time.Time, zones map[string]*time.Location) []time.Time {
	rule, ok := e.one("RRULE")
	_, hasRdate := e.one("RDATE")
	if !ok && !hasRdate {
		return []time.Time{start}
	}
	set := rrule.Set{}
	if ok {
		opt, err := rrule.StrToROption(rule.value)
		if err != nil {
			return []time.Time{start}
		}
		opt.Dtstart = start
		r, err := rrule.NewRRule(*opt)
		if err != nil {
			return []time.Time{start}
		}
		set.RRule(r)
	} else {
		set.RDate(start)
	}
	for _, name := range []string{"RDATE", "EXDATE"} {
		for _, p := range e[name] {
			for _, v := range strings.Split(p.value, ",") {
				t, _, err := parseTime(prop{params: p.params, value: v}, start.Location(), zones)
				if err != nil {
					continue
				}
				if name == "RDATE" {
					set.RDate(t)
				} else {
					set.ExDate(t)
				}
			}
		}
	}
	return set.Between(from.Add(-length), to, true)
}
