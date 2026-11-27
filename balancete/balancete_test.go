package balancete_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/victorcondackdev/cadoc-bacen-go/balancete"
	"github.com/victorcondackdev/cadoc-bacen-go/cosif"
	"github.com/victorcondackdev/cadoc-bacen-go/internal/example"
)

func c(s string) cosif.Code { return cosif.MustParse(s) }

func TestRollUpAndChecks(t *testing.T) {
	b := example.Balancete()
	if issues := b.Check(); len(issues) != 0 {
		t.Fatalf("issues: %v", issues)
	}
	// Loans: 300,000 + 150,000 + interest 12,180.55 - provision 13,500.
	if got := b.Signed(c("160000000")); got != 448_680_55 {
		t.Fatalf("operações de crédito = %d", got)
	}
	// Equity: 1,000,000 subscribed - 200,000 to be paid in (contra).
	if got := -b.Signed(c("600000000")); got != 800_000_00 {
		t.Fatalf("PL = %d", got)
	}
	var contra balancete.Row
	for _, r := range b.Rows(false) {
		if r.Account.Code == c("161900000") {
			contra = r
		}
	}
	if contra.Saldo != 13_500_00 || contra.Inverted || contra.Account.Nature() != cosif.Credit {
		t.Fatalf("contra account row %+v", contra)
	}
}

func TestCheckFindsProblems(t *testing.T) {
	chart := cosif.SampleChart()
	b, err := balancete.FromLeaves(chart, map[cosif.Code]balancete.Money{c("113000000"): 100, c("411000000"): -90, c("310000000"): 5})
	if err != nil {
		t.Fatal(err)
	}
	got := ""
	for _, i := range b.Check() {
		got += i.String() + "\n"
	}
	for _, want := range []string{"não fecham: diferença de 0.15", "compensação ativa (3) e passiva (9) não se anulam"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in\n%s", want, got)
		}
	}
	inv, _ := balancete.FromLeaves(chart, map[cosif.Code]balancete.Money{c("113000000"): -100, c("611000000"): 100})
	// Only analytical accounts are flagged, plus negative equity.
	s := inv.Check()
	if len(s) != 3 || !strings.Contains(s[2].Message, "patrimônio líquido negativo") {
		t.Fatalf("inverted: %v", s)
	}
	for _, i := range s {
		if i.Severity != "aviso" {
			t.Fatalf("inversion must be a warning: %v", i)
		}
	}
}

func TestPostingRules(t *testing.T) {
	chart := cosif.SampleChart()
	if _, err := balancete.FromLeaves(chart, map[cosif.Code]balancete.Money{c("110000000"): 1}); !errors.Is(err, balancete.ErrNotLeaf) {
		t.Fatalf("synthetic posting: %v", err)
	}
	if _, err := balancete.FromLeaves(chart, map[cosif.Code]balancete.Money{c("118000000"): 1}); !errors.Is(err, balancete.ErrUnknownAccount) {
		t.Fatalf("unknown: %v", err)
	}
	if _, err := balancete.FromEntries(chart, []balancete.Entry{{Account: c("113000000"), Debit: 1, Credit: 1}}); !errors.Is(err, balancete.ErrEntry) {
		t.Fatalf("two-sided entry: %v", err)
	}
}

func TestCloseResult(t *testing.T) {
	b := example.Balancete()
	if !b.HasResult() {
		t.Fatal("sample month has a result")
	}
	closed, err := b.CloseResult(c("641000000"), c("649000000"))
	if err != nil {
		t.Fatal(err)
	}
	if closed.HasResult() {
		t.Fatal("result still open")
	}
	if issues := closed.Check(); len(issues) != 0 {
		t.Fatalf("after closing: %v", issues)
	}
	// Result: revenue 15,636.45 - expenses 56,564.93 = loss of 40,928.48,
	// carried by the contra account "(-) Prejuízos acumulados".
	if got := closed.Signed(c("649000000")); got != 40_928_48 {
		t.Fatalf("accumulated loss = %d", got)
	}
	if closed.Signed(c("641000000")) != 0 {
		t.Fatal("a loss must not touch the profit account")
	}
	if _, err := b.CloseResult(c("113000000"), c("649000000")); err == nil {
		t.Fatal("closing into an asset accepted")
	}
}
