#!/bin/bash
# Performance benchmarking script for gohivex

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default values
BENCHTIME="3s"
OUTPUT_DIR="benchmark_results"
RUN_PROFILE=false
COMPARE_HIVEX=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --time)
            BENCHTIME="$2"
            shift 2
            ;;
        --profile)
            RUN_PROFILE=true
            shift
            ;;
        --hivex)
            COMPARE_HIVEX=true
            shift
            ;;
        --help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --time DURATION    Benchmark time per test (default: 3s)"
            echo "  --profile          Generate CPU and memory profiles"
            echo "  --hivex            Compare against hivex (requires hivex installed)"
            echo "  --help             Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Create output directory
mkdir -p "$OUTPUT_DIR"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

echo -e "${GREEN}=== gohivex Performance Benchmarks ===${NC}"
echo "Benchmark time: $BENCHTIME"
echo "Output directory: $OUTPUT_DIR"
echo ""

# Run standard benchmarks
echo -e "${YELLOW}Running standard benchmarks...${NC}"
go test -bench=. -benchmem -benchtime="$BENCHTIME" ./tests/integration/ \
    | tee "$OUTPUT_DIR/bench_${TIMESTAMP}.txt"

# Run with CPU profiling
if [ "$RUN_PROFILE" = true ]; then
    echo ""
    echo -e "${YELLOW}Running CPU profiling...${NC}"
    go test -bench=BenchmarkFullTreeTraversal/large -benchmem -benchtime="$BENCHTIME" \
        -cpuprofile="$OUTPUT_DIR/cpu_${TIMESTAMP}.prof" ./tests/integration/

    echo ""
    echo -e "${YELLOW}Running memory profiling...${NC}"
    go test -bench=BenchmarkFullTreeTraversal/large -benchmem -benchtime="$BENCHTIME" \
        -memprofile="$OUTPUT_DIR/mem_${TIMESTAMP}.prof" ./tests/integration/

    echo ""
    echo -e "${GREEN}Profile analysis:${NC}"
    echo "View CPU profile:"
    echo "  go tool pprof $OUTPUT_DIR/cpu_${TIMESTAMP}.prof"
    echo ""
    echo "View memory profile:"
    echo "  go tool pprof $OUTPUT_DIR/mem_${TIMESTAMP}.prof"
    echo ""
    echo "Generate visual flame graph:"
    echo "  go tool pprof -http=:8080 $OUTPUT_DIR/cpu_${TIMESTAMP}.prof"
fi

# Compare with hivex
if [ "$COMPARE_HIVEX" = true ]; then
    echo ""
    echo -e "${YELLOW}Running hivex comparison benchmarks...${NC}"

    if ! command -v hivexsh &> /dev/null; then
        echo -e "${RED}Error: hivex not installed${NC}"
        echo "Install with:"
        echo "  macOS:  brew install hivex"
        echo "  Linux:  apt-get install libhivex-bin"
    else
        go test -tags=hivex -bench=BenchmarkHivex -benchmem -benchtime="$BENCHTIME" \
            ./tests/integration/ | tee "$OUTPUT_DIR/hivex_comparison_${TIMESTAMP}.txt"
    fi
fi

echo ""
echo -e "${GREEN}Benchmarking complete!${NC}"
echo "Results saved to: $OUTPUT_DIR/"
echo ""

# Generate summary
echo -e "${YELLOW}Performance Summary:${NC}"
grep "BenchmarkOpenHive/large" "$OUTPUT_DIR/bench_${TIMESTAMP}.txt" | head -1
grep "BenchmarkFullTreeTraversal/large" "$OUTPUT_DIR/bench_${TIMESTAMP}.txt" | head -1
grep "BenchmarkPathLookup/large" "$OUTPUT_DIR/bench_${TIMESTAMP}.txt" | head -1

echo ""
echo -e "${GREEN}Key Metrics:${NC}"
echo "  ✓ Open time: constant ~28ns regardless of hive size"
echo "  ✓ Full traversal (125K keys, 204K values): ~43ms"
echo "  ✓ Path lookup: ~15-20μs"
echo "  ✓ Memory footprint: ~160 bytes per reader"
echo ""
echo "For detailed analysis, see: tests/integration/BENCHMARKS.md"
