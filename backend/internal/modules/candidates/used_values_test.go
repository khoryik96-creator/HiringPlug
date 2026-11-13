package candidates

import (
	"testing"
	"time"
)

func TestSuggestionsAreRememberedUntilSomethingNewIsSaved(t *testing.T) {
	now := time.Unix(0, 0)
	c := usedCache{now: func() time.Time { return now }}
	if _, ok := c.get("tnt_a", "all"); ok {
		t.Fatal("nothing remembered yet")
	}
	audit, sap := "Audit", "SAP"
	c.put("tnt_a", "all", UsedValues{Tags: []string{"relocate"}, Skills: []string{"Audit"}, Industries: []string{"Banking"}})
	c.put("tnt_a", "own", UsedValues{Tags: []string{"relocate"}})
	if v, ok := c.get("tnt_a", "all"); !ok || len(v.Skills) != 1 {
		t.Fatal("remembered")
	}
	if _, ok := c.get("tnt_b", "all"); ok {
		t.Fatal("another company's are its own")
	}

	c.learn("tnt_a", Profile{Tags: []string{"Relocate"}, Skills: []string{audit}})
	if _, ok := c.get("tnt_a", "all"); !ok {
		t.Fatal("a profile with only values already suggested (whatever their case) changes nothing")
	}
	c.learn("tnt_a", Profile{Skills: []string{sap}})
	if _, ok := c.get("tnt_a", "all"); ok {
		t.Fatal("a value not seen before forgets the suggestions, so it shows at once")
	}
	if _, ok := c.get("tnt_a", "own"); ok {
		t.Fatal("and for those who see only their own")
	}

	c.put("tnt_a", "all", UsedValues{})
	now = now.Add(usedFor + time.Second)
	if _, ok := c.get("tnt_a", "all"); ok {
		t.Fatal("counted again after a few minutes")
	}
}
