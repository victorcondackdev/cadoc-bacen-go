// Package example holds the sample month of a small digital bank used by
// tests, the demo and the .NET parity vectors.
package example

import (
	"github.com/victorcondackdev/cadoc-bacen-go/balancete"
	"github.com/victorcondackdev/cadoc-bacen-go/cosif"
)

func c(s string) cosif.Code { return cosif.MustParse(s) }

func pair(debit, credit string, amount balancete.Money) []balancete.Entry {
	return []balancete.Entry{{Account: c(debit), Debit: amount}, {Account: c(credit), Credit: amount}}
}

// Entries returns the month's journal: capital, deposits, loans, provision,
// income, fees, expenses, Pix in transit and a guarantee in memorandum.
func Entries() []balancete.Entry {
	var e []balancete.Entry
	add := func(d, cr string, v balancete.Money) { e = append(e, pair(d, cr, v)...) }
	add("113000000", "611000000", 800_000_00) // capital paid in to free reserves
	add("619000000", "611000000", 200_000_00) // capital subscribed, not yet paid
	add("113000000", "411000000", 500_000_00) // demand deposits
	add("113000000", "412000000", 120_000_00) // savings deposits
	add("161100000", "113000000", 300_000_00) // personal loans
	add("161200000", "113000000", 150_000_00) // payroll loans
	add("820000000", "161900000", 13_500_00)  // expected loss provision
	add("161100000", "711000000", 9_870_45)   // interest accrued
	add("161200000", "711000000", 2_310_10)   // interest accrued
	add("113000000", "720000000", 3_455_90)   // fees received
	add("830000000", "113000000", 41_220_37)  // administrative expenses
	add("810000000", "412000000", 610_00)     // savings interest
	add("191000000", "491000000", 2_000_00)   // Pix in transit
	add("830000000", "492000000", 1_234_56)   // taxes payable
	add("111000000", "113000000", 5_000_00)   // cash withdrawal to the vault
	add("310000000", "910000000", 50_000_00)  // guarantees received
	return e
}

// Balancete returns the sample month's trial balance on the sample chart.
func Balancete() *balancete.Balancete {
	b, err := balancete.FromEntries(cosif.SampleChart(), Entries())
	if err != nil {
		panic(err)
	}
	return b
}
