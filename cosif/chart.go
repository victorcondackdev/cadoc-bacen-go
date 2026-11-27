package cosif

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Account is one rubric of a chart of accounts.
type Account struct {
	Code Code
	Name string
	// Contra marks a contra account (retificadora, "(-) ..."), whose normal
	// balance is opposite to its group's nature.
	Contra bool
}

// Nature returns the account's normal balance side.
func (a Account) Nature() Nature {
	n := GroupNature(a.Code.Group())
	if a.Contra {
		return 1 - n
	}
	return n
}

// Chart is a chart of accounts keyed by code.
type Chart struct {
	accounts map[Code]Account
}

// NewChart builds a chart, checking that every account's ancestors exist
// and that a contra account is never an ancestor (contra accounts are
// leaves).
func NewChart(accounts []Account) (*Chart, error) {
	c := &Chart{accounts: map[Code]Account{}}
	for _, a := range accounts {
		if _, dup := c.accounts[a.Code]; dup {
			return nil, fmt.Errorf("cosif: duplicate account %s", a.Code)
		}
		c.accounts[a.Code] = a
	}
	for _, a := range accounts {
		for _, p := range a.Code.Ancestors() {
			pa, ok := c.accounts[p]
			if !ok {
				return nil, fmt.Errorf("cosif: %s has no parent %s in the chart", a.Code, p)
			}
			if pa.Contra {
				return nil, fmt.Errorf("cosif: contra account %s cannot have children", p)
			}
		}
	}
	return c, nil
}

// Account returns an account by code.
func (c *Chart) Account(code Code) (Account, bool) {
	a, ok := c.accounts[code]
	return a, ok
}

// Accounts returns all accounts ordered by code.
func (c *Chart) Accounts() []Account {
	out := make([]Account, 0, len(c.accounts))
	for _, a := range c.accounts {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code.Less(out[j].Code) })
	return out
}

// IsLeaf reports whether no account in the chart has code as ancestor.
func (c *Chart) IsLeaf(code Code) bool {
	for k := range c.accounts {
		if code.IsAncestorOf(k) {
			return false
		}
	}
	return true
}

// ReadChart reads a CSV chart: code;name[;R] where R marks a contra
// account. Lines starting with '#' are comments.
func ReadChart(r io.Reader) (*Chart, error) {
	cr := csv.NewReader(r)
	cr.Comma = ';'
	cr.Comment = '#'
	cr.FieldsPerRecord = -1
	var accs []Account
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) < 2 {
			return nil, fmt.Errorf("cosif: line %v: want code;name[;R]", rec)
		}
		code, err := Parse(strings.TrimSpace(rec[0]))
		if err != nil {
			return nil, fmt.Errorf("cosif: %q: %w", rec[0], err)
		}
		accs = append(accs, Account{Code: code, Name: strings.TrimSpace(rec[1]),
			Contra: len(rec) > 2 && strings.EqualFold(strings.TrimSpace(rec[2]), "R")})
	}
	return NewChart(accs)
}
