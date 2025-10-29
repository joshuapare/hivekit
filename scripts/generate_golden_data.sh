#!/bin/bash
# Generate golden reference data from hivex for all test hives

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
GOLDEN_DIR="$PROJECT_ROOT/tests/integration/golden"
OUTPUT_DIR="$PROJECT_ROOT/tests/integration/output"

echo "Generating golden reference data..."
echo "Project root: $PROJECT_ROOT"
echo "Golden data dir: $GOLDEN_DIR"
echo ""

# Create directories
mkdir -p "$GOLDEN_DIR"
mkdir -p "$OUTPUT_DIR"

# Check hivex is installed
if ! command -v hivexregedit &> /dev/null; then
    echo "❌ hivexregedit not found. Install hivex first: make install-hivex"
    exit 1
fi

echo "Using hivex version: $(hivexsh --version 2>&1 | head -1)"
echo ""

# Function to generate golden data for a single hive
generate_golden() {
    local hive_path="$1"
    local hive_name="$2"

    echo "Processing: $hive_name"

    if [ -f "$hive_path" ]; then
        # Generate .reg export (more reliable than XML, no control character issues)
        echo "  Generating .reg export..."
        hivexregedit --export "$hive_path" '\\' > "$GOLDEN_DIR/${hive_name}.reg" 2>/dev/null || {
            echo "  ⚠️  Failed to generate .reg for $hive_name"
            return 1
        }

        # Generate statistics from .reg file
        echo "  Generating statistics..."
        {
            local reg_file="$GOLDEN_DIR/${hive_name}.reg"
            # Count keys: lines starting with [
            local node_count=$(grep -c '^\[' "$reg_file" || echo "0")
            # Count values: lines with "name"=value format (excluding @ for default values)
            local value_count=$(grep -c '^".*"=' "$reg_file" || echo "0")
            # Add default values (lines starting with @=)
            local default_count=$(grep -c '^@=' "$reg_file" || echo "0")
            value_count=$((value_count + default_count))
            local file_size=$(stat -f%z "$hive_path" 2>/dev/null || stat -c%s "$hive_path" 2>/dev/null || echo "0")

            echo "# Statistics for $hive_name"
            echo "hive_file: $hive_name"
            echo "file_size: $file_size"
            echo "node_count: $node_count"
            echo "value_count: $value_count"
            echo "generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
        } > "$GOLDEN_DIR/${hive_name}.stats.txt"

        echo "  ✅ Complete"
        return 0
    else
        echo "  ⚠️  File not found: $hive_path"
        return 1
    fi
}

# Track statistics
total=0
success=0
failed=0

# Process core test hives
echo "=== Core Test Hives ==="
for hive in "$PROJECT_ROOT"/testdata/{minimal,special,rlenvalue_test_hive,large}; do
    if [ -f "$hive" ]; then
        total=$((total + 1))
        hive_name=$(basename "$hive")
        if generate_golden "$hive" "$hive_name"; then
            success=$((success + 1))
        else
            failed=$((failed + 1))
        fi
        echo ""
    fi
done

# Process real Windows hives from suite
echo "=== Real Windows Hives (Suite) ==="
if [ -d "$PROJECT_ROOT/testdata/suite" ]; then
    for hive in "$PROJECT_ROOT"/testdata/suite/windows-*; do
        # Skip .xz and .reg files
        if [[ "$hive" == *.xz ]] || [[ "$hive" == *.reg ]]; then
            continue
        fi

        if [ -f "$hive" ] && [ ! -d "$hive" ]; then
            total=$((total + 1))
            hive_name=$(basename "$hive")
            if generate_golden "$hive" "$hive_name"; then
                success=$((success + 1))
            else
                failed=$((failed + 1))
            fi
            echo ""
        fi
    done
fi

# Summary
echo "==================================="
echo "Golden Data Generation Complete"
echo "==================================="
echo "Total hives:    $total"
echo "Successful:     $success"
echo "Failed:         $failed"
echo ""
echo "Golden data location: $GOLDEN_DIR"
echo ""

# List generated files
echo "Generated files:"
find "$GOLDEN_DIR" -type f | sort | while read -r file; do
    size=$(du -h "$file" | cut -f1)
    name=$(basename "$file")
    echo "  $name ($size)"
done

if [ $failed -gt 0 ]; then
    echo ""
    echo "⚠️  Some hives failed to process. Check output above for details."
    exit 1
fi

echo ""
echo "✅ All golden reference data generated successfully!"
