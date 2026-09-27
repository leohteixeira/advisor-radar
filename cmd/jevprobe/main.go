// jevprobe scores the labeled message set through Jev and the keyword baseline.
//
//	go run ./cmd/jevprobe [-file internal/triage/testdata/messages.json] [-zdr]
//
// AI_GATEWAY_API_KEY comes from the process environment, or from .env when unset.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/leohteixeira/advisor-radar/internal/envfile"
	"github.com/leohteixeira/advisor-radar/internal/jev"
	"github.com/leohteixeira/advisor-radar/internal/triage"
)

type labeled struct {
	triage.Message
	Expected struct {
		Intent string `json:"intent"`
		Churn  bool   `json:"churn"`
		Human  bool   `json:"human"`
	} `json:"expected"`
}

type tally struct {
	intent, churn, human int
	latencies            []time.Duration
	cost                 float64
}

func main() {
	file := flag.String("file", "internal/triage/testdata/messages.json", "labeled messages")
	zdr := flag.Bool("zdr", true, "require zero data retention")
	threshold := flag.Float64("threshold", 0.5, "boolean threshold")
	flag.Parse()

	if err := envfile.Load(".env"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	key := os.Getenv("AI_GATEWAY_API_KEY")
	if key == "" {
		fmt.Fprintln(os.Stderr, "AI_GATEWAY_API_KEY is not set")
		os.Exit(1)
	}

	raw, err := os.ReadFile(*file)
	must(err)
	var set []labeled
	must(json.Unmarshal(raw, &set))

	opts := []jev.Option{}
	if *zdr {
		opts = append(opts, jev.WithZeroDataRetention())
	}
	jevC := triage.NewJevClassifier(jev.New(key, opts...))
	heur := triage.HeuristicClassifier{}

	var tj, th tally
	models := map[string]bool{}
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "id\texpected\tjev\tp\tfrust\tchurn\thuman\theuristic\tms")

	for _, l := range set {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		rj, err := jevC.Classify(ctx, l.Message)
		cancel()
		if err != nil {
			fmt.Fprintf(tw, "%s\t%s\tERROR: %v\n", l.ID, l.Expected.Intent, err)
			continue
		}
		rh, _ := heur.Classify(context.Background(), l.Message)
		models[rj.ModelVersion] = true

		score(&tj, rj, l, *threshold)
		score(&th, rh, l, *threshold)

		fmt.Fprintf(tw, "%s\t%s\t%s%s\t%.2f\t%.2f\t%.2f\t%.2f\t%s%s\t%d\n",
			l.ID, l.Expected.Intent,
			rj.Intent, mark(string(rj.Intent) == l.Expected.Intent), rj.IntentProb,
			rj.Frustration, rj.ChurnRisk, rj.WantsHuman,
			rh.Intent, mark(string(rh.Intent) == l.Expected.Intent),
			rj.Latency.Milliseconds())
	}
	_ = tw.Flush()

	n := len(set)
	fmt.Printf("\nmodel(s): %v\n", keys(models))
	fmt.Printf("%-11s intent %2d/%d  churn %2d/%d  human %2d/%d\n", "jev", tj.intent, n, tj.churn, n, tj.human, n)
	fmt.Printf("%-11s intent %2d/%d  churn %2d/%d  human %2d/%d\n", "heuristic", th.intent, n, th.churn, n, th.human, n)
	if len(tj.latencies) > 0 {
		slices.Sort(tj.latencies)
		fmt.Printf("jev latency p50 %v  p95 %v  total cost USD %.6f\n",
			pct(tj.latencies, 50), pct(tj.latencies, 95), tj.cost)
	}
}

func score(t *tally, r triage.Result, l labeled, thr float64) {
	if string(r.Intent) == l.Expected.Intent {
		t.intent++
	}
	if (r.ChurnRisk >= thr) == l.Expected.Churn {
		t.churn++
	}
	if (r.WantsHuman >= thr) == l.Expected.Human {
		t.human++
	}
	t.latencies = append(t.latencies, r.Latency)
	if c, err := strconv.ParseFloat(r.CostUSD, 64); err == nil {
		t.cost += c
	}
}

func pct(sorted []time.Duration, p int) time.Duration {
	i := (len(sorted)*p + 99) / 100
	return sorted[max(0, min(len(sorted)-1, i-1))].Round(time.Millisecond)
}

func mark(ok bool) string {
	if ok {
		return ""
	}
	return " (x)"
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
