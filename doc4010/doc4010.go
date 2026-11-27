// Package doc4010 generates, parses and validates the Cosif accounting
// documents 4010 (Balancete Patrimonial Analítico, monthly) and 4016
// (Balanço Patrimonial Analítico, semiannual) in the XML layout published
// by the BCB, and computes their filing deadlines.
package doc4010

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/victorcondackdev/cadoc-bacen-go/balancete"
	"github.com/victorcondackdev/cadoc-bacen-go/bizday"
	"github.com/victorcondackdev/cadoc-bacen-go/cosif"
)

// Document codes.
const (
	Balancete = "4010"
	Balanco   = "4016"
)

// Submission types (tipoRemessa).
const (
	Inclusion    = "I" // first submission for a data-base
	Substitution = "S" // replaces an accepted document
)

// Line is one <conta>.
type Line struct {
	Code  cosif.Code
	Saldo balancete.Money // absolute value
}

// Document is a 4010 or 4016 document.
type Document struct {
	Code     string // "4010" or "4016"
	CNPJ     string // 8-digit root
	DataBase string // AAAA-MM
	Remessa  string // "I" or "S"
	Lines    []Line
}

// Options for Build.
type Options struct {
	// IncludeZero also lists accounts with zero balance (optional in the
	// layout).
	IncludeZero bool
}

var (
	reCNPJ     = regexp.MustCompile(`^[0-9]{8}$`)
	reDataBase = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)
	reSaldo    = regexp.MustCompile(`^\d{1,16}\.\d{2}$`)
	reConta    = regexp.MustCompile(`^[0-9]{10}$`)
)

// ErrInvalid wraps every validation failure.
var ErrInvalid = errors.New("doc4010: invalid document")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Build creates a document from a trial balance. Balances contrary to an
// account's nature are rejected, since the layout has no sign. A 4016 is
// semiannual (data-base June or December) and must not carry revenue or
// expense balances: close the result into equity first
// (Balancete.CloseResult).
func Build(code, cnpj, dataBase, remessa string, b *balancete.Balancete, o Options) (*Document, error) {
	d := &Document{Code: code, CNPJ: cnpj, DataBase: dataBase, Remessa: remessa}
	if err := d.checkHeader(); err != nil {
		return nil, err
	}
	if code == Balanco && b.HasResult() {
		return nil, invalid("o 4016 não pode ter saldos nos grupos 7 e 8; encerre o resultado no patrimônio líquido")
	}
	for _, issue := range b.Check() {
		if issue.Severity == "erro" {
			return nil, invalid("balancete inconsistente: %s", issue.Message)
		}
	}
	rows := b.Rows(o.IncludeZero)
	for _, r := range rows {
		// The layout carries no sign: an analytical balance on the wrong
		// side of its account's nature cannot be represented and must be
		// reclassified. Synthetic accounts are reported as absolute values.
		if r.Inverted && b.Chart().IsLeaf(r.Account.Code) {
			return nil, invalid("%s %s com saldo %s, contrário à natureza %s: reclassifique antes de gerar o documento", r.Account.Code, r.Account.Name, r.Side, r.Account.Nature())
		}
	}
	for _, r := range rows {
		if code == Balanco && (r.Account.Code.Group() == 7 || r.Account.Code.Group() == 8) {
			continue
		}
		d.Lines = append(d.Lines, Line{Code: r.Account.Code, Saldo: r.Saldo})
	}
	return d, nil
}

func (d *Document) checkHeader() error {
	switch {
	case d.Code != Balancete && d.Code != Balanco:
		return invalid("codigoDocumento deve ser 4010 ou 4016")
	case !reCNPJ.MatchString(d.CNPJ):
		return invalid("cnpj deve ter 8 dígitos (raiz)")
	case !reDataBase.MatchString(d.DataBase):
		return invalid("dataBase deve estar no formato AAAA-MM")
	case d.Remessa != Inclusion && d.Remessa != Substitution:
		return invalid("tipoRemessa deve ser I ou S")
	case d.Code == Balanco && !strings.HasSuffix(d.DataBase, "-06") && !strings.HasSuffix(d.DataBase, "-12"):
		return invalid("o 4016 é semestral: data-base de junho ou dezembro")
	}
	return nil
}

