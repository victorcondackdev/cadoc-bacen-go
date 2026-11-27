// Package doc3040 builds the core of the SCR document 3040 (Sistema de
// Informações de Crédito): clients (<Cli>), operations (<Op>) and their
// maturity buckets (<Venc>), computing each installment's vencimento code
// from the data-base exactly as the BCB filling instructions define it.
//
// Scope: the header, Cli, Op and Venc attributes listed here. Guarantees
// (<Gar>), aggregated operations (<Agreg>), additional information (<Inf>)
// and the Resolução CMN 4.966 blocks (<ContInstFinRes4966>) are not
// generated; validate the final file with the BCB validator.
package doc3040

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/victorcondackdev/cadoc-bacen-go/balancete"
	"github.com/victorcondackdev/cadoc-bacen-go/bizday"
)

// Money is an amount in centavos.
type Money = balancete.Money

// Bucket is a <Venc> attribute code.
type Bucket int

// Bucket describes the vencimento codes of the 3040 instructions.
var bucketNames = map[Bucket]string{
	110: "a vencer até 30 dias", 120: "a vencer de 31 a 60 dias", 130: "a vencer de 61 a 90 dias",
	140: "a vencer de 91 a 180 dias", 150: "a vencer de 181 a 360 dias", 160: "a vencer de 361 a 720 dias",
	165: "a vencer de 721 a 1080 dias", 170: "a vencer de 1081 a 1440 dias", 175: "a vencer de 1441 a 1800 dias",
	180: "a vencer de 1801 a 5400 dias", 190: "a vencer acima de 5400 dias", 199: "a vencer com prazo indeterminado",
	205: "vencidos de 1 a 14 dias", 210: "vencidos de 15 a 30 dias", 220: "vencidos de 31 a 60 dias",
	230: "vencidos de 61 a 90 dias", 240: "vencidos de 91 a 120 dias", 245: "vencidos de 121 a 150 dias",
	250: "vencidos de 151 a 180 dias", 255: "vencidos de 181 a 240 dias", 260: "vencidos de 241 a 300 dias",
	270: "vencidos de 301 a 360 dias", 280: "vencidos de 361 a 540 dias", 290: "vencidos acima de 540 dias",
	310: "baixados como prejuízo até 12 meses", 320: "baixados como prejuízo há mais de 12 e até 48 meses",
	330: "baixados como prejuízo há mais de 48 meses",
}

// String returns the bucket's description.
func (b Bucket) String() string { return bucketNames[b] }

var toFall = []struct {
	max int
	b   Bucket
}{{30, 110}, {60, 120}, {90, 130}, {180, 140}, {360, 150}, {720, 160}, {1080, 165}, {1440, 170}, {1800, 175}, {5400, 180}}

var overdue = []struct {
	max int
	b   Bucket
}{{14, 205}, {30, 210}, {60, 220}, {90, 230}, {120, 240}, {150, 245}, {180, 250}, {240, 255}, {300, 260}, {360, 270}, {540, 280}}

// LastDay returns the last day of the data-base month (AAAA-MM), from
// which days are counted.
func LastDay(dataBase string) (bizday.Date, error) {
	first, err := bizday.Parse(dataBase + "-01")
	if err != nil || len(dataBase) != 7 {
		return bizday.Date{}, fmt.Errorf("doc3040: data-base %q must be AAAA-MM", dataBase)
	}
	return bizday.LastOfMonth(first), nil
}

// BucketFor returns the vencimento code of an installment due on due,
// counting days from ref, the last day of the data-base month. An
// installment due on ref itself falls due in 0 days (bucket 110).
func BucketFor(ref, due bizday.Date) Bucket {
	d := ref.DaysUntil(due)
	if d >= 0 {
		for _, x := range toFall {
			if d <= x.max {
				return x.b
			}
		}
		return 190
	}
	late := -d
	for _, x := range overdue {
		if late <= x.max {
			return x.b
		}
	}
	return 290
}

// WriteOffBucket returns 310 (written off up to 12 months before ref),
// 320 (more than 12 and up to 48 months) or 330 (more than 48 months).
func WriteOffBucket(ref, writtenOff bizday.Date) Bucket {
	after := func(months int) bool {
		limit := bizday.FirstOfMonth(writtenOff, months)
		last := bizday.LastOfMonth(limit).Day
		day := writtenOff.Day
		if day > last {
			day = last
		}
		limit = limit.AddDays(day - 1)
		return ref.After(limit)
	}
	switch {
	case !after(12):
		return 310
	case !after(48):
		return 320
	}
	return 330
}

// Installment is an unpaid amount of an operation.
type Installment struct {
	Due    bizday.Date
	Amount Money
}

