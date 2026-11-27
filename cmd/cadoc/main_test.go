package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"demo"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "prazo de entrega: 2026-11-18 (quarta-feira)") {
		t.Fatalf("demo:\n%s", out.String())
	}
	dir := t.TempDir()
	bal := filepath.Join(dir, "saldos.csv")
	os.WriteFile(bal, []byte("# saldos\n1.1.3.00.00.00-3;D;1000.00\n6.1.1.00.00.00-4;C;1000\n"), 0o644)
	out.Reset()
	if err := run([]string{"4010", "-cnpj", "12345678", "-database", "2026-10", "-balances", bal}, &out); err != nil {
		t.Fatal(err)
	}
	xml := filepath.Join(dir, "4010.xml")
	os.WriteFile(xml, out.Bytes(), 0o644)
	out.Reset()
	if err := run([]string{"validate", xml}, &out); err != nil || !strings.Contains(out.String(), "ok: documento 4010") {
		t.Fatalf("validate: %v %s", err, out.String())
	}
	out.Reset()
	if err := run([]string{"diff", xml, xml}, &out); err != nil || !strings.Contains(out.String(), "sem diferenças") {
		t.Fatalf("diff: %v %s", err, out.String())
	}
	out.Reset()
	if err := run([]string{"venc", "2026-10", "2026-11-05"}, &out); err != nil || !strings.HasPrefix(out.String(), "v110") {
		t.Fatalf("venc: %v %s", err, out.String())
	}
	out.Reset()
	if err := run([]string{"prazo", "4016", "2026-12"}, &out); err != nil || !strings.Contains(out.String(), "2027-01-29") {
		t.Fatalf("prazo: %v %s", err, out.String())
	}
	if err := run([]string{"nope"}, &out); err == nil {
		t.Fatal("unknown command accepted")
	}
}
