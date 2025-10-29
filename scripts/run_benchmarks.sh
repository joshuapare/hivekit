#!/usr/bin/env bash
#
# run_benchmarks.sh - Run gohivex comparison benchmarks and generate reports
#
# Usage:
#   ./scripts/run_benchmarks.sh [options]
#
# Options:
#   --quick          Run quick benchmarks (1s benchtime, 1 count)
#   --output FILE    Output report to specific file
#   --benchtime T    Benchmark time per test (default: 5s)
#   --count N        Number of runs per benchmark (default: 5)
#   --bench PATTERN  Run specific benchmarks matching regex pattern
#   --category NAME  Run benchmarks by category (open, navigation, metadata, values, etc.)
#   --list           List available benchmark categories and exit
#   --help           Show this help message
#
# Examples:
#   # Run only Open benchmarks
#   ./scripts/run_benchmarks.sh --bench "BenchmarkOpen"
#
#   # Run navigation category
#   ./scripts/run_benchmarks.sh --category navigation
#
#   # Run benchmarks for medium hive only
#   ./scripts/run_benchmarks.sh --bench ".*medium"
#
#   # Run specific operation quickly
#   ./scripts/run_benchmarks.sh --quick --bench "BenchmarkNodeChildren"

set -euo pipefail

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Default settings
BENCHTIME="${BENCHTIME:-5s}"
COUNT="${COUNT:-5}"
OUTPUT_FILE=""
QUICK_MODE=false
BENCH_PATTERN="."  # Default: run all benchmarks

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Benchmark categories mapping
get_category_pattern() {
    local category="$1"
    case "$category" in
        open|opening)
            echo "Benchmark(Open|Close)"
            ;;
        navigation|nav)
            echo "Benchmark(Root|NodeChildren|NodeGetChild|FullTreeWalk)"
            ;;
        metadata|meta)
            echo "Benchmark(NodeTimestamp|NodeNrChildren|NodeNrValues|NodeName|StatKey|DetailKey|MetadataOnChildren)"
            ;;
        values|value)
            echo "Benchmark(NodeValues|NodeGetValue|ValueKey|ValueType|ValueValue|StatValue|AllValues)"
            ;;
        typed|typed-values)
            echo "Benchmark(ValueDword|ValueQword|ValueString)"
            ;;
        introspection|intro)
            echo "Benchmark(NodeNameLen|ValueKeyLen|NodeStructLength|ValueStructLength|Introspection)"
            ;;
        all)
            echo "."
            ;;
        *)
            log_error "Unknown category: $category"
            echo ""
            echo "Available categories:"
            echo "  open          - Open/Close operations"
            echo "  navigation    - Root, Children, GetChild, TreeWalk"
            echo "  metadata      - Timestamp, Name, StatKey, etc."
            echo "  values        - NodeValues, GetValue, ValueKey, etc."
            echo "  typed-values  - DWORD, QWORD, String values"
            echo "  introspection - Length, Structure benchmarks"
            echo "  all           - All benchmarks (default)"
            exit 1
            ;;
    esac
}

list_categories() {
    echo "Available Benchmark Categories:"
    echo ""
    echo "  open           Open/Close operations"
    echo "  navigation     Root, Children, GetChild, FullTreeWalk"
    echo "  metadata       Timestamp, NrChildren, NrValues, Name, StatKey, DetailKey"
    echo "  values         NodeValues, GetValue, ValueKey, ValueType, ValueValue"
    echo "  typed-values   ValueDword, ValueQword, ValueString"
    echo "  introspection  NodeNameLen, ValueKeyLen, StructLength"
    echo "  all            All benchmarks (default)"
    echo ""
    echo "Examples:"
    echo "  # Run only navigation benchmarks"
    echo "  ./scripts/run_benchmarks.sh --category navigation"
    echo ""
    echo "  # Run only metadata benchmarks quickly"
    echo "  ./scripts/run_benchmarks.sh --quick --category metadata"
    echo ""
    echo "  # Run specific benchmark by name"
    echo "  ./scripts/run_benchmarks.sh --bench 'BenchmarkOpen'"
    echo ""
    echo "  # Run all benchmarks for medium hive"
    echo "  ./scripts/run_benchmarks.sh --bench '.*medium'"
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --quick)
            QUICK_MODE=true
            BENCHTIME="1s"
            COUNT="1"
            shift
            ;;
        --output)
            OUTPUT_FILE="$2"
            shift 2
            ;;
        --benchtime)
            BENCHTIME="$2"
            shift 2
            ;;
        --count)
            COUNT="$2"
            shift 2
            ;;
        --bench)
            BENCH_PATTERN="$2"
            shift 2
            ;;
        --category)
            BENCH_PATTERN=$(get_category_pattern "$2")
            shift 2
            ;;
        --list)
            list_categories
            exit 0
            ;;
        --help)
            head -n 29 "$0" | tail -n +3 | sed 's/^# \?//'
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $*" >&2
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*" >&2
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*" >&2
}

