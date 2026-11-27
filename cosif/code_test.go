package cosif

import (
	"errors"
	"testing"
)

// Codes published by the BCB (4010 XML example and Instruções Normativas
// 429/2023, 493/2024 and 640/2025).
var published = []string{
	"1000000009", "1100000002", "1130000003", "1139000004",
	"4.1.0.00.00.00-9", "4.1.1.00.00.00-6", "1.1.0.00.00.00-2", "1.2.0.00.00.00-5", "9.0.9.61.00.00-1",
}

func TestCheckDigitMatchesPublishedCodes(t *testing.T) {
	for _, s := range published {
		if _, err := Parse(s); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	if _, err := Parse("1100000003"); !errors.Is(err, ErrCheckDigit) {
		t.Fatalf("wrong check digit accepted: %v", err)
	}
	if _, err := Parse("11000"); !errors.Is(err, ErrFormat) {
		t.Fatal("short code accepted")
	}
}

func TestFormsAndLevels(t *testing.T) {
	c := MustParse("909610000")
	if c.String() != "9.0.9.61.00.00-1" || c.Compact() != "9096100001" || c.Level() != 4 || c.Group() != 9 {
		t.Fatalf("%s %s %d", c, c.Compact(), c.Level())
	}
	p, ok := c.Parent()
	if !ok || p.String() != "9.0.9.00.00.00-"+string(p.CheckDigit()) || p.Level() != 3 {
		t.Fatalf("parent %s", p)
	}
	anc := MustParse("1.1.3.90.00.00").Ancestors()
	if len(anc) != 3 || anc[0].Compact() != "1000000009" || anc[1].Compact() != "1100000002" || anc[2].Compact() != "1130000003" {
		t.Fatalf("ancestors %v", anc)
	}
	if !MustParse("1000000009").IsAncestorOf(MustParse("1139000004")) || MustParse("1139000004").IsAncestorOf(MustParse("1130000003")) {
		t.Fatal("IsAncestorOf")
	}
	deep := MustParse("1.1.3.90.12.34")
	if deep.Level() != 6 {
		t.Fatalf("level %d", deep.Level())
	}
	if _, ok := MustParse("1000000009").Parent(); ok {
		t.Fatal("group has no parent")
	}
}

func TestNature(t *testing.T) {
	for g, want := range map[int]Nature{1: Debit, 2: Debit, 3: Debit, 4: Credit, 5: Credit, 6: Credit, 7: Credit, 8: Debit, 9: Credit} {
		if GroupNature(g) != want {
			t.Errorf("group %d", g)
		}
	}
}
