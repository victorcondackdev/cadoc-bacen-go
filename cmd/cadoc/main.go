// Command cadoc generates and checks BCB regulatory documents.
//
//	cadoc demo                                    sample month: trial balance, 4010, 4016, deadlines
//	cadoc 4010 -cnpj 12345678 -database 2026-10 -balances saldos.csv [-chart plano.csv]
//	cadoc 4016 -cnpj 12345678 -database 2026-12 -balances saldos.csv -lucros 6.4.1.00.00.00 -prejuizos 6.4.9.00.00.00
//	cadoc validate [-chart plano.csv] arquivo.xml
//	cadoc diff anterior.xml novo.xml
//	cadoc prazo 4010|4016 AAAA-MM
//	cadoc venc AAAA-MM AAAA-MM-DD                 SCR 3040 vencimento code of an installment
//
// Balances CSV: code;D|C;amount (e.g. 1.1.3.00.00.00-3;D;927235.53), leaf
// accounts only. Chart CSV: code;name[;R] (R = contra account). Without
// -chart the illustrative sample chart is used.
package main

import (
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/victorcondackdev/cadoc-bacen-go/balancete"
	"github.com/victorcondackdev/cadoc-bacen-go/bizday"
	"github.com/victorcondackdev/cadoc-bacen-go/cosif"
	"github.com/victorcondackdev/cadoc-bacen-go/doc3040"
	"github.com/victorcondackdev/cadoc-bacen-go/doc4010"
	"github.com/victorcondackdev/cadoc-bacen-go/internal/example"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "cadoc:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("uso: cadoc demo|4010|4016|validate|diff|prazo|venc ...")
	}
	switch args[0] {
	case "demo":
		return demo(out)
	case "4010", "4016":
		return build(args[0], args[1:], out)
	case "validate":
		return validate(args[1:], out)
	case "diff":
		return diff(args[1:], out)
	case "prazo":
		if len(args) != 3 {
			return errors.New("uso: cadoc prazo 4010|4016 AAAA-MM")
		}
		d, err := doc4010.Deadline(args[1], args[2], nil)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s data-base %s: entrega até %s (%s)\n", args[1], args[2], d, weekday(d))
		return nil
	case "venc":
		if len(args) != 3 {
			return errors.New("uso: cadoc venc AAAA-MM AAAA-MM-DD")
		}
		ref, err := doc3040.LastDay(args[1])
		if err != nil {
			return err
		}
		due, err := bizday.Parse(args[2])
		if err != nil {
			return err
		}
		b := doc3040.BucketFor(ref, due)
		fmt.Fprintf(out, "v%d — %s (%d dias a partir de %s)\n", b, b, ref.DaysUntil(due), ref)
		return nil
	}
	return fmt.Errorf("comando desconhecido %q", args[0])
}

var dias = [...]string{"domingo", "segunda-feira", "terça-feira", "quarta-feira", "quinta-feira", "sexta-feira", "sábado"}

func weekday(d bizday.Date) string { return dias[d.Weekday()] }

func loadChart(path string) (*cosif.Chart, error) {
	if path == "" {
		return cosif.SampleChart(), nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return cosif.ReadChart(f)
}

func loadBalances(chart *cosif.Chart, path string) (*balancete.Balancete, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = ';'
	r.Comment = '#'
	leaves := map[cosif.Code]balancete.Money{}
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) != 3 {
			return nil, fmt.Errorf("linha %v: esperado código;D|C;valor", rec)
		}
		code, err := cosif.Parse(strings.TrimSpace(rec[0]))
		if err != nil {
			return nil, err
		}
		v, err := parseMoney(strings.TrimSpace(rec[2]))
		if err != nil {
			return nil, err
		}
		switch strings.ToUpper(strings.TrimSpace(rec[1])) {
		case "D":
		case "C":
			v = -v
		default:
			return nil, fmt.Errorf("linha %v: natureza deve ser D ou C", rec)
		}
		leaves[code] += v
	}
	return balancete.FromLeaves(chart, leaves)
}

func parseMoney(s string) (balancete.Money, error) {
	whole, frac, _ := strings.Cut(s, ".")
	if len(frac) > 2 {
		return 0, fmt.Errorf("valor %q com mais de 2 casas", s)
	}
	frac += strings.Repeat("0", 2-len(frac))
	w, err1 := strconv.ParseInt(whole, 10, 64)
	f, err2 := strconv.ParseInt(frac, 10, 64)
	if err1 != nil || err2 != nil || w < 0 {
		return 0, fmt.Errorf("valor %q inválido", s)
	}
	return balancete.Money(w*100 + f), nil
}

