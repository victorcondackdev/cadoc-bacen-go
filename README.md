# cadoc-bacen-go

[![CI](https://github.com/victorcondackdev/cadoc-bacen-go/actions/workflows/ci.yml/badge.svg)](https://github.com/victorcondackdev/cadoc-bacen-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/victorcondackdev/cadoc-bacen-go.svg)](https://pkg.go.dev/github.com/victorcondackdev/cadoc-bacen-go)

BCB regulatory documents (CADOC) for Go: Cosif account codes with check
digits, a trial balance that rolls leaf balances up the six Cosif levels,
documents **4010** (Balancete Patrimonial Analítico) and **4016** (Balanço
Patrimonial Analítico) in the published XML layout with their filing
deadlines, and the core of the SCR document **3040** with the maturity
buckets (`v110`…`v330`) computed from the data-base.

The last mile of a ledger — the one that turns balances into what the
Central Bank receives — is usually a spreadsheet. This makes it code:
[ledger-go](https://github.com/victorcondackdev/ledger-go) balances in, a
validated 4010 out, with a diff for substitutions.

Standard library only. Go 1.22+.

```
bizday/      civil dates and the national banking calendar (deadlines)
cosif/       account codes (1.1.0.00.00.00-2 / 1100000002), check digit, levels, nature, charts
balancete/   trial balance: roll-up, double-entry and memorandum checks, contra accounts, result closing
doc4010/     4010 / 4016 XML: build, parse, validate against a chart, diff, deadlines
doc3040/     SCR 3040 core: Cli, Op, Venc buckets, DiaAtraso, IPOC, CPF/CNPJ checks
cmd/cadoc/   CLI
```

## Quick start

```sh
go run ./cmd/cadoc demo
```

```
== balancete de outubro/2026 (plano de exemplo) ==
1.0.0.00.00.00-9 ATIVO                                         1382916.08 D
1.1.0.00.00.00-2   Disponibilidades                             932235.53 D
...
1.6.1.90.00.00-5       (-) Provisão para perdas esperadas        13500.00 C
...
verificações: 0 problema(s)

== 4010 ==
<?xml version="1.0" encoding="UTF-8"?>
<documento codigoDocumento="4010" cnpj="12345678" dataBase="2026-10" tipoRemessa="I">
  <contas>
    <conta codigoConta="1000000009" saldo="1382916.08"/>
    ...
prazo de entrega: 2026-11-18 (quarta-feira)
```

```sh
cadoc 4010 -cnpj 12345678 -database 2026-10 -balances saldos.csv -chart plano.csv > 4010.xml
cadoc 4016 -cnpj 12345678 -database 2026-12 -balances saldos.csv -lucros 6.4.1.00.00.00 -prejuizos 6.4.9.00.00.00
cadoc validate -chart plano.csv 4010.xml
cadoc diff 4010-I.xml 4010-S.xml
cadoc prazo 4010 2026-03          # 4010 data-base 2026-03: entrega até 2026-04-20 (segunda-feira)
cadoc venc 2026-10 2026-11-05     # v110 — a vencer até 30 dias (5 dias a partir de 2026-10-31)
```

## Cosif codes

Since January 2025 a Cosif code has six levels plus a check digit:
`9.0.9.61.00.00-1`, written `9096100001` in XML. The check digit is the
complement to the next ten of the body's digits weighted 1, 7, 3 from the
left. The tests pin it to codes published by the BCB (the 4010 XML example
and Instruções Normativas 429/2023, 493/2024 and 640/2025).

Each group has a nature: 1, 2, 3 and 8 are debit; 4, 5, 6, 7 and 9 credit.
Accounts flagged as contra (`R` in the chart CSV, e.g. "(-) Provisão") carry
the opposite nature and are subtracted when synthetic balances are checked.

The bundled chart is **illustrative** (a small digital bank with valid
codes); load your institution's chart with `cosif.ReadChart`.

## 4010 / 4016

| Rule | |
|---|---|
| Root `documento` with `codigoDocumento`, `cnpj` (8-digit root), `dataBase` (AAAA-MM), `tipoRemessa` (I/S) | `doc4010.Build`, `Parse` |
| `codigoConta` 10 digits with valid check digit; `saldo` matching `\d{1,16}\.\d{2}`; no duplicates | `Parse` |
| All accounts at every level; zero balances optional | `Build` (`Options.IncludeZero`) |
| Synthetic balance = sum of sub-accounts, contra accounts subtracted | `Document.Validate` |
| Trial balance closes (debits = credits) and groups 3 and 9 offset | `Balancete.Check` |
| The layout has no sign: a balance contrary to its account's nature must be reclassified first | `Build` |
| 4016 semiannual (June, December), without groups 7 and 8: result closed into equity (profit and loss accounts) | `Build`, `Balancete.CloseResult` |
| 4010 due on the 18th of the following month (next business day); 4016 on the last business day of the following month | `Deadline` |
| A substitution (S) replaces an accepted document; `Diff` lists what changes | `Diff` |

Source: *Balancete e Balanço Patrimonial Analítico — Documentos 4010/4016,
Instruções de Preenchimento* (BCB/Desig). The instructions show `dataBase` as
AAAA-MM in the text and example; validate against the official XSD before
sending through STA.

## SCR 3040

`doc3040.BucketFor` implements the vencimento table of the 3040 filling
instructions: days are counted from the last day of the data-base month (an
installment due on the 5th of the next month falls due in 5 days, bucket
110); overdue amounts go to 205–290 by days late; written-off operations to
310/320/330 by months since the write-off. `Build` renders the header, `Cli`,
`Op` (IPOC, modality, rate, dates, value, provision, `DiaAtraso`,
`CaracEspecial`) and `Venc`, validating CPF/CNPJ, sizes and duplicate
contracts, and counting `TotalCli` by CNPJ root.

It is the core, not the whole document: guarantees (`Gar`), aggregated
operations (`Agreg`), `Inf` and the Resolução CMN 4.966 blocks are not
generated. Run the result through the BCB validator.

## .NET

[cadoc-bacen-dotnet](https://github.com/victorcondackdev/cadoc-bacen-dotnet)
is a port whose XML output is byte-identical to this one.

## License

MIT
