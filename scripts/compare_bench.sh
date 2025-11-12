#!/bin/bash

# Script to compare benchmark results from different runs
# Usage: ./scripts/compare_bench.sh [csv_file] [label1] [label2]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CSV_FILE="${1:-$REPO_ROOT/bench_results.csv}"

if [ ! -f "$CSV_FILE" ]; then
    echo "Error: CSV file not found: $CSV_FILE"
    echo "Run ./scripts/bench_regtext.sh first to generate benchmark data"
    exit 1
fi

# Get unique labels
LABELS=($(tail -n +2 "$CSV_FILE" | cut -d',' -f2 | sort -u))

if [ ${#LABELS[@]} -eq 0 ]; then
    echo "Error: No benchmark data found in $CSV_FILE"
    exit 1
fi

echo "==================================="
echo "Regtext Parser Benchmark Comparison"
echo "==================================="
echo ""

# If specific labels provided, use those
if [ -n "$2" ] && [ -n "$3" ]; then
    LABEL1="$2"
    LABEL2="$3"
else
    # Otherwise use the two most recent
    if [ ${#LABELS[@]} -lt 2 ]; then
        echo "Available labels:"
        printf '%s\n' "${LABELS[@]}"
        echo ""
        echo "Only one label found. Showing results for: ${LABELS[-1]}"
        LABEL1="${LABELS[-1]}"
        LABEL2=""
    else
        LABEL1="${LABELS[-2]}"
        LABEL2="${LABELS[-1]}"
        echo "Comparing: $LABEL1 (baseline) vs $LABEL2 (current)"
    fi
fi

echo ""
echo "Key Benchmarks Summary:"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
printf "%-35s | %12s | %12s | %10s\n" "Benchmark" "Throughput" "Memory/op" "Allocs/op"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Function to format bytes
format_bytes() {
    local bytes=$1
    if [ $bytes -lt 1024 ]; then
        echo "${bytes}B"
    elif [ $bytes -lt 1048576 ]; then
        echo "$((bytes / 1024))KB"
    else
        echo "$((bytes / 1048576))MB"
    fi
}

# Key benchmarks to show
KEY_BENCHMARKS=(
    "BenchmarkParseReg_XP_System"
    "BenchmarkParseReg_2003_Software"
    "BenchmarkParseReg_Win8_Software"
    "BenchmarkParseReg_2012_Software"
    "BenchmarkParseReg_Generated_1KB"
    "BenchmarkParseReg_Generated_1MB"
    "BenchmarkParseReg_Generated_10MB"
    "BenchmarkParseReg_StringHeavy"
    "BenchmarkParseReg_BinaryHeavy"
    "BenchmarkParseReg_DWORDHeavy"
)

for bench in "${KEY_BENCHMARKS[@]}"; do
    # Get latest result for this benchmark
    result=$(grep ",$bench-" "$CSV_FILE" | tail -1)

    if [ -n "$result" ]; then
        mb_per_sec=$(echo "$result" | cut -d',' -f5)
        bytes_per_op=$(echo "$result" | cut -d',' -f6)
        allocs_per_op=$(echo "$result" | cut -d',' -f7)

        # Format benchmark name (remove prefix and -10 suffix)
        short_name=$(echo "$bench" | sed 's/BenchmarkParseReg_//' | sed 's/-10$//')

        # Format bytes
        mem_formatted=$(format_bytes $bytes_per_op)

        printf "%-35s | %9s MB/s | %12s | %10s\n" \
            "$short_name" "$mb_per_sec" "$mem_formatted" "$allocs_per_op"
    fi
done

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# If we have two labels to compare
if [ -n "$LABEL2" ] && [ "$LABEL1" != "$LABEL2" ]; then
    echo ""
    echo "Performance Changes ($LABEL1 → $LABEL2):"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    printf "%-35s | %15s | %15s\n" "Benchmark" "Throughput Δ" "Memory Δ"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    for bench in "${KEY_BENCHMARKS[@]}"; do
        result1=$(grep ",$LABEL1,$bench-" "$CSV_FILE" | tail -1)
        result2=$(grep ",$LABEL2,$bench-" "$CSV_FILE" | tail -1)

        if [ -n "$result1" ] && [ -n "$result2" ]; then
            mb1=$(echo "$result1" | cut -d',' -f5)
            mb2=$(echo "$result2" | cut -d',' -f5)
            mem1=$(echo "$result1" | cut -d',' -f6)
            mem2=$(echo "$result2" | cut -d',' -f6)

            # Calculate percentage changes
            mb_change=$(echo "scale=2; ($mb2 - $mb1) / $mb1 * 100" | bc)
            mem_change=$(echo "scale=2; ($mem2 - $mem1) / $mem1 * 100" | bc)

            # Format with + or - sign
            if (( $(echo "$mb_change > 0" | bc -l) )); then
                mb_display="+${mb_change}%"
            else
                mb_display="${mb_change}%"
            fi

            if (( $(echo "$mem_change > 0" | bc -l) )); then
                mem_display="+${mem_change}%"
            else
                mem_display="${mem_change}%"
            fi

            short_name=$(echo "$bench" | sed 's/BenchmarkParseReg_//' | sed 's/-10$//')
            printf "%-35s | %15s | %15s\n" "$short_name" "$mb_display" "$mem_display"
        fi
    done

    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
fi

echo ""
echo "Available labels in $CSV_FILE:"
printf '%s\n' "${LABELS[@]}" | sed 's/^/  - /'
echo ""
echo "To compare specific runs: $0 [csv_file] [label1] [label2]"
echo "To view raw data: cat $CSV_FILE"
