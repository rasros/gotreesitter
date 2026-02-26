package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type benchSample struct {
	NsPerOp     float64 `json:"ns_per_op,omitempty"`
	BytesPerOp  float64 `json:"bytes_per_op,omitempty"`
	AllocsPerOp float64 `json:"allocs_per_op,omitempty"`
	MBPerSec    float64 `json:"mb_per_sec,omitempty"`
}

type benchStats struct {
	Samples         int     `json:"samples"`
	MeanNsPerOp     float64 `json:"mean_ns_per_op,omitempty"`
	P95NsPerOp      float64 `json:"p95_ns_per_op,omitempty"`
	MeanBytesPerOp  float64 `json:"mean_bytes_per_op,omitempty"`
	MeanAllocsPerOp float64 `json:"mean_allocs_per_op,omitempty"`
	MeanMBPerSec    float64 `json:"mean_mb_per_sec,omitempty"`
}

type benchGroupResult struct {
	Name       string                `json:"name"`
	Regex      string                `json:"regex"`
	Package    string                `json:"package"`
	Benchmarks map[string]benchStats `json:"benchmarks"`
	RawOutput  string                `json:"raw_output_path"`
}

type matrixResult struct {
	GeneratedAtUTC string             `json:"generated_at_utc"`
	GoVersion      string             `json:"go_version"`
	Count          int                `json:"count"`
	StressFuncN    int                `json:"stress_func_count"`
	Groups         []benchGroupResult `json:"groups"`
}

type benchGroupConfig struct {
	Name     string
	Regex    string
	Package  string
	ExtraEnv map[string]string
}

var benchLineRe = regexp.MustCompile(`^Benchmark[^\s]+`)
var suffixNumRe = regexp.MustCompile(`-\d+$`)

func main() {
	var (
		count       int
		outPath     string
		mdPath      string
		rawDir      string
		stressFuncN int
		noStress    bool
	)

	flag.IntVar(&count, "count", 10, "go test benchmark count")
	flag.StringVar(&outPath, "out", "bench_out/matrix.json", "output JSON path")
	flag.StringVar(&mdPath, "markdown", "bench_out/matrix.md", "output markdown path")
	flag.StringVar(&rawDir, "raw-dir", "bench_out/raw", "directory for raw benchmark outputs")
	flag.IntVar(&stressFuncN, "stress-func-count", 5000, "value for GOT_BENCH_FUNC_COUNT in stress class")
	flag.BoolVar(&noStress, "no-stress", false, "skip stress class")
	flag.Parse()

	if count <= 0 {
		fatalf("--count must be > 0")
	}

	groups := []benchGroupConfig{
		{
			Name:    "editor_hot_path",
			Package: ".",
			Regex:   "^(BenchmarkGoParseIncrementalSingleByteEdit|BenchmarkGoParseIncrementalRandomSingleByteEdit|BenchmarkGoParseIncrementalNoEdit|BenchmarkHighlightIncremental|BenchmarkTaggerTagIncremental)$",
		},
		{
			Name:    "indexing_path",
			Package: ".",
			Regex:   "^(BenchmarkGoParseFull|BenchmarkGoParseFullDFA|BenchmarkQueryExecCompiled|BenchmarkTaggerTag)$",
		},
	}
	if !noStress {
		groups = append(groups, benchGroupConfig{
			Name:    "stress_path",
			Package: ".",
			Regex:   "^(BenchmarkGoParseFull|BenchmarkGoParseFullDFA)$",
			ExtraEnv: map[string]string{
				"GOT_BENCH_FUNC_COUNT": strconv.Itoa(stressFuncN),
			},
		})
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		fatalf("create JSON output dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(mdPath), 0o755); err != nil {
		fatalf("create markdown output dir: %v", err)
	}
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		fatalf("create raw output dir: %v", err)
	}

	results := make([]benchGroupResult, 0, len(groups))
	for _, group := range groups {
		res, err := runBenchGroup(group, count, rawDir)
		if err != nil {
			fatalf("group %s failed: %v", group.Name, err)
		}
		results = append(results, res)
	}

	matrix := matrixResult{
		GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339),
		GoVersion:      runtime.Version(),
		Count:          count,
		StressFuncN:    stressFuncN,
		Groups:         results,
	}
	if err := writeJSON(outPath, matrix); err != nil {
		fatalf("write JSON: %v", err)
	}
	if err := writeMarkdown(mdPath, matrix); err != nil {
		fatalf("write markdown: %v", err)
	}

	fmt.Printf("benchmatrix JSON: %s\n", outPath)
	fmt.Printf("benchmatrix markdown: %s\n", mdPath)
}