// Operation is a credit operation (<Op>).
type Operation struct {
	Contract     string // Contrt, 1–40 characters
	Modality     string // Mod, 4 digits (e.g. 0202 payroll loan, 0401 vehicle financing)
	Origin       string // OrigemRec (default 0101, free resources)
	Index        string // Indx (default 11, prefixed)
	IndexPct     string // PercIndx (default 0)
	Currency     string // VarCamb (default 790, real)
	CEP          string // 8 digits
	Rate         string // TaxEft, annual %, e.g. "24.50"
	Contracted   bizday.Date
	Value        Money  // VlrContr
	Nature       string // NatuOp (default 01, originated)
	Provision    Money  // ProvConsttd
	Special      []string
	Installments []Installment
	// WrittenOff marks the operation as loss (prejuízo) since that date;
	// the outstanding balance then goes to buckets 310–330.
	WrittenOff bizday.Date
}

// Client is a borrower (<Cli>).
type Client struct {
	Type       int    // 1 PF (CPF), 2 PJ (CNPJ)
	Document   string // CPF 11 digits or full CNPJ 14 digits
	Authorized bool   // Autorzc
	Size       int    // PorteCli
	Since      bizday.Date
	Income     Money // FatAnual: annual revenue (PJ) or monthly income (PF)
	Operations []Operation
}

// Code returns Cd: the CPF, or the CNPJ root.
func (c Client) Code() string {
	if c.Type == 2 && len(c.Document) == 14 {
		return c.Document[:8]
	}
	return c.Document
}

// Header is the <Doc3040> element.
type Header struct {
	CNPJ      string // institution CNPJ root, 8 digits
	DataBase  string // AAAA-MM
	Remessa   int
	Parte     int
	NomeResp  string
	EmailResp string
	TelResp   string // DD + 8 digits
}

// ErrInvalid wraps validation failures.
var ErrInvalid = errors.New("doc3040: invalid")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

