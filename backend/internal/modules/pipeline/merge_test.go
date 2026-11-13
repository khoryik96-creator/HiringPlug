package pipeline

import "testing"

func TestOverlapDrop(t *testing.T) {
	for _, tc := range []struct {
		name string
		o    Overlap
		drop string
		ok   bool
	}{
		{"the duplicate's has a placement: the bare kept one goes", Overlap{FromID: "dup", ToID: "keep", FromHas: true}, "keep", true},
		{"the kept one has interviews: the duplicate's bare one goes", Overlap{FromID: "dup", ToID: "keep", ToHas: true}, "dup", true},
		{"both carry records: neither is deleted", Overlap{FromID: "dup", ToID: "keep", FromHas: true, ToHas: true}, "", false},
		{"both bare: the kept candidate's stays", Overlap{FromID: "dup", ToID: "keep"}, "dup", true},
		{"both bare, the duplicate's moved last: it is the truth", Overlap{FromID: "dup", ToID: "keep", FromChangedLatest: true}, "keep", true},
	} {
		id, ok := tc.o.Drop()
		if id != tc.drop || ok != tc.ok {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", tc.name, id, ok, tc.drop, tc.ok)
		}
	}
}
