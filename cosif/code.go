// Package cosif models account codes of the Cosif (Padrão Contábil das
// Instituições Reguladas pelo Banco Central do Brasil): the nine-digit,
// six-level code with its check digit, the hierarchy between levels, the
// nature of each group and a chart of accounts.
//
// Since January 2025 a Cosif code has six levels — group (1 digit),
// subgroup (1), subgroup breakdown (1), title (2), first-degree subtitle (2)
// and second-degree subtitle (2) — followed by a check digit, written
// 1.1.0.00.00.00-2 or, in XML documents, 1100000002.
package cosif

import (
	"errors"
	"fmt"
	"strings"
)

// Code is a Cosif account code: nine digits plus check digit.
type Code struct{ body [9]byte }

// Errors returned by this package.
var (
	ErrFormat     = errors.New("cosif: code must be 9 digits plus check digit (1.1.0.00.00.00-2 or 1100000002)")
	ErrCheckDigit = errors.New("cosif: invalid check digit")
)

var weights = [9]int{1, 7, 3, 1, 7, 3, 1, 7, 3}

// CheckDigit computes the check digit of a nine-digit body: digits
// weighted 1, 7, 3 from the left, the sum's complement to the next ten
// (zero when the sum is a multiple of ten).
func CheckDigit(body string) (byte, error) {
	if len(body) != 9 {
		return 0, ErrFormat
	}
	sum := 0
	for i := 0; i < 9; i++ {
		c := body[i]
		if c < '0' || c > '9' {
			return 0, ErrFormat
		}
		sum += int(c-'0') * weights[i]
	}
	return byte('0' + (10-sum%10)%10), nil
}

// Parse accepts "1.1.0.00.00.00-2", "1100000002" or the nine-digit body
// alone ("110000000", check digit computed).
func Parse(s string) (Code, error) {
	digits := strings.NewReplacer(".", "", "-", "", " ", "").Replace(s)
	var c Code
	if len(digits) != 9 && len(digits) != 10 {
		return c, ErrFormat
	}
	dv, err := CheckDigit(digits[:9])
	if err != nil {
		return c, err
	}
	if len(digits) == 10 && digits[9] != dv {
		return c, fmt.Errorf("%w: %s should end in %c", ErrCheckDigit, s, dv)
	}
	copy(c.body[:], digits[:9])
	return c, nil
}

// MustParse is Parse for constants.
func MustParse(s string) Code {
	c, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return c
}

// IsZero reports whether c is the zero Code.
func (c Code) IsZero() bool { return c.body == [9]byte{} }

// Body returns the nine digits without check digit.
func (c Code) Body() string { return string(c.body[:]) }

// CheckDigit returns the check digit.
func (c Code) CheckDigit() byte {
	d, _ := CheckDigit(c.Body())
	return d
}

// Compact returns the ten-digit form used in XML documents.
func (c Code) Compact() string { return c.Body() + string(c.CheckDigit()) }

// String returns the dotted form 1.1.0.00.00.00-2.
func (c Code) String() string {
	b := c.Body()
	return fmt.Sprintf("%s.%s.%s.%s.%s.%s-%c", b[0:1], b[1:2], b[2:3], b[3:5], b[5:7], b[7:9], c.CheckDigit())
}

// level boundaries: end offset of each level in the body.
var levelEnd = [6]int{1, 2, 3, 5, 7, 9}

// Level returns 1 (group) to 6 (second-degree subtitle): the deepest level
// with a non-zero digit.
func (c Code) Level() int {
	b := c.Body()
	for l := 5; l >= 1; l-- {
		if strings.Trim(b[levelEnd[l-1]:levelEnd[l]], "0") != "" {
			return l + 1
		}
	}
	return 1
}

// Group returns the first digit (1–9).
func (c Code) Group() int { return int(c.body[0] - '0') }

// Parent returns the code one level up, and false for a group.
func (c Code) Parent() (Code, bool) {
	l := c.Level()
	if l == 1 {
		return Code{}, false
	}
	p := c
	for i := levelEnd[l-2]; i < 9; i++ {
		p.body[i] = '0'
	}
	return p, true
}

// Ancestors returns the parents of c from the group down.
func (c Code) Ancestors() []Code {
	var out []Code
	for p, ok := c.Parent(); ok; p, ok = p.Parent() {
		out = append([]Code{p}, out...)
	}
	return out
}

// IsAncestorOf reports whether c is a strict ancestor of d.
func (c Code) IsAncestorOf(d Code) bool {
	for _, a := range d.Ancestors() {
		if a == c {
			return true
		}
	}
	return false
}

// Less orders codes by body.
func (c Code) Less(d Code) bool { return c.Body() < d.Body() }

// Nature is the side on which an account's balance is normally carried.
type Nature int

// Natures.
const (
	Debit Nature = iota
	Credit
)

func (n Nature) String() string {
	if n == Debit {
		return "devedora"
	}
	return "credora"
}

// GroupNature returns the nature of a Cosif group: assets (1, 2), debit
// memorandum accounts (3) and expenses (8) are debit; liabilities (4, 5),
// equity (6), revenues (7) and credit memorandum accounts (9) are credit.
func GroupNature(group int) Nature {
	switch group {
	case 1, 2, 3, 8:
		return Debit
	}
	return Credit
}
