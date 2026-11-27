package doc4010_test

import (
	"errors"
	"flag"
	"math/rand"
	"os"
	"strings"
	"testing"

	"github.com/victorcondackdev/cadoc-bacen-go/balancete"
	"github.com/victorcondackdev/cadoc-bacen-go/bizday"
	"github.com/victorcondackdev/cadoc-bacen-go/cosif"
	"github.com/victorcondackdev/cadoc-bacen-go/doc4010"
	"github.com/victorcondackdev/cadoc-bacen-go/internal/example"
)

var update = flag.Bool("update", false, "rewrite golden files")

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := "testdata/" + name
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (go test ./doc4010 -update)", err)
	}
	if string(want) != string(got) {
		t.Fatalf("%s differs:\n%s", name, got)
	}
}

func TestBuild4010(t *testing.T) {
	d, err := doc4010.Build(doc4010.Balancete, "12345678", "2026-10", doc4010.Inclusion, example.Balancete(), doc4010.Options{})
	if err != nil {
		t.Fatal(err)
	}
	out := d.Marshal()
	golden(t, "4010.xml", out)
	if !strings.Contains(string(out), `<conta codigoConta="1619000005" saldo="13500.00"/>`) {
		t.Fatalf("contra account line missing:\n%s", out)
	}
	back, err := doc4010.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(back.Marshal()) != string(out) {
		t.Fatal("parse/marshal round trip")
	}
	if p := back.Validate(cosif.SampleChart()); len(p) != 0 {
		t.Fatalf("problems: %v", p)
	}
}

func TestBuild4016NeedsClosedResult(t *testing.T) {
	b := example.Balancete()
	_, err := doc4010.Build(doc4010.Balanco, "12345678", "2026-12", doc4010.Inclusion, b, doc4010.Options{})
	if !errors.Is(err, doc4010.ErrInvalid) || !strings.Contains(err.Error(), "encerre o resultado") {
		t.Fatalf("open result: %v", err)
	}
	closed, _ := b.CloseResult(cosif.MustParse("641000000"), cosif.MustParse("649000000"))
	_, err = doc4010.Build(doc4010.Balanco, "12345678", "2026-11", doc4010.Inclusion, closed, doc4010.Options{})
	if err == nil || !strings.Contains(err.Error(), "semestral") {
		t.Fatalf("November 4016: %v", err)
	}
	d, err := doc4010.Build(doc4010.Balanco, "12345678", "2026-12", doc4010.Inclusion, closed, doc4010.Options{})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "4016.xml", d.Marshal())
	for _, l := range d.Lines {
		if g := l.Code.Group(); g == 7 || g == 8 {
			t.Fatalf("result account %s in 4016", l.Code)
		}
	}
}

func TestInvertedBalanceIsRejected(t *testing.T) {
	chart := cosif.SampleChart()
	// An overdrawn reserve account: an asset with a credit balance.
	b, _ := balancete.FromLeaves(chart, map[cosif.Code]balancete.Money{cosif.MustParse("113000000"): -100, cosif.MustParse("111000000"): 100})
	_, err := doc4010.Build(doc4010.Balancete, "12345678", "2026-10", doc4010.Inclusion, b, doc4010.Options{})
	if !errors.Is(err, doc4010.ErrInvalid) || !strings.Contains(err.Error(), "reclassifique") {
		t.Fatalf("inverted balance: %v", err)
	}
}

