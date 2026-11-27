package cosif

import (
	_ "embed"
	"strings"
)

//go:embed sample_chart.csv
var sampleChart string

// SampleChart returns a small illustrative chart for a digital bank. Codes
// carry valid check digits and follow the Cosif level structure, but the
// set and the names are an example, not the official elenco de contas:
// load the real chart of your institution with ReadChart.
func SampleChart() *Chart {
	c, err := ReadChart(strings.NewReader(sampleChart))
	if err != nil {
		panic(err)
	}
	return c
}