// Marshal renders the document exactly in the published layout.
func (d *Document) Marshal() []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&b, `<documento codigoDocumento="%s" cnpj="%s" dataBase="%s" tipoRemessa="%s">`+"\n", d.Code, d.CNPJ, d.DataBase, d.Remessa)
	b.WriteString("  <contas>\n")
	for _, l := range d.Lines {
		fmt.Fprintf(&b, `    <conta codigoConta="%s" saldo="%s"/>`+"\n", l.Code.Compact(), l.Saldo.Decimal())
	}
	b.WriteString("  </contas>\n</documento>\n")
	return b.Bytes()
}

type xmlDoc struct {
	XMLName  xml.Name `xml:"documento"`
	Codigo   string   `xml:"codigoDocumento,attr"`
	CNPJ     string   `xml:"cnpj,attr"`
	DataBase string   `xml:"dataBase,attr"`
	Remessa  string   `xml:"tipoRemessa,attr"`
	Contas   []struct {
		Codigo string `xml:"codigoConta,attr"`
		Saldo  string `xml:"saldo,attr"`
	} `xml:"contas>conta"`
}

// Parse reads and validates a document's syntax: header patterns, ten-digit
// account codes with valid check digits, saldo pattern, no duplicates.
func Parse(data []byte) (*Document, error) {
	var x xmlDoc
	if err := xml.Unmarshal(data, &x); err != nil {
		return nil, invalid("XML: %v", err)
	}
	d := &Document{Code: x.Codigo, CNPJ: x.CNPJ, DataBase: x.DataBase, Remessa: x.Remessa}
	if err := d.checkHeader(); err != nil {
		return nil, err
	}
	seen := map[cosif.Code]bool{}
	for i, c := range x.Contas {
		if !reConta.MatchString(c.Codigo) {
			return nil, invalid("conta %d: codigoConta deve ter 10 dígitos", i+1)
		}
		code, err := cosif.Parse(c.Codigo)
		if err != nil {
			return nil, invalid("conta %s: %v", c.Codigo, err)
		}
		if !reSaldo.MatchString(c.Saldo) {
			return nil, invalid("conta %s: saldo %q fora do padrão \\d{1,16}\\.\\d{2}", c.Codigo, c.Saldo)
		}
		if seen[code] {
			return nil, invalid("conta %s repetida", c.Codigo)
		}
		seen[code] = true
		whole, frac, _ := strings.Cut(c.Saldo, ".")
		w, _ := strconv.ParseInt(whole, 10, 64)
		f, _ := strconv.ParseInt(frac, 10, 64)
		d.Lines = append(d.Lines, Line{Code: code, Saldo: balancete.Money(w*100 + f)})
	}
	return d, nil
}