func digits(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for i := 0; i < n; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ValidCPF checks a CPF's two check digits.
func ValidCPF(s string) bool {
	if !digits(s, 11) || strings.Count(s, s[:1]) == 11 {
		return false
	}
	for n := 9; n <= 10; n++ {
		sum := 0
		for i := 0; i < n; i++ {
			sum += int(s[i]-'0') * (n + 1 - i)
		}
		d := sum * 10 % 11 % 10
		if d != int(s[n]-'0') {
			return false
		}
	}
	return true
}

// ValidCNPJ checks a numeric CNPJ's two check digits.
func ValidCNPJ(s string) bool {
	if !digits(s, 14) || strings.Count(s, s[:1]) == 14 {
		return false
	}
	w := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	for n := 12; n <= 13; n++ {
		sum := 0
		for i := 0; i < n; i++ {
			sum += int(s[i]-'0') * w[len(w)-n+i]
		}
		d := sum % 11
		if d < 2 {
			d = 0
		} else {
			d = 11 - d
		}
		if d != int(s[n]-'0') {
			return false
		}
	}
	return true
}

// Venc is the bucketed outstanding balance of an operation.
type Venc map[Bucket]Money

// Total returns the sum of all buckets.
func (v Venc) Total() Money {
	var t Money
	for _, m := range v {
		t += m
	}
	return t
}

// Buckets computes the <Venc> of an operation at the data-base.
func Buckets(op Operation, ref bizday.Date) Venc {
	v := Venc{}
	if !op.WrittenOff.IsZero() {
		var t Money
		for _, i := range op.Installments {
			t += i.Amount
		}
		v[WriteOffBucket(ref, op.WrittenOff)] = t
		return v
	}
	for _, i := range op.Installments {
		v[BucketFor(ref, i.Due)] += i.Amount
	}
	return v
}

// DaysLate returns DiaAtraso: days overdue of the oldest installment due
// before ref, or 0.
func DaysLate(op Operation, ref bizday.Date) int {
	worst := 0
	for _, i := range op.Installments {
		if d := i.Due.DaysUntil(ref); d > worst {
			worst = d
		}
	}
	return worst
}

// IPOC builds the operation identifier: institution CNPJ root, modality,
// client type, client code and contract.
func IPOC(cnpj string, c Client, op Operation) string {
	return cnpj + op.Modality + fmt.Sprint(c.Type) + c.Code() + op.Contract
}

func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// Build validates the clients and renders the document.
func Build(h Header, clients []Client) ([]byte, error) {
	if !digits(h.CNPJ, 8) {
		return nil, invalid("CNPJ da instituição deve ser a raiz de 8 dígitos")
	}
	ref, err := LastDay(h.DataBase)
	if err != nil {
		return nil, invalid("%v", err)
	}
	if h.Remessa < 1 || h.Parte < 1 {
		return nil, invalid("Remessa e Parte começam em 1")
	}
	if !digits(h.TelResp, 10) || h.NomeResp == "" || !strings.Contains(h.EmailResp, "@") {
		return nil, invalid("responsável: nome, e-mail e telefone (DD + 8 dígitos) são obrigatórios")
	}
	type key struct {
		tp int
		cd string
	}
	exposed := map[key]bool{}
	contracts := map[string]bool{}
	sorted := append([]Client(nil), clients...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Type != sorted[j].Type {
			return sorted[i].Type < sorted[j].Type
		}
		return sorted[i].Code() < sorted[j].Code()
	})
	var body bytes.Buffer
	for _, c := range sorted {
		switch {
		case c.Type == 1 && !ValidCPF(c.Document):
			return nil, invalid("CPF %s inválido", c.Document)
		case c.Type == 2 && !ValidCNPJ(c.Document):
			return nil, invalid("CNPJ %s inválido", c.Document)
		case c.Type != 1 && c.Type != 2:
			return nil, invalid("tipo de cliente %d não suportado (1 PF, 2 PJ)", c.Type)
		case c.Size < 0 || (c.Type == 1 && c.Size > 8) || (c.Type == 2 && c.Size > 4):
			return nil, invalid("PorteCli %d inválido para o tipo %d", c.Size, c.Type)
		case c.Size == 0 && c.Income > 100:
			return nil, invalid("PorteCli 0 só é permitido com FatAnual até R$ 1,00")
		}
		aut := "N"
		if c.Authorized {
			aut = "S"
		}
		fmt.Fprintf(&body, `  <Cli Tp="%d" Cd="%s" Autorzc="%s" PorteCli="%d" IniRelactCli="%s" FatAnual="%s">`+"\n",
			c.Type, c.Code(), aut, c.Size, c.Since, c.Income.Decimal())
		for _, op := range c.Operations {
			if !digits(op.Modality, 4) {
				return nil, invalid("modalidade %q deve ter 4 dígitos", op.Modality)
			}
			if op.Contract == "" || len(op.Contract) > 40 {
				return nil, invalid("contrato deve ter de 1 a 40 caracteres")
			}
			ck := fmt.Sprintf("%d/%s/%s/%s", c.Type, c.Code(), op.Modality, op.Contract)
			if contracts[ck] {
				return nil, invalid("contrato %s repetido para o cliente e a modalidade", op.Contract)
			}
			contracts[ck] = true
			if len(op.Installments) == 0 {
				return nil, invalid("operação %s sem parcelas em aberto", op.Contract)
			}
			if !digits(op.CEP, 8) {
				return nil, invalid("CEP da operação %s deve ter 8 dígitos", op.Contract)
			}
			v := Buckets(op, ref)
			if v.Total() > 0 {
				exposed[key{c.Type, c.Code()}] = true
			}
			last := op.Installments[0].Due
			for _, i := range op.Installments {
				if i.Due.After(last) {
					last = i.Due
				}
			}
			fmt.Fprintf(&body, `    <Op IPOC="%s" Contrt="%s" Mod="%s" OrigemRec="%s" Indx="%s" PercIndx="%s" VarCamb="%s" CEP="%s" TaxEft="%s" DtContr="%s" VlrContr="%s" NatuOp="%s" DtVencOp="%s" ProvConsttd="%s"`,
				IPOC(h.CNPJ, c, op), op.Contract, op.Modality, or(op.Origin, "0101"), or(op.Index, "11"), or(op.IndexPct, "0"),
				or(op.Currency, "790"), op.CEP, op.Rate, op.Contracted, op.Value.Decimal(), or(op.Nature, "01"), last, op.Provision.Decimal())
			if late := DaysLate(op, ref); late > 0 {
				fmt.Fprintf(&body, ` DiaAtraso="%d"`, late)
			}
			if len(op.Special) > 0 {
				fmt.Fprintf(&body, ` CaracEspecial="%s"`, strings.Join(op.Special, ";"))
			}
			body.WriteString(">\n      <Venc")
			buckets := make([]int, 0, len(v))
			for b := range v {
				buckets = append(buckets, int(b))
			}
			sort.Ints(buckets)
			for _, b := range buckets {
				fmt.Fprintf(&body, ` v%d="%s"`, b, v[Bucket(b)].Decimal())
			}
			body.WriteString("/>\n    </Op>\n")
		}
		body.WriteString("  </Cli>\n")
	}
	var out bytes.Buffer
	out.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&out, `<Doc3040 CNPJ="%s" DtBase="%s" Remessa="%d" Parte="%d" TpArq="F" NomeResp="%s" EmailResp="%s" TelResp="%s" TotalCli="%d">`+"\n",
		h.CNPJ, h.DataBase, h.Remessa, h.Parte, xmlEscape(h.NomeResp), xmlEscape(h.EmailResp), h.TelResp, len(exposed))
	out.Write(body.Bytes())
	out.WriteString("</Doc3040>\n")
	return out.Bytes(), nil
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}