func TestValidateCatchesTampering(t *testing.T) {
	d, _ := doc4010.Build(doc4010.Balancete, "12345678", "2026-10", doc4010.Inclusion, example.Balancete(), doc4010.Options{})
	xml := strings.Replace(string(d.Marshal()), `codigoConta="1611000003" saldo="309870.45"`, `codigoConta="1611000003" saldo="309870.46"`, 1)
	back, err := doc4010.Parse([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	p := back.Validate(cosif.SampleChart())
	if len(p) != 1 || !strings.Contains(p[0], "1.6.1.00.00.00-4") {
		t.Fatalf("problems: %v", p)
	}
	changes := doc4010.Diff(d, back)
	if len(changes) != 1 || changes[0].Kind != "alterada" || changes[0].New-changes[0].Old != 1 {
		t.Fatalf("diff %+v", changes)
	}
}

func TestParseRejects(t *testing.T) {
	head := `<documento codigoDocumento="4010" cnpj="12345678" dataBase="2026-10" tipoRemessa="I"><contas>`
	cases := map[string]string{
		"dígitos":       head + `<conta codigoConta="110000000" saldo="1.00"/></contas></documento>`,
		"should end in": head + `<conta codigoConta="1100000003" saldo="1.00"/></contas></documento>`,
		"padrão":        head + `<conta codigoConta="1100000002" saldo="-1.00"/></contas></documento>`,
		"repetida":      head + `<conta codigoConta="1100000002" saldo="1.00"/><conta codigoConta="1100000002" saldo="1.00"/></contas></documento>`,
		"AAAA-MM":       strings.Replace(head, "2026-10", "2026-10-31", 1) + `</contas></documento>`,
		"raiz":          strings.Replace(head, "12345678", "123", 1) + `</contas></documento>`,
		"I ou S":        strings.Replace(head, `"I"`, `"X"`, 1) + `</contas></documento>`,
	}
	for want, x := range cases {
		if _, err := doc4010.Parse([]byte(x)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", want, err)
		}
	}
}

func TestDeadlines(t *testing.T) {
	cases := []struct{ code, base, want string }{
		{doc4010.Balancete, "2026-10", "2026-11-18"},
		{doc4010.Balancete, "2026-03", "2026-04-20"}, // the 18th is a Saturday
		{doc4010.Balancete, "2026-12", "2027-01-18"},
		{doc4010.Balanco, "2026-06", "2026-07-31"},
		{doc4010.Balanco, "2026-12", "2027-01-29"}, // Jan 31 2027 is a Sunday
	}
	for _, c := range cases {
		d, err := doc4010.Deadline(c.code, c.base, nil)
		if err != nil || d.String() != c.want {
			t.Errorf("%s %s = %s %v, want %s", c.code, c.base, d, err, c.want)
		}
	}
	if _, err := doc4010.Deadline("4010", "2026-13", bizday.National{}); err == nil {
		t.Fatal("bad data-base accepted")
	}
}

// Every document Build accepts must parse back and validate cleanly.
func TestBuiltDocumentsAlwaysValidate(t *testing.T) {
	chart := cosif.SampleChart()
	var leaves []cosif.Code
	for _, a := range chart.Accounts() {
		if chart.IsLeaf(a.Code) {
			leaves = append(leaves, a.Code)
		}
	}
	rng := rand.New(rand.NewSource(1))
	built := 0
	for i := 0; i < 2000; i++ {
		m := map[cosif.Code]balancete.Money{}
		for k := 0; k < 2+rng.Intn(10); k++ {
			a, b := leaves[rng.Intn(len(leaves))], leaves[rng.Intn(len(leaves))]
			v := balancete.Money(rng.Int63n(1_000_000_00))
			m[a] += v
			m[b] -= v
		}
		bal, err := balancete.FromLeaves(chart, m)
		if err != nil {
			t.Fatal(err)
		}
		for _, code := range []string{doc4010.Balancete, doc4010.Balanco} {
			src := bal
			if code == doc4010.Balanco {
				if src, err = bal.CloseResult(cosif.MustParse("641000000"), cosif.MustParse("649000000")); err != nil {
					t.Fatal(err)
				}
			}
			d, err := doc4010.Build(code, "12345678", "2026-12", doc4010.Inclusion, src, doc4010.Options{IncludeZero: i%2 == 0})
			if err != nil {
				continue
			}
			built++
			back, err := doc4010.Parse(d.Marshal())
			if err != nil {
				t.Fatal(err)
			}
			if p := back.Validate(chart); len(p) != 0 {
				t.Fatalf("built document does not validate: %v\n%s", p, d.Marshal())
			}
		}
	}
	if built < 50 {
		t.Fatalf("only %d documents built; generator too strict", built)
	}
}
