package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// BenchmarkResult represents a parsed benchmark result.
type BenchmarkResult struct {
	Name        string
	Operation   string
	HiveSize    string
	Impl        string // "gohivex" or "hivex"
	Iterations  int
	NsPerOp     float64
	BytesPerOp  int64
	AllocsPerOp int64
}

// ComparisonResult represents a comparison between gohivex and hivex.
type ComparisonResult struct {
	Operation     string
	HiveSize      string
	GohivexNs     float64
	HivexNs       float64
	Speedup       float64
	GohivexMem    int64
	HivexMem      int64
	GohivexAllocs int64
	HivexAllocs   int64
	GohivexOnly   bool
}

var (
	inputFile = flag.String(
		"input",
		"",
		"Input file with benchmark output (stdin if not specified)",
	)
	outputFile = flag.String("output", "", "Output markdown file (stdout if not specified)")
	quiet      = flag.Bool("quiet", false, "Suppress progress output")
)

func main() {
	flag.Parse()

	// Read benchmark output
	var scanner *bufio.Scanner
	var inputF *os.File
	if *inputFile != "" {
		f, err := os.Open(*inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening input file: %v\n", err)
			os.Exit(1)
		}
		inputF = f
		scanner = bufio.NewScanner(f)
	} else {
		scanner = bufio.NewScanner(os.Stdin)
	}

	// Parse benchmarks
	results := parseBenchmarks(scanner)

	if !*quiet {
		fmt.Fprintf(os.Stderr, "Parsed %d benchmark results\n", len(results))
	}

	// Generate comparisons
	comparisons := generateComparisons(results)

	if !*quiet {
		fmt.Fprintf(os.Stderr, "Generated %d comparisons\n", len(comparisons))
	}

	// Generate markdown report
	report := generateMarkdownReport(comparisons, results)

	// Write output
	if *outputFile != "" {
		err := os.WriteFile(*outputFile, []byte(report), 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing output file: %v\n", err)
			if inputF != nil {
				inputF.Close()
			}
			os.Exit(1)
		}
		if !*quiet {
			fmt.Fprintf(os.Stderr, "Report written to %s\n", *outputFile)
		}
	} else {
		fmt.Fprint(os.Stdout, report)
	}

	// Close input file if opened
	if inputF != nil {
		inputF.Close()
	}
}

func parseBenchmarks(scanner *bufio.Scanner) []BenchmarkResult {
	var results []BenchmarkResult

	// Regex to parse benchmark output lines
	// BenchmarkOpen/gohivex/small-8    10000    12450 ns/op    4096 B/op    8 allocs/op
	benchmarkRegex := regexp.MustCompile(
		`^(Benchmark\S+)\s+(\d+)\s+([\d.]+)\s+ns/op(?:\s+([\d.]+)\s+(?:B|MB)/op)?(?:\s+([\d.]+)\s+allocs/op)?`,
	)

	for scanner.Scan() {
		line := scanner.Text()

		// Try to parse as JSON (from -json flag)
		var testEvent map[string]any
		if err := json.Unmarshal([]byte(line), &testEvent); err == nil {
			if output, ok := testEvent["Output"].(string); ok {
				line = output
			}
		}

		// Parse benchmark line
		matches := benchmarkRegex.FindStringSubmatch(strings.TrimSpace(line))
		if matches == nil {
			continue
		}

		name := matches[1]
		iterations, _ := strconv.Atoi(matches[2])
		nsPerOp, _ := strconv.ParseFloat(matches[3], 64)

		var bytesPerOp int64
		var allocsPerOp int64

		if matches[4] != "" {
			bytesPerOp, _ = strconv.ParseInt(matches[4], 10, 64)
		}
		if matches[5] != "" {
			allocsPerOp, _ = strconv.ParseInt(matches[5], 10, 64)
		}

		// Parse name to extract operation, impl, and hive size
		// Format: Benchmark<Operation>/<impl>/<size>-<procs>
		// Or: Benchmark<Operation>_<variant>/<impl>/<size>-<procs>
		parts := strings.Split(name, "/")
		if len(parts) < 2 {
			// Handle benchmarks without implementation split (gohivex-only)
			// e.g., BenchmarkOpenBytes_Gohivex/small-8
			operation, hiveSize := parseGohivexOnlyBenchmark(name)
			if operation != "" {
				results = append(results, BenchmarkResult{
					Name:        name,
					Operation:   operation,
					HiveSize:    hiveSize,
					Impl:        "gohivex",
					Iterations:  iterations,
					NsPerOp:     nsPerOp,
					BytesPerOp:  bytesPerOp,
					AllocsPerOp: allocsPerOp,
				})
			}
			continue
		}

		// Extract operation from first part
		operation := strings.TrimPrefix(parts[0], "Benchmark")

		// Extract implementation (gohivex or hivex)
		impl := parts[1]

		// Extract hive size from last part (remove -N suffix)
		hiveSize := ""
		if len(parts) >= 3 {
			lastPart := parts[len(parts)-1]
			dashIdx := strings.LastIndex(lastPart, "-")
			if dashIdx > 0 {
				hiveSize = lastPart[:dashIdx]
			} else {
				hiveSize = lastPart
			}
		}

		results = append(results, BenchmarkResult{
			Name:        name,
			Operation:   operation,
			HiveSize:    hiveSize,
			Impl:        impl,
			Iterations:  iterations,
			NsPerOp:     nsPerOp,
			BytesPerOp:  bytesPerOp,
			AllocsPerOp: allocsPerOp,
		})
	}

	return results
}