// Validate checks the document against a chart: every account exists, no
// revenue or expense accounts in a 4016, and every synthetic account equals
// what its sub-accounts add up to. Since the layout carries no sign, the
// signs are rebuilt bottom-up: an analytical (leaf) account carries its
// nature's sign (contra accounts the opposite one), a synthetic account the
// sign of the sum of its children, and each synthetic is compared in
// absolute value. Absent accounts count as zero (zero balances are
// optional); a synthetic account whose children are all absent is taken as
// reported.
func (d *Document) Validate(chart *cosif.Chart) []string {
	var problems []string
	lines := map[cosif.Code]balancete.Money{}
	for _, l := range d.Lines {
		lines[l.Code] = l.Saldo
		if _, ok := chart.Account(l.Code); !ok {
			problems = append(problems, fmt.Sprintf("conta %s não existe no plano", l.Code))
		}
		if d.Code == Balanco && (l.Code.Group() == 7 || l.Code.Group() == 8) {
			problems = append(problems, fmt.Sprintf("conta de resultado %s no 4016", l.Code))
		}
	}
	natural := func(c cosif.Code, v balancete.Money) balancete.Money {
		a, ok := chart.Account(c)
		n := cosif.GroupNature(c.Group())
		if ok {
			n = a.Nature()
		}
		if n == cosif.Credit {
			return -v
		}
		return v
	}
	// Children present in the document, by parent.
	children := map[cosif.Code][]cosif.Code{}
	for c := range lines {
		if p, ok := c.Parent(); ok {
			children[p] = append(children[p], c)
		}
	}
	signedOf := map[cosif.Code]balancete.Money{}
	var compute func(c cosif.Code) balancete.Money
	compute = func(c cosif.Code) balancete.Money {
		if v, ok := signedOf[c]; ok {
			return v
		}
		kids := children[c]
		var v balancete.Money
		if len(kids) == 0 {
			v = natural(c, lines[c])
		} else {
			for _, k := range kids {
				v += compute(k)
			}
		}
		signedOf[c] = v
		return v
	}
	// The sign of a reported value comes from the reconstruction; the
	// magnitude is the reported one, so a wrong line is reported once, at
	// its parent, instead of at every ancestor.
	reportedSigned := func(c cosif.Code) balancete.Money {
		v := lines[c]
		s := compute(c)
		if s == 0 {
			s = natural(c, 1)
		}
		if s < 0 {
			return -v
		}
		return v
	}
	parents := make([]cosif.Code, 0, len(children))
	for p := range children {
		parents = append(parents, p)
	}
	sort.Slice(parents, func(i, j int) bool { return parents[i].Less(parents[j]) })
	for _, p := range parents {
		var sum balancete.Money
		for _, k := range children[p] {
			sum += reportedSigned(k)
		}
		if sum < 0 {
			sum = -sum
		}
		got, present := lines[p]
		if !present && sum != 0 {
			problems = append(problems, fmt.Sprintf("conta sintética %s ausente", p))
			continue
		}
		if sum != got {
			problems = append(problems, fmt.Sprintf("conta %s: saldo %s difere da soma das subcontas %s", p, got.Decimal(), sum.Decimal()))
		}
	}
	return problems
}

// Change is a difference between two submissions.
type Change struct {
	Code     cosif.Code
	Old, New balancete.Money
	Kind     string // "alterada", "incluída", "excluída"
}

// Diff lists what a substitution (S) changes relative to the previous
// submission.
func Diff(prev, next *Document) []Change {
	a := map[cosif.Code]balancete.Money{}
	for _, l := range prev.Lines {
		a[l.Code] = l.Saldo
	}
	b := map[cosif.Code]balancete.Money{}
	for _, l := range next.Lines {
		b[l.Code] = l.Saldo
	}
	var out []Change
	for c, v := range a {
		if w, ok := b[c]; !ok {
			out = append(out, Change{c, v, 0, "excluída"})
		} else if w != v {
			out = append(out, Change{c, v, w, "alterada"})
		}
	}
	for c, w := range b {
		if _, ok := a[c]; !ok {
			out = append(out, Change{c, 0, w, "incluída"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code.Less(out[j].Code) })
	return out
}

// Deadline returns the filing deadline: for 4010, the 18th of the month
// after the data-base or the next business day; for 4016, the last business
// day of the month after the data-base.
func Deadline(code, dataBase string, h bizday.Holidays) (bizday.Date, error) {
	if !reDataBase.MatchString(dataBase) {
		return bizday.Date{}, invalid("dataBase deve estar no formato AAAA-MM")
	}
	if h == nil {
		h = bizday.National{}
	}
	first, _ := bizday.Parse(dataBase + "-01")
	next := bizday.FirstOfMonth(first, 1)
	switch code {
	case Balancete:
		return bizday.NextBusinessDay(h, next.AddDays(17)), nil
	case Balanco:
		return bizday.PreviousBusinessDay(h, bizday.LastOfMonth(next)), nil
	}
	return bizday.Date{}, invalid("codigoDocumento deve ser 4010 ou 4016")
}
