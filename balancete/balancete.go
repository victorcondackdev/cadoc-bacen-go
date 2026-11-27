// Package balancete builds a Cosif trial balance from journal entries or
// leaf balances: it rolls balances up through every level of the chart,
// checks double entry and the offset between debit and credit memorandum
// accounts, flags balances contrary to an account's nature and closes the
// result of the period into equity for the semiannual balance sheet.
package balancete

import (
	"errors"
	"fmt"
	"sort"

	"github.com/victorcondackdev/cadoc-bacen-go/cosif"
)

// Money is an amount in centavos.
type Money int64

// Decimal formats the absolute value as "1234.56", the 4010 XML format.
func (m Money) Decimal() string {
	if m < 0 {
		m = -m
	}
	return fmt.Sprintf("%d.%02d", int64(m)/100, int64(m)%100)
}

// Entry is one line of a journal entry. Exactly one of Debit and Credit is
// positive.
type Entry struct {
	Account cosif.Code
	Debit   Money
	Credit  Money
}

// Balancete is a trial balance on a chart. Balances are signed, debit
// positive, for every account at every level.
type Balancete struct {
	chart    *cosif.Chart
	balances map[cosif.Code]Money
}

// Errors returned by this package.
var (
	ErrUnknownAccount = errors.New("balancete: account not in chart")
	ErrNotLeaf        = errors.New("balancete: postings must go to leaf accounts")
	ErrEntry          = errors.New("balancete: entry must have exactly one positive side")
)

// FromLeaves builds a trial balance from signed leaf balances.
func FromLeaves(chart *cosif.Chart, leaves map[cosif.Code]Money) (*Balancete, error) {
	b := &Balancete{chart: chart, balances: map[cosif.Code]Money{}}
	codes := make([]cosif.Code, 0, len(leaves))
	for c := range leaves {
		codes = append(codes, c)
	}
	sort.Slice(codes, func(i, j int) bool { return codes[i].Less(codes[j]) })
	for _, c := range codes {
		if _, ok := chart.Account(c); !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownAccount, c)
		}
		if !chart.IsLeaf(c) {
			return nil, fmt.Errorf("%w: %s", ErrNotLeaf, c)
		}
		v := leaves[c]
		b.balances[c] += v
		for _, a := range c.Ancestors() {
			b.balances[a] += v
		}
	}
	return b, nil
}

// FromEntries builds a trial balance from journal entries.
func FromEntries(chart *cosif.Chart, entries []Entry) (*Balancete, error) {
	leaves := map[cosif.Code]Money{}
	for _, e := range entries {
		if (e.Debit > 0) == (e.Credit > 0) || e.Debit < 0 || e.Credit < 0 {
			return nil, fmt.Errorf("%w (%s)", ErrEntry, e.Account)
		}
		leaves[e.Account] += e.Debit - e.Credit
	}
	return FromLeaves(chart, leaves)
}

// Chart returns the chart.
func (b *Balancete) Chart() *cosif.Chart { return b.chart }

// Signed returns the signed (debit positive) balance of any account.
func (b *Balancete) Signed(c cosif.Code) Money { return b.balances[c] }

// Row is one line of the trial balance.
type Row struct {
	Account  cosif.Account
	Signed   Money // debit positive
	Saldo    Money // absolute value, as reported
	Side     cosif.Nature
	Inverted bool // non-zero balance on the side opposite to the account's nature
}

// Rows returns every account with a non-zero balance (or every account,
// with all), ordered by code.
func (b *Balancete) Rows(all bool) []Row {
	var out []Row
	for _, a := range b.chart.Accounts() {
		v := b.balances[a.Code]
		if v == 0 && !all {
			continue
		}
		side := cosif.Debit
		if v < 0 {
			side = cosif.Credit
		}
		saldo := v
		if saldo < 0 {
			saldo = -saldo
		}
		out = append(out, Row{Account: a, Signed: v, Saldo: saldo, Side: side, Inverted: v != 0 && side != a.Nature()})
	}
	return out
}

// Issue is a finding of Check.
type Issue struct {
	Severity string // "erro" or "aviso"
	Message  string
}

func (i Issue) String() string { return i.Severity + ": " + i.Message }

// Check verifies double entry (all groups sum to zero) and that debit (3)
// and credit (9) memorandum groups offset each other, and warns about
// analytical (leaf) accounts whose balance is contrary to their nature and
// about negative equity. Synthetic accounts may net to either side (for
// example profits and losses under one title), so they are not flagged.
func (b *Balancete) Check() []Issue {
	var issues []Issue
	var total Money
	for g := 1; g <= 9; g++ {
		total += b.balances[groupCode(g)]
	}
	if total != 0 {
		issues = append(issues, Issue{"erro", fmt.Sprintf("débitos e créditos não fecham: diferença de %s", signedDecimal(total))})
	}
	if d := b.balances[groupCode(3)] + b.balances[groupCode(9)]; d != 0 {
		issues = append(issues, Issue{"erro", fmt.Sprintf("compensação ativa (3) e passiva (9) não se anulam: diferença de %s", signedDecimal(d))})
	}
	for _, r := range b.Rows(false) {
		if r.Inverted && b.chart.IsLeaf(r.Account.Code) {
			issues = append(issues, Issue{"aviso", fmt.Sprintf("%s %s com saldo %s, contrário à natureza %s", r.Account.Code, r.Account.Name, r.Side, r.Account.Nature())})
		}
	}
	if pl := b.balances[groupCode(6)]; pl > 0 {
		issues = append(issues, Issue{"aviso", fmt.Sprintf("patrimônio líquido negativo: %s", signedDecimal(-pl))})
	}
	return issues
}

func signedDecimal(m Money) string {
	if m < 0 {
		return "-" + m.Decimal()
	}
	return m.Decimal()
}

func groupCode(g int) cosif.Code { return cosif.MustParse(fmt.Sprintf("%d00000000", g)) }

// HasResult reports whether revenue (7) or expense (8) accounts carry
// balances.
func (b *Balancete) HasResult() bool {
	return b.balances[groupCode(7)] != 0 || b.balances[groupCode(8)] != 0
}

// CloseResult returns a new trial balance with the period's result
// (groups 7 and 8) transferred into equity, as done before the semiannual
// balance sheet (document 4016): a profit to profit, a loss to loss. Both
// must be leaf accounts of group 6; loss is normally a contra account
// ("(-) Prejuízos acumulados"), since the documents carry no sign.
func (b *Balancete) CloseResult(profit, loss cosif.Code) (*Balancete, error) {
	for _, c := range []cosif.Code{profit, loss} {
		if c.Group() != 6 {
			return nil, fmt.Errorf("balancete: result must close into group 6 accounts, not %s", c)
		}
	}
	leaves := map[cosif.Code]Money{}
	var result Money
	for _, a := range b.chart.Accounts() {
		if !b.chart.IsLeaf(a.Code) {
			continue
		}
		v := b.balances[a.Code]
		if g := a.Code.Group(); g == 7 || g == 8 {
			result += v
			continue
		}
		if v != 0 {
			leaves[a.Code] = v
		}
	}
	if result < 0 {
		leaves[profit] += result
	} else if result > 0 {
		leaves[loss] += result
	}
	return FromLeaves(b.chart, leaves)
}