func parseGohivexOnlyBenchmark(name string) (string, string) {
	// Handle benchmarks like: BenchmarkOpenBytes_Gohivex/small-8
	// Or: BenchmarkOpenBytes_ZeroCopy/zerocopy/small-8

	parts := strings.Split(name, "/")
	if len(parts) == 0 {
		return "", ""
	}

	var operation, hiveSize string

	// Extract operation from first part
	firstPart := strings.TrimPrefix(parts[0], "Benchmark")
	// Remove _Gohivex, _ZeroCopy, etc.
	if idx := strings.Index(firstPart, "_"); idx > 0 {
		operation = firstPart[:idx]
	} else {
		operation = firstPart
	}

	// Extract hive size from last part
	if len(parts) > 0 {
		lastPart := parts[len(parts)-1]
		dashIdx := strings.LastIndex(lastPart, "-")
		if dashIdx > 0 {
			hiveSize = lastPart[:dashIdx]
		} else {
			hiveSize = lastPart
		}
	}

	return operation, hiveSize
}

func generateComparisons(results []BenchmarkResult) []ComparisonResult {
	// Group results by operation and hive size
	type key struct {
		operation string
		hiveSize  string
	}

	grouped := make(map[key]map[string]BenchmarkResult)

	for _, result := range results {
		k := key{result.Operation, result.HiveSize}
		if grouped[k] == nil {
			grouped[k] = make(map[string]BenchmarkResult)
		}
		grouped[k][result.Impl] = result
	}

	// Generate comparisons
	var comparisons []ComparisonResult

	for k, impls := range grouped {
		gohivex, hasGohivex := impls["gohivex"]
		hivex, hasHivex := impls["hivex"]

		if hasGohivex && hasHivex {
			// Both implementations exist - compare them
			speedup := hivex.NsPerOp / gohivex.NsPerOp

			comparisons = append(comparisons, ComparisonResult{
				Operation:     k.operation,
				HiveSize:      k.hiveSize,
				GohivexNs:     gohivex.NsPerOp,
				HivexNs:       hivex.NsPerOp,
				Speedup:       speedup,
				GohivexMem:    gohivex.BytesPerOp,
				HivexMem:      hivex.BytesPerOp,
				GohivexAllocs: gohivex.AllocsPerOp,
				HivexAllocs:   hivex.AllocsPerOp,
				GohivexOnly:   false,
			})
		} else if hasGohivex {
			// Only gohivex implementation
			comparisons = append(comparisons, ComparisonResult{
				Operation:     k.operation,
				HiveSize:      k.hiveSize,
				GohivexNs:     gohivex.NsPerOp,
				GohivexMem:    gohivex.BytesPerOp,
				GohivexAllocs: gohivex.AllocsPerOp,
				GohivexOnly:   true,
			})
		}
	}

	// Sort by operation then hive size
	sort.Slice(comparisons, func(i, j int) bool {
		if comparisons[i].Operation != comparisons[j].Operation {
			return comparisons[i].Operation < comparisons[j].Operation
		}
		return comparisons[i].HiveSize < comparisons[j].HiveSize
	})

	return comparisons
}