# Check dependencies
check_dependencies() {
    log_info "Checking dependencies..."

    if ! command -v go &> /dev/null; then
        log_error "Go is not installed"
        exit 1
    fi

    # Check for Python virtual environment
    if [ ! -d "$PROJECT_ROOT/venv" ]; then
        log_error "Python virtual environment not found"
        log_error "Run: make install-python-deps"
        exit 1
    fi

    # Check for matplotlib in venv
    if ! "$PROJECT_ROOT/venv/bin/python3" -c "import matplotlib" &> /dev/null; then
        log_error "matplotlib is not installed in virtual environment"
        log_error "Run: make install-python-deps"
        exit 1
    fi

    if ! command -v hivexregedit &> /dev/null; then
        log_warn "hivex is not installed - hivex benchmarks will be skipped"
        log_warn "Install with: make install-hivex"
    fi

    log_success "Dependencies OK"
}

# Create reports directory
setup_output() {
    local reports_dir="$PROJECT_ROOT/benchmarks/reports"

    if [[ ! -d "$reports_dir" ]]; then
        log_info "Creating reports directory: $reports_dir"
        mkdir -p "$reports_dir"
    fi

    # Generate output filename if not specified
    if [[ -z "$OUTPUT_FILE" ]]; then
        local timestamp=$(date +"%Y-%m-%d_%H%M%S")
        local mode_suffix=""
        if [[ "$QUICK_MODE" == "true" ]]; then
            mode_suffix="_quick"
        fi
        OUTPUT_FILE="$reports_dir/${timestamp}${mode_suffix}_benchmark_report.md"
    fi

    log_info "Report will be saved to: $OUTPUT_FILE"
}

# Run benchmarks
run_benchmarks() {
    local bench_package="$PROJECT_ROOT/tests/benchmarks/comparison"
    local temp_output=$(mktemp)

    log_info "Running benchmarks..."
    log_info "  Package: $bench_package"
    log_info "  Pattern: $BENCH_PATTERN"
    log_info "  Benchtime: $BENCHTIME"
    log_info "  Count: $COUNT"

    if [[ "$QUICK_MODE" == "true" ]]; then
        log_info "  Mode: QUICK (faster, less accurate)"
    else
        log_info "  Mode: FULL (slower, more accurate)"
    fi

    echo ""

    # Run benchmarks and capture output
    # Note: We don't use -json because the parser can handle both formats
    # and regular output is easier to debug
    cd "$bench_package"

    if go test -bench="$BENCH_PATTERN" -benchmem -benchtime="$BENCHTIME" -count="$COUNT" -timeout=30m 2>&1 | tee "$temp_output"; then
        log_success "Benchmarks completed successfully"
    else
        local exit_code=$?
        log_error "Benchmarks failed with exit code $exit_code"
        log_error "Output saved to: $temp_output"
        exit $exit_code
    fi

    echo ""

    # Parse results and generate report
    log_info "Generating report..."

    if go run "$SCRIPT_DIR/benchmark_parser.go" -input="$temp_output" -output="$OUTPUT_FILE"; then
        log_success "Report generated: $OUTPUT_FILE"
    else
        log_error "Failed to generate report"
        log_error "Benchmark output saved to: $temp_output"
        exit 1
    fi

    # Generate comparison graphs
    echo ""
    log_info "Generating comparison graphs..."

    # Extract timestamp from output filename (e.g., 2025-10-26_183212 from 2025-10-26_183212_quick_benchmark_report.md)
    local timestamp=$(basename "$OUTPUT_FILE" | sed -E 's/^([0-9]{4}-[0-9]{2}-[0-9]{2}_[0-9]{6}).*$/\1/')

    # Use absolute paths for output directories
    local graphs_output="$PROJECT_ROOT/benchmarks/graphs/png"
    local docs_output="$PROJECT_ROOT/docs/images"

    if "$PROJECT_ROOT/venv/bin/python3" "$SCRIPT_DIR/generate_graphs.py" --input="$temp_output" --output="$graphs_output" --docs-dir="$docs_output" --timestamp="$timestamp" 2>&1; then
        log_success "Graphs generated in $graphs_output"
        log_success "Documentation images updated in $docs_output"
    else
        local exit_code=$?
        log_error "Failed to generate graphs (exit code: $exit_code)"
        log_error "Check stderr output above for details"
        # Don't exit - report is still available
    fi

    # Clean up temp file
    rm -f "$temp_output"
}

# Display summary
display_summary() {
    log_info "Opening report preview..."
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    # Show first 50 lines of report
    head -n 50 "$OUTPUT_FILE"

    local total_lines=$(wc -l < "$OUTPUT_FILE")
    if [[ $total_lines -gt 50 ]]; then
        echo ""
        echo "... ($(($total_lines - 50)) more lines)"
    fi

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    log_success "Full report available at: $OUTPUT_FILE"
}

# Main execution
main() {
    log_info "Starting benchmark run..."
    echo ""

    check_dependencies
    setup_output
    run_benchmarks
    display_summary

    echo ""
    log_success "Benchmark run complete!"

    if command -v open &> /dev/null; then
        echo ""
        echo "Open report with:"
        echo "  open \"$OUTPUT_FILE\""
    elif command -v xdg-open &> /dev/null; then
        echo ""
        echo "Open report with:"
        echo "  xdg-open \"$OUTPUT_FILE\""
    fi
}

# Run main
main
