package doc3040

import (
	"errors"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/victorcondackdev/cadoc-bacen-go/bizday"
)

var update = flag.Bool("update", false, "rewrite golden files")

var d = bizday.MustParse

func TestBucketBoundaries(t *testing.T) {
	ref := d("2026-10-31")
	cases := map[string]Bucket{
		// Days to fall due, counted from the last day of the data-base.
		"2026-10-31": 110, "2026-11-05": 110, "2026-11-30": 110, "2026-12-01": 120, "2026-12-30": 120,
		"2026-12-31": 130, "2027-01-29": 130, "2027-01-30": 140, "2027-04-29": 140, "2027-04-30": 150,
		"2027-10-26": 150, "2027-10-27": 160, "2028-10-20": 160, "2028-10-21": 165, "2041-12-31": 190,
		// Overdue.
		"2026-10-30": 205, "2026-10-17": 205, "2026-10-16": 210, "2026-10-01": 210, "2026-09-30": 220,
		"2026-08-02": 230, "2026-08-01": 240, "2025-11-05": 270, "2025-11-04": 280, "2025-04-01": 290,
	}
	for due, want := range cases {
		if got := BucketFor(ref, d(due)); got != want {
			t.Errorf("%s: bucket %d (%s), want %d (%s)", due, got, got, want, want)
		}
	}
	// The instructions' example: due on the 5th of the next month = 5 days.
	if d("2026-10-31").DaysUntil(d("2026-11-05")) != 5 {
		t.Fatal("day counting")
	}
}

func TestWriteOffBuckets(t *testing.T) {
	ref := d("2026-10-31")
	for off, want := range map[string]Bucket{"2026-10-01": 310, "2025-10-31": 310, "2025-10-30": 320, "2022-10-31": 320, "2022-10-30": 330} {
		if got := WriteOffBucket(ref, d(off)); got != want {
			t.Errorf("write-off %s: %d, want %d", off, got, want)
		}
	}
}

func TestDocuments(t *testing.T) {
	if !ValidCPF("12345678909") || ValidCPF("12345678900") || ValidCPF("11111111111") {
		t.Fatal("CPF")
	}
	if !ValidCNPJ("11222333000181") || ValidCNPJ("11222333000182") {
		t.Fatal("CNPJ")
	}
}

func sample() (Header, []Client) {
	h := Header{CNPJ: "12345678", DataBase: "2026-10", Remessa: 1, Parte: 1, NomeResp: "Victor Condack", EmailResp: "scr@banco.example", TelResp: "2133334444"}
	monthly := func(first string, n int, amount Money) []Installment {
		var out []Installment
		f := d(first)
		for i := 0; i < n; i++ {
			out = append(out, Installment{Due: bizday.FirstOfMonth(f, i).AddDays(f.Day - 1), Amount: amount})
		}
		return out
	}
	return h, []Client{
		{Type: 2, Document: "11222333000181", Authorized: true, Size: 2, Since: d("2023-05-10"), Income: 3_600_000_00,
			Operations: []Operation{{Contract: "CG-2026-0042", Modality: "0216", CEP: "20760005", Rate: "21.90",
				Contracted: d("2026-04-15"), Value: 240_000_00, Provision: 3_100_00,
				Installments: monthly("2026-11-15", 18, 13_333_33)}}},
		{Type: 1, Document: "12345678909", Authorized: true, Size: 5, Since: d("2024-03-15"), Income: 8_500_00,
			Operations: []Operation{
				{Contract: "CP-7781", Modality: "0202", CEP: "26255000", Rate: "28.40", Contracted: d("2026-01-10"), Value: 15_000_00,
					Provision: 420_00, Special: []string{"1"}, Installments: monthly("2026-09-10", 16, 1_062_50)},
				{Contract: "CC-0001", Modality: "0101", CEP: "26255000", Rate: "143.20", Contracted: d("2024-06-01"), Value: 5_000_00,
					Provision: 4_800_00, WrittenOff: d("2025-08-20"), Installments: []Installment{{Due: d("2025-03-01"), Amount: 4_812_77}}},
			}},
	}
}

func TestBuildGolden(t *testing.T) {
	h, clients := sample()
	out, err := Build(h, clients)
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		_ = os.WriteFile("testdata/3040.xml", out, 0o644)
	}
	want, _ := os.ReadFile("testdata/3040.xml")
	if string(out) != string(want) {
		t.Fatalf("3040 differs:\n%s", out)
	}
	s := string(out)
	for _, frag := range []string{`TotalCli="2"`, `Cd="11222333"`, `IPOC="123456780216211222333CG-2026-0042"`, `DiaAtraso="51"`, `v210="1062.50"`, `v220="1062.50"`, `v320="4812.77"`} {
		if !strings.Contains(s, frag) {
			t.Errorf("missing %s", frag)
		}
	}
}

func TestBuildRejects(t *testing.T) {
	h, clients := sample()
	bad := append([]Client(nil), clients...)
	bad[1].Document = "12345678900"
	if _, err := Build(h, bad); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "CPF") {
		t.Fatalf("bad CPF: %v", err)
	}
	h2 := h
	h2.TelResp = "123"
	if _, err := Build(h2, clients); err == nil {
		t.Fatal("bad phone accepted")
	}
	dup := append([]Client(nil), clients...)
	dup[1].Operations = append(dup[1].Operations, dup[1].Operations[0])
	if _, err := Build(h, dup); err == nil || !strings.Contains(err.Error(), "repetido") {
		t.Fatalf("duplicate contract: %v", err)
	}
}