func generateMarkdownReport(comparisons []ComparisonResult, _ []BenchmarkResult) string {
	var sb strings.Builder

	// Header
	sb.WriteString("# Benchmark Report\n\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

	// Summary statistics
	gohivexFaster := 0
	hivexFaster := 0
	gohivexOnly := 0
	totalSpeedup := 0.0

	for _, comp := range comparisons {
		if comp.GohivexOnly {
			gohivexOnly++
		} else {
			if comp.Speedup > 1.0 {
				gohivexFaster++
			} else if comp.Speedup < 1.0 {
				hivexFaster++
			}
			totalSpeedup += comp.Speedup
		}
	}

	comparableCount := len(comparisons) - gohivexOnly
	avgSpeedup := 0.0
	if comparableCount > 0 {
		avgSpeedup = totalSpeedup / float64(comparableCount)
	}

	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("- **Total benchmarks**: %d\n", len(comparisons)))
	sb.WriteString(fmt.Sprintf("- **Comparable** (both implementations): %d\n", comparableCount))
	sb.WriteString(
		fmt.Sprintf(
			"  - gohivex faster: %d (%.1f%%)\n",
			gohivexFaster,
			float64(gohivexFaster)/float64(comparableCount)*100,
		),
	)
	sb.WriteString(
		fmt.Sprintf(
			"  - hivex faster: %d (%.1f%%)\n",
			hivexFaster,
			float64(hivexFaster)/float64(comparableCount)*100,
		),
	)
	sb.WriteString(fmt.Sprintf("  - Average speedup: **%.2fx**\n", avgSpeedup))
	sb.WriteString(fmt.Sprintf("- **gohivex-only features**: %d\n", gohivexOnly))
	sb.WriteString("\n")

	// Detailed results table
	sb.WriteString("## Detailed Results\n\n")
	sb.WriteString(
		"| Operation | Hive | gohivex (ns/op) | hivex (ns/op) | Speedup | Memory (B/op) | Allocs |\n",
	)
	sb.WriteString(
		"|-----------|------|-----------------|---------------|---------|---------------|--------|\n",
	)

	for _, comp := range comparisons {
		if comp.GohivexOnly {
			// gohivex-only benchmark
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | *N/A* | *gohivex only* | %s | %s |\n",
				comp.Operation,
				comp.HiveSize,
				formatNumber(comp.GohivexNs),
				formatBytes(comp.GohivexMem),
				formatNumber(float64(comp.GohivexAllocs)),
			))
		} else {
			// Comparison benchmark
			indicator := "✓"
			speedupStyle := "**"
			if comp.Speedup < 1.0 {
				indicator = "✗"
				speedupStyle = ""
			}

			memIndicator := ""
			if comp.GohivexMem < comp.HivexMem {
				memIndicator = " ✓"
			} else if comp.GohivexMem > comp.HivexMem {
				memIndicator = " ✗"
			}

			allocIndicator := ""
			if comp.GohivexAllocs < comp.HivexAllocs {
				allocIndicator = " ✓"
			} else if comp.GohivexAllocs > comp.HivexAllocs {
				allocIndicator = " ✗"
			}

			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s%.2fx%s %s | %s vs %s%s | %s vs %s%s |\n",
				comp.Operation,
				comp.HiveSize,
				formatNumber(comp.GohivexNs),
				formatNumber(comp.HivexNs),
				speedupStyle,
				comp.Speedup,
				speedupStyle,
				indicator,
				formatBytes(comp.GohivexMem),
				formatBytes(comp.HivexMem),
				memIndicator,
				formatNumber(float64(comp.GohivexAllocs)),
				formatNumber(float64(comp.HivexAllocs)),
				allocIndicator,
			))
		}
	}

	sb.WriteString("\n")

	// Category summaries
	sb.WriteString("## Performance by Category\n\n")

	categories := categorizeOperations(comparisons)
	for category, comps := range categories {
		if len(comps) == 0 {
			continue
		}

		avgSpeed := 0.0
		count := 0
		for _, comp := range comps {
			if !comp.GohivexOnly {
				avgSpeed += comp.Speedup
				count++
			}
		}

		if count > 0 {
			avgSpeed /= float64(count)
			status := "✓"
			if avgSpeed < 1.0 {
				status = "✗"
			}
			sb.WriteString(fmt.Sprintf("- %s **%s**: %.2fx average speedup %s\n",
				status, category, avgSpeed, status))
		} else {
			sb.WriteString(fmt.Sprintf("- **%s**: gohivex-only features\n", category))
		}
	}

	sb.WriteString("\n")

	// Notes
	sb.WriteString("## Notes\n\n")
	sb.WriteString("- **Speedup > 1.0**: gohivex is faster ✓\n")
	sb.WriteString("- **Speedup < 1.0**: hivex is faster ✗\n")
	sb.WriteString("- **Memory comparison**: Lower is better\n")
	sb.WriteString("- **Allocations**: Fewer is better\n")
	sb.WriteString("- **gohivex-only**: Features not available in hivex\n")

	return sb.String()
}