func runBenchGroup(group benchGroupConfig, count int, rawDir string) (benchGroupResult, error) {
	args := []string{
		"test",
		"-run", "^$",
		"-bench", group.Regex,
		"-benchmem",
		"-count", strconv.Itoa(count),
		group.Package,
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = "."
	cmd.Env = os.Environ()
	for k, v := range group.ExtraEnv {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return benchGroupResult{}, fmt.Errorf("go %s: %w\nstderr:\n%s\nstdout:\n%s", strings.Join(args, " "), err, stderr.String(), stdout.String())
	}

	rawPath := filepath.Join(rawDir, group.Name+".txt")
	if err := os.WriteFile(rawPath, stdout.Bytes(), 0o644); err != nil {
		return benchGroupResult{}, fmt.Errorf("write raw output %s: %w", rawPath, err)
	}

	parsed, err := parseBenchOutput(stdout.Bytes())
	if err != nil {
		return benchGroupResult{}, fmt.Errorf("parse output: %w", err)
	}
	aggregated := aggregateBenchSamples(parsed)
	return benchGroupResult{
		Name:       group.Name,
		Regex:      group.Regex,
		Package:    group.Package,
		Benchmarks: aggregated,
		RawOutput:  rawPath,
	}, nil
}

func parseBenchOutput(out []byte) (map[string][]benchSample, error) {
	samples := map[string][]benchSample{}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !benchLineRe.MatchString(line) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		name := suffixNumRe.ReplaceAllString(fields[0], "")
		sample := benchSample{}
		for i := 2; i+1 < len(fields); i += 2 {
			value, err := strconv.ParseFloat(strings.TrimSpace(fields[i]), 64)
			if err != nil {
				continue
			}
			unit := fields[i+1]
			switch unit {
			case "ns/op":
				sample.NsPerOp = value
			case "B/op":
				sample.BytesPerOp = value
			case "allocs/op":
				sample.AllocsPerOp = value
			case "MB/s":
				sample.MBPerSec = value
			}
		}
		samples[name] = append(samples[name], sample)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return samples, nil
}

func aggregateBenchSamples(raw map[string][]benchSample) map[string]benchStats {
	out := make(map[string]benchStats, len(raw))
	for name, runs := range raw {
		ns := make([]float64, 0, len(runs))
		bytesOp := make([]float64, 0, len(runs))
		allocs := make([]float64, 0, len(runs))
		mbs := make([]float64, 0, len(runs))
		for _, s := range runs {
			if s.NsPerOp > 0 {
				ns = append(ns, s.NsPerOp)
			}
			if s.BytesPerOp > 0 {
				bytesOp = append(bytesOp, s.BytesPerOp)
			}
			if s.AllocsPerOp > 0 {
				allocs = append(allocs, s.AllocsPerOp)
			}
			if s.MBPerSec > 0 {
				mbs = append(mbs, s.MBPerSec)
			}
		}
		out[name] = benchStats{
			Samples:         len(runs),
			MeanNsPerOp:     mean(ns),
			P95NsPerOp:      percentile(ns, 95),
			MeanBytesPerOp:  mean(bytesOp),
			MeanAllocsPerOp: mean(allocs),
			MeanMBPerSec:    mean(mbs),
		}
	}
	return out
}

func writeJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeMarkdown(path string, matrix matrixResult) error {
	var b strings.Builder
	b.WriteString("# Bench Matrix\n\n")
	b.WriteString(fmt.Sprintf("- generated: %s\n", matrix.GeneratedAtUTC))
	b.WriteString(fmt.Sprintf("- go: `%s`\n", matrix.GoVersion))
	b.WriteString(fmt.Sprintf("- count: `%d`\n", matrix.Count))
	b.WriteString(fmt.Sprintf("- stress func count: `%d`\n\n", matrix.StressFuncN))

	for _, group := range matrix.Groups {
		b.WriteString(fmt.Sprintf("## %s\n\n", group.Name))
		b.WriteString("| Benchmark | Samples | Mean ns/op | P95 ns/op | Mean B/op | Mean allocs/op | Mean MB/s |\n")
		b.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
		names := make([]string, 0, len(group.Benchmarks))
		for name := range group.Benchmarks {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			s := group.Benchmarks[name]
			b.WriteString(fmt.Sprintf("| `%s` | %d | %s | %s | %s | %s | %s |\n",
				name,
				s.Samples,
				fmtFloat(s.MeanNsPerOp),
				fmtFloat(s.P95NsPerOp),
				fmtFloat(s.MeanBytesPerOp),
				fmtFloat(s.MeanAllocsPerOp),
				fmtFloat(s.MeanMBPerSec),
			))
		}
		b.WriteString(fmt.Sprintf("\nraw: `%s`\n\n", group.RawOutput))
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	ys := make([]float64, len(xs))
	copy(ys, xs)
	sort.Float64s(ys)
	if len(ys) == 1 {
		return ys[0]
	}
	rank := (p / 100.0) * float64(len(ys)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return ys[lo]
	}
	w := rank - float64(lo)
	return ys[lo]*(1.0-w) + ys[hi]*w
}

func fmtFloat(v float64) string {
	if v == 0 {
		return ""
	}
	if v >= 1000 {
		return fmt.Sprintf("%.0f", v)
	}
	if v >= 10 {
		return fmt.Sprintf("%.2f", v)
	}
	return fmt.Sprintf("%.4f", v)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