func build(code string, args []string, out io.Writer) error {
	fs := flag.NewFlagSet(code, flag.ContinueOnError)
	cnpj := fs.String("cnpj", "", "raiz do CNPJ (8 dígitos)")
	base := fs.String("database", "", "data-base AAAA-MM")
	remessa := fs.String("remessa", "I", "I (inclusão) ou S (substituição)")
	chartPath := fs.String("chart", "", "plano de contas CSV (padrão: exemplo)")
	balances := fs.String("balances", "", "saldos das contas analíticas CSV")
	profit := fs.String("lucros", "", "conta do grupo 6 que recebe o lucro ao encerrar o resultado (4016)")
	loss := fs.String("prejuizos", "", "conta do grupo 6 que recebe o prejuízo ao encerrar o resultado (4016)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	chart, err := loadChart(*chartPath)
	if err != nil {
		return err
	}
	b, err := loadBalances(chart, *balances)
	if err != nil {
		return err
	}
	if *profit != "" || *loss != "" {
		p, err := cosif.Parse(*profit)
		if err != nil {
			return fmt.Errorf("-lucros: %w", err)
		}
		l, err := cosif.Parse(*loss)
		if err != nil {
			return fmt.Errorf("-prejuizos: %w", err)
		}
		if b, err = b.CloseResult(p, l); err != nil {
			return err
		}
	}
	for _, i := range b.Check() {
		fmt.Fprintln(os.Stderr, i)
	}
	d, err := doc4010.Build(code, *cnpj, *base, *remessa, b, doc4010.Options{})
	if err != nil {
		return err
	}
	_, err = out.Write(d.Marshal())
	return err
}

func readDoc(path string) (*doc4010.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return doc4010.Parse(data)
}

func validate(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	chartPath := fs.String("chart", "", "plano de contas CSV (padrão: exemplo)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("uso: cadoc validate [-chart plano.csv] arquivo.xml")
	}
	chart, err := loadChart(*chartPath)
	if err != nil {
		return err
	}
	d, err := readDoc(fs.Arg(0))
	if err != nil {
		return err
	}
	problems := d.Validate(chart)
	for _, p := range problems {
		fmt.Fprintln(out, "erro:", p)
	}
	if len(problems) > 0 {
		return fmt.Errorf("%d problema(s)", len(problems))
	}
	fmt.Fprintf(out, "ok: documento %s, data-base %s, %d contas\n", d.Code, d.DataBase, len(d.Lines))
	return nil
}

func diff(args []string, out io.Writer) error {
	if len(args) != 2 {
		return errors.New("uso: cadoc diff anterior.xml novo.xml")
	}
	a, err := readDoc(args[0])
	if err != nil {
		return err
	}
	b, err := readDoc(args[1])
	if err != nil {
		return err
	}
	ch := doc4010.Diff(a, b)
	for _, c := range ch {
		fmt.Fprintf(out, "%-18s %-9s %14s -> %14s\n", c.Code, c.Kind, c.Old.Decimal(), c.New.Decimal())
	}
	if len(ch) == 0 {
		fmt.Fprintln(out, "sem diferenças")
	}
	return nil
}

func demo(out io.Writer) error {
	b := example.Balancete()
	fmt.Fprintln(out, "== balancete de outubro/2026 (plano de exemplo) ==")
	for _, r := range b.Rows(false) {
		indent := strings.Repeat("  ", r.Account.Code.Level()-1)
		name := indent + r.Account.Name
		name += strings.Repeat(" ", max(0, 44-utf8.RuneCountInString(name)))
		fmt.Fprintf(out, "%s %s %14s %s\n", r.Account.Code, name, r.Saldo.Decimal(), map[cosif.Nature]string{cosif.Debit: "D", cosif.Credit: "C"}[r.Side])
	}
	issues := b.Check()
	fmt.Fprintf(out, "verificações: %d problema(s)\n\n", len(issues))
	d, err := doc4010.Build(doc4010.Balancete, "12345678", "2026-10", doc4010.Inclusion, b, doc4010.Options{})
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "== 4010 ==")
	out.Write(d.Marshal())
	dl, _ := doc4010.Deadline(doc4010.Balancete, "2026-10", nil)
	fmt.Fprintf(out, "prazo de entrega: %s (%s)\n\n", dl, weekday(dl))
	closed, err := b.CloseResult(cosif.MustParse("641000000"), cosif.MustParse("649000000"))
	if err != nil {
		return err
	}
	d16, err := doc4010.Build(doc4010.Balanco, "12345678", "2026-12", doc4010.Inclusion, closed, doc4010.Options{})
	if err != nil {
		return err
	}
	dl16, _ := doc4010.Deadline(doc4010.Balanco, "2026-12", nil)
	fmt.Fprintf(out, "== 4016 (prejuízo do período encerrado em %s): %d contas, prazo %s (%s) ==\n", cosif.MustParse("649000000"), len(d16.Lines), dl16, weekday(dl16))
	return nil
}
