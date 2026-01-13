#!/bin/bash
set -e

# Script to run regtext benchmarks and save results for tracking
# Usage: ./scripts/bench_regtext.sh [output_file] [label]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_FILE="${1:-$REPO_ROOT/bench_results.csv}"
LABEL="${2:-$(date +%Y-%m-%d_%H-%M-%S)}"

# Create CSV header if file doesn't exist
if [ ! -f "$OUTPUT_FILE" ]; then
    echo "timestamp,label,benchmark,ns_per_op,mb_per_sec,bytes_per_op,allocs_per_op,file_size_mb" > "$OUTPUT_FILE"
fi

echo "Running regtext benchmarks..."
echo "Output file: $OUTPUT_FILE"
echo "Label: $LABEL"
echo ""

# Run benchmarks and capture output
cd "$REPO_ROOT/internal/regtext"

BENCH_OUTPUT=$(go test -bench='BenchmarkParseReg_(XP|2003|Win8|2012|Generated|StringHeavy|BinaryHeavy|DWORDHeavy|HeavyEscaping|WithDeletions)' \
    -benchmem -run='^$' -benchtime=200ms 2>&1 | grep "^Benchmark")

# Parse and append results
TIMESTAMP=$(date -u +"%Y-%m-%d %H:%M:%S")

# Use process substitution to avoid subshell issues with while loop
while IFS= read -r line; do
    # Extract benchmark name and metrics using awk for robust parsing
    # Format: BenchmarkName-10    count    ns/op    MB/s    bytes/op    allocs/op
    bench_name=$(echo "$line" | awk '{print $1}')
    ns_per_op=$(echo "$line" | awk '{print $3}')
    mb_per_sec=$(echo "$line" | awk '{print $5}')
    bytes_per_op=$(echo "$line" | awk '{print $7}')
    allocs_per_op=$(echo "$line" | awk '{print $9}')

    # Skip if we didn't extract valid data
    if [ -z "$bench_name" ] || [ -z "$ns_per_op" ]; then
        continue
    fi

    # Calculate file size from benchmark name
    file_size_mb="N/A"
    case "$bench_name" in
        *"Generated_1KB"*) file_size_mb="0.001";;
        *"Generated_10KB"*) file_size_mb="0.01";;
        *"Generated_100KB"*) file_size_mb="0.1";;
        *"Generated_1MB"*) file_size_mb="1";;
        *"Generated_10MB"*) file_size_mb="10";;
        *"XP_System"*) file_size_mb="9.1";;
        *"XP_Software"*) file_size_mb="3.1";;
        *"2003_System"*) file_size_mb="2.6";;
        *"2003_Software"*) file_size_mb="18";;
        *"Win8_System"*) file_size_mb="9.1";;
        *"Win8_Software"*) file_size_mb="30";;
        *"Win8CP_Software"*) file_size_mb="48";;
        *"2012_System"*) file_size_mb="12";;
        *"2012_Software"*) file_size_mb="43";;
        *"StringHeavy"*) file_size_mb="1";;
        *"BinaryHeavy"*) file_size_mb="1";;
        *"DWORDHeavy"*) file_size_mb="1";;
        *"HeavyEscaping"*) file_size_mb="1";;
        *"WithDeletions"*) file_size_mb="1";;
    esac

    echo "$TIMESTAMP,$LABEL,$bench_name,$ns_per_op,$mb_per_sec,$bytes_per_op,$allocs_per_op,$file_size_mb" >> "$OUTPUT_FILE"
done <<< "$BENCH_OUTPUT"

echo ""
echo "✓ Results saved to $OUTPUT_FILE"
echo ""
echo "Recent results for $LABEL:"
grep "$LABEL" "$OUTPUT_FILE" | head -10
echo ""
echo "To view all results: cat $OUTPUT_FILE"
echo "To compare with previous run: ./scripts/compare_bench.sh"
