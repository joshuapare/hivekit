#!/bin/bash
# Update README.md with tool help outputs and benchmark images

set -e

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
README="${REPO_ROOT}/README.md"
TEMP_README="${REPO_ROOT}/.readme.tmp"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Build tools if needed
build_tools() {
    log_info "Building tools..."
    cd "${REPO_ROOT}"

    # Build hivectl
    if [ ! -f "hivectl" ]; then
        log_info "Building hivectl..."
        cd cmd/hivectl && go build -o ../../hivectl . && cd ../..
    fi

    # Build hiveexplorer
    if [ ! -f "hiveexplorer" ]; then
        log_info "Building hiveexplorer..."
        cd cmd/hiveexplorer && go build -o ../../hiveexplorer . && cd ../..
    fi
}

# Update tool help sections
update_help_sections() {
    log_info "Updating help sections in README..."

    # Get help outputs and save to temp files
    ./hivectl --help 2>&1 > /tmp/hivectl_help.txt || true
    ./hiveexplorer --help 2>&1 > /tmp/hiveexplorer_help.txt || true

    # Clear temp file
    > "$TEMP_README"

    # Process README line by line
    in_hivectl=0
    in_explorer=0

    while IFS= read -r line; do
        if [[ "$line" == "<!-- HIVECTL_HELP_START -->" ]]; then
            echo "$line" >> "$TEMP_README"
            echo '```' >> "$TEMP_README"
            cat /tmp/hivectl_help.txt >> "$TEMP_README"
            echo '```' >> "$TEMP_README"
            in_hivectl=1
            continue
        elif [[ "$line" == "<!-- HIVECTL_HELP_END -->" ]]; then
            echo "$line" >> "$TEMP_README"
            in_hivectl=0
            continue
        elif [[ "$line" == "<!-- HIVEEXPLORER_HELP_START -->" ]]; then
            echo "$line" >> "$TEMP_README"
            echo '```' >> "$TEMP_README"
            cat /tmp/hiveexplorer_help.txt >> "$TEMP_README"
            echo '```' >> "$TEMP_README"
            in_explorer=1
            continue
        elif [[ "$line" == "<!-- HIVEEXPLORER_HELP_END -->" ]]; then
            echo "$line" >> "$TEMP_README"
            in_explorer=0
            continue
        fi

        # Skip lines between markers
        if [[ $in_hivectl -eq 1 || $in_explorer -eq 1 ]]; then
            continue
        fi

        echo "$line" >> "$TEMP_README"
    done < "$README"

    mv "$TEMP_README" "$README"
    rm -f /tmp/hivectl_help.txt /tmp/hiveexplorer_help.txt

    log_info "Help sections updated"
}

# Update benchmark images
update_benchmarks() {
    log_info "Checking for benchmark images..."

    # Check if docs images directory exists
    DOCS_IMAGES_DIR="${REPO_ROOT}/docs/images"
    if [ ! -d "$DOCS_IMAGES_DIR" ]; then
        log_warn "No benchmark images found at $DOCS_IMAGES_DIR"
        log_info "Run 'make benchmark-compare' to generate benchmark graphs"
        return 0
    fi

    # Find all PNG files in the docs images directory (static versions)
    BENCHMARK_IMAGES=$(find "$DOCS_IMAGES_DIR" -name "*.png" -type f | sort)

    if [ -z "$BENCHMARK_IMAGES" ]; then
        log_warn "No benchmark images found in $DOCS_IMAGES_DIR"
        return 0
    fi

    log_info "Found $(echo "$BENCHMARK_IMAGES" | wc -l | tr -d ' ') benchmark images"

    # Build markdown for benchmark images
    BENCHMARK_MD="Performance benchmarks comparing hivekit against hivex (libguestfs C implementation).\n\n"
    BENCHMARK_MD="${BENCHMARK_MD}### Standard Operations\n\n"
    BENCHMARK_MD="${BENCHMARK_MD}Read operations and basic traversal:\n\n"

    # Add standard operation graphs
    for img_name in "time.png" "memory.png" "allocations.png"; do
        img="${DOCS_IMAGES_DIR}/${img_name}"
        if [ -f "$img" ]; then
            REL_PATH="$(realpath --relative-to="$REPO_ROOT" "$img" 2>/dev/null || python3 -c "import os; print(os.path.relpath('$img', '$REPO_ROOT'))")"
            BASENAME=$(basename "$img" .png)
            TITLE=$(echo "$BASENAME" | sed 's/_/ /g' | sed 's/\b\(.\)/\u\1/g')
            BENCHMARK_MD="${BENCHMARK_MD}![${TITLE}](${REL_PATH})\n\n"
        fi
    done

    BENCHMARK_MD="${BENCHMARK_MD}### Mutation Operations\n\n"
    BENCHMARK_MD="${BENCHMARK_MD}Write operations and recursive traversal:\n\n"

    # Add mutation operation graphs
    for img_name in "mutation_time.png" "mutation_memory.png" "mutation_allocations.png"; do
        img="${DOCS_IMAGES_DIR}/${img_name}"
        if [ -f "$img" ]; then
            REL_PATH="$(realpath --relative-to="$REPO_ROOT" "$img" 2>/dev/null || python3 -c "import os; print(os.path.relpath('$img', '$REPO_ROOT'))")"
            BASENAME=$(basename "$img" .png)
            TITLE=$(echo "$BASENAME" | sed 's/mutation_//g' | sed 's/_/ /g' | sed 's/\b\(.\)/\u\1/g')
            BENCHMARK_MD="${BENCHMARK_MD}![Mutation ${TITLE}](${REL_PATH})\n\n"
        fi
    done

    BENCHMARK_MD="${BENCHMARK_MD}Run \`make benchmark-compare\` to generate updated benchmark results."

    # Update README with benchmark images
    awk -v bench_md="$BENCHMARK_MD" '
    BEGIN { in_benchmarks = 0 }
    /<!-- BENCHMARKS_START -->/ {
        in_benchmarks = 1
        print $0
        printf "%s\n", bench_md
        next
    }
    /<!-- BENCHMARKS_END -->/ {
        in_benchmarks = 0
        print $0
        next
    }
    in_benchmarks { next }
    { print }
    ' "$README" > "$TEMP_README"

    mv "$TEMP_README" "$README"
    log_info "Benchmark section updated with $(echo "$BENCHMARK_IMAGES" | wc -l | tr -d ' ') images"
}

# Main execution
main() {
    log_info "Starting README update..."

    build_tools
    update_help_sections
    update_benchmarks

    log_info "README updated successfully!"
}

main "$@"