func categorizeOperations(comparisons []ComparisonResult) map[string][]ComparisonResult {
	categories := map[string][]ComparisonResult{
		"Open/Close":       {},
		"Navigation":       {},
		"Metadata":         {},
		"Values":           {},
		"Typed Values":     {},
		"Introspection":    {},
		"gohivex Features": {},
	}

	for _, comp := range comparisons {
		op := strings.ToLower(comp.Operation)

		switch {
		case comp.GohivexOnly:
			categories["gohivex Features"] = append(categories["gohivex Features"], comp)
		case strings.Contains(op, "open") || strings.Contains(op, "close"):
			categories["Open/Close"] = append(categories["Open/Close"], comp)
		case strings.Contains(op, "root") || strings.Contains(op, "children") ||
			strings.Contains(op, "getchild") || strings.Contains(op, "walk"):
			categories["Navigation"] = append(categories["Navigation"], comp)
		case strings.Contains(op, "timestamp") || strings.Contains(op, "nrchildren") ||
			strings.Contains(op, "nrvalues") || strings.Contains(op, "stat") ||
			strings.Contains(op, "nodename") || strings.Contains(op, "detail"):
			categories["Metadata"] = append(categories["Metadata"], comp)
		case strings.Contains(op, "string") || strings.Contains(op, "dword") ||
			strings.Contains(op, "qword"):
			categories["Typed Values"] = append(categories["Typed Values"], comp)
		case strings.Contains(op, "value"):
			categories["Values"] = append(categories["Values"], comp)
		default:
			categories["Introspection"] = append(categories["Introspection"], comp)
		}
	}

	return categories
}

func formatNumber(n float64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.2fM", n/1000000)
	} else if n >= 1000 {
		return fmt.Sprintf("%.1fK", n/1000)
	}
	return fmt.Sprintf("%.0f", n)
}

func formatBytes(b int64) string {
	if b >= 1024*1024 {
		return fmt.Sprintf("%.2fMB", float64(b)/(1024*1024))
	} else if b >= 1024 {
		return fmt.Sprintf("%.1fKB", float64(b)/1024)
	}
	return fmt.Sprintf("%dB", b)
}
