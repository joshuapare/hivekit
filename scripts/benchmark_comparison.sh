#!/bin/bash
# Benchmark gohivex vs hivexregedit comparison

set -e

cd "$(dirname "$0")/../tests/benchmarks/comparison"

# Test cases to benchmark
TESTS=(
    "win2003-system/small"
    "win2003-system/medium"
    "win2012-system/small"
    "win2012-system/medium"
)

echo "==================================================================="
echo "gohivex vs hivexregedit Performance Comparison"
echo "==================================================================="
echo ""
echo "Running benchmarks (5 iterations each)..."
echo ""

# Temporary files for results
GOHIVEX_RESULTS=$(mktemp)
HIVEX_RESULTS=$(mktemp)

# Run gohivex benchmarks
echo "  Running gohivex benchmarks..."
go test -bench="BenchmarkComparison_Gohivex" -benchtime=5x -run=^$ 2>&1 | \
    grep "BenchmarkComparison_Gohivex" > "$GOHIVEX_RESULTS" || true

# Run hivexregedit benchmarks
echo "  Running hivexregedit benchmarks..."
go test -bench="BenchmarkComparison_Hivexregedit" -benchtime=5x -run=^$ 2>&1 | \
    grep "BenchmarkComparison_Hivexregedit" > "$HIVEX_RESULTS" || true

echo ""
echo "==================================================================="
printf "%-30s %12s %12s %10s\n" "Test" "gohivex" "hivexregedit" "Speedup"
echo "==================================================================="

# Parse results and calculate speedups
for test in "${TESTS[@]}"; do
    # Escape slashes for grep
    test_escaped=$(echo "$test" | sed 's/\//\\\//g')

    gohivex_line=$(grep "BenchmarkComparison_Gohivex/${test_escaped}" "$GOHIVEX_RESULTS" || echo "")
    hivex_line=$(grep "BenchmarkComparison_Hivexregedit/${test_escaped}" "$HIVEX_RESULTS" || echo "")

    if [ -n "$gohivex_line" ] && [ -n "$hivex_line" ]; then
        # Extract ns/op values (3rd column)
        gohivex_ns=$(echo "$gohivex_line" | awk '{print $3}')
        hivex_ns=$(echo "$hivex_line" | awk '{print $3}')

        # Calculate speedup
        speedup=$(echo "scale=2; $hivex_ns / $gohivex_ns" | bc)

        # Format times
        gohivex_ms=$(echo "scale=1; $gohivex_ns / 1000000" | bc)
        hivex_ms=$(echo "scale=1; $hivex_ns / 1000000" | bc)

        # Color code based on speedup
        if (( $(echo "$speedup >= 1.0" | bc -l) )); then
            status="✓"
        else
            status="✗"
        fi

        printf "%-30s %10s ms %10s ms %8.2fx %s\n" "$test" "$gohivex_ms" "$hivex_ms" "$speedup" "$status"
    else
        printf "%-30s %12s %12s %10s\n" "$test" "FAILED" "FAILED" "N/A"
    fi
done

echo "==================================================================="

# Cleanup
rm -f "$GOHIVEX_RESULTS" "$HIVEX_RESULTS"
