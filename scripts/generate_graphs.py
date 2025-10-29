#!/usr/bin/env python3
"""
Generate benchmark comparison graphs from Go benchmark output.
Replaces the Go implementation using matplotlib instead of go-echarts.
"""

import argparse
import json
import re
import sys
from collections import defaultdict
from datetime import datetime
from pathlib import Path
from typing import List, Dict, Optional, Tuple

import matplotlib.pyplot as plt
import matplotlib
matplotlib.use('Agg')  # Use non-GUI backend for server environments

# Color scheme matching the Go version
GOHIVEX_COLOR = '#00ADD8'  # Official Go blue
HIVEX_COLOR = '#808080'    # Neutral grey for C


class BenchmarkResult:
    """Represents a parsed benchmark result."""
    def __init__(self, name: str, operation: str, hive_size: str, impl: str,
                 iterations: int, ns_per_op: float, bytes_per_op: int, allocs_per_op: int):
        self.name = name
        self.operation = operation
        self.hive_size = hive_size
        self.impl = impl
        self.iterations = iterations
        self.ns_per_op = ns_per_op
        self.bytes_per_op = bytes_per_op
        self.allocs_per_op = allocs_per_op


class ComparisonResult:
    """Represents a comparison between gohivex and hivex."""
    def __init__(self, operation: str, hive_size: str):
        self.operation = operation
        self.hive_size = hive_size
        self.gohivex_ns = 0.0
        self.hivex_ns = 0.0
        self.speedup = 0.0
        self.gohivex_mem = 0
        self.hivex_mem = 0
        self.gohivex_allocs = 0
        self.hivex_allocs = 0
        self.gohivex_only = False

    @property
    def label(self) -> str:
        """Generate display label for the comparison."""
        if self.hive_size:
            return f"{self.operation} ({self.hive_size})"
        return self.operation


def parse_json_benchmark(line: str) -> Optional[BenchmarkResult]:
    """Parse a JSON-formatted benchmark line."""
    try:
        data = json.loads(line)
        name = data.get('Name', '')

        # Parse name: Benchmark<Operation>/<impl>/<size>/<variant>
        parts = name.split('/')
        if len(parts) < 2:
            return None

        operation = parts[0].replace('Benchmark', '', 1)
        impl = parts[1]
        hive_size = parts[2] if len(parts) >= 3 else ''
        variant = parts[3] if len(parts) >= 4 else ''

        # Build display name
        display_name = operation
        if variant:
            display_name = f"{operation}/{variant}"

        return BenchmarkResult(
            name=name,
            operation=display_name,
            hive_size=hive_size,
            impl=impl,
            iterations=data.get('N', 0),
            ns_per_op=data.get('T', 0.0),
            bytes_per_op=data.get('B', 0),
            allocs_per_op=data.get('A', 0)
        )
    except (json.JSONDecodeError, KeyError):
        return None


def parse_text_benchmark(line: str) -> Optional[BenchmarkResult]:
    """Parse a text-formatted benchmark line."""
    # Example: BenchmarkNodeGetChild/gohivex/medium/abcd_äöüß-10  3456789  352.5 ns/op  160 B/op  7 allocs/op
    pattern = r'^Benchmark(\w+)/(gohivex|hivex)/(\w+)(?:/(.+?))?-\d+\s+(\d+)\s+([\d.]+)\s+ns/op(?:\s+(\d+)\s+B/op)?(?:\s+(\d+)\s+allocs/op)?'
    match = re.match(pattern, line)

    if not match:
        return None

    operation, impl, hive_size, variant, iterations, ns_per_op, bytes_per_op, allocs_per_op = match.groups()

    # Build display name
    display_name = operation
    if variant:
        display_name = f"{operation}/{variant}"

    return BenchmarkResult(
        name=f"Benchmark{operation}/{impl}/{hive_size}/{variant or ''}",
        operation=display_name,
        hive_size=hive_size,
        impl=impl,
        iterations=int(iterations),
        ns_per_op=float(ns_per_op),
        bytes_per_op=int(bytes_per_op) if bytes_per_op else 0,
        allocs_per_op=int(allocs_per_op) if allocs_per_op else 0
    )


def parse_benchmarks(lines: List[str]) -> List[BenchmarkResult]:
    """Parse benchmark results from input lines."""
    results = []

    for line in lines:
        line = line.strip()
        if not line:
            continue

        # Try JSON format first
        if line.startswith('{'):
            result = parse_json_benchmark(line)
            if result:
                results.append(result)
                continue

        # Try text format
        result = parse_text_benchmark(line)
        if result:
            results.append(result)

    return results


def generate_comparisons(results: List[BenchmarkResult],
                         filter_type: str = 'standard') -> List[ComparisonResult]:
    """Generate comparisons between gohivex and hivex implementations.

    Args:
        results: Parsed benchmark results
        filter_type: 'standard' for regular ops, 'mutation' for write/recursive ops
    """
    # Operations with high memory usage that pollute standard graphs
    MUTATION_OPERATIONS = {
        'NodeAddChild', 'NodeSetValue', 'NodeSetValues', 'NodeDeleteChild',
        'Commit', 'IntrospectionRecursive'
    }

    # Group by operation and hive size
    groups = defaultdict(list)
    for result in results:
        operation_base = result.operation.split('/')[0]  # Handle "Operation/variant" format
        is_mutation = operation_base in MUTATION_OPERATIONS

        # Filter based on type
        if filter_type == 'standard' and is_mutation:
            continue  # Skip mutations for standard graphs
        elif filter_type == 'mutation' and not is_mutation:
            continue  # Skip standard ops for mutation graphs

        key = (result.operation, result.hive_size)
        groups[key].append(result)

    comparisons = []
    for (operation, hive_size), group in groups.items():
        gohivex = None
        hivex = None

        for result in group:
            if result.impl == 'gohivex':
                gohivex = result
            elif result.impl == 'hivex':
                hivex = result

        comp = ComparisonResult(operation, hive_size)

        if gohivex:
            comp.gohivex_ns = gohivex.ns_per_op
            comp.gohivex_mem = gohivex.bytes_per_op
            comp.gohivex_allocs = gohivex.allocs_per_op

        if hivex:
            comp.hivex_ns = hivex.ns_per_op
            comp.hivex_mem = hivex.bytes_per_op
            comp.hivex_allocs = hivex.allocs_per_op

        if gohivex and not hivex:
            comp.gohivex_only = True

        if comp.gohivex_ns > 0 and comp.hivex_ns > 0:
            comp.speedup = comp.hivex_ns / comp.gohivex_ns

        comparisons.append(comp)

    # Sort by operation name, then hive size
    comparisons.sort(key=lambda c: (c.operation, c.hive_size))

    return comparisons


def create_horizontal_bar_chart(comparisons: List[ComparisonResult],
                                title: str, subtitle: str,
                                gohivex_values: List[float],
                                hivex_values: List[float],
                                output_path: Path):
    """Create a horizontal bar chart comparing gohivex and hivex."""
    # Calculate figure size based on number of comparisons
    height = max(8, len(comparisons) * 0.25)
    fig, ax = plt.subplots(figsize=(16, height))

    # Prepare data
    labels = [comp.label for comp in comparisons]
    y_pos = range(len(labels))

    # Create horizontal bars
    bar_height = 0.35
    ax.barh([y - bar_height/2 for y in y_pos], gohivex_values, bar_height,
            label='gohivex', color=GOHIVEX_COLOR)
    ax.barh([y + bar_height/2 for y in y_pos], hivex_values, bar_height,
            label='hivex', color=HIVEX_COLOR)

    # Customize chart
    ax.set_yticks(y_pos)
    ax.set_yticklabels(labels)
    ax.invert_yaxis()  # Labels read top-to-bottom
    ax.set_title(f"{title}\n{subtitle}", fontsize=14, pad=20)
    ax.legend(loc='upper right')
    ax.grid(axis='x', alpha=0.3)

    # Adjust layout to prevent label cutoff
    plt.tight_layout()

    # Save to file
    plt.savefig(output_path, dpi=150, bbox_inches='tight')
    plt.close()


def generate_time_graph(comparisons: List[ComparisonResult], output_dir: Path,
                        timestamp: str, prefix: str = '', docs_dir: Path = None):
    """Generate time comparison graph."""
    gohivex_values = [comp.gohivex_ns for comp in comparisons]
    hivex_values = [0 if comp.gohivex_only else comp.hivex_ns for comp in comparisons]

    # Timestamped version for history
    filename = f"{timestamp}_{prefix}time.png" if prefix else f"{timestamp}_time.png"
    output_path = output_dir / filename
    create_horizontal_bar_chart(
        comparisons,
        "Performance Comparison (ns/op)",
        "gohivex vs hivex - Lower is Better",
        gohivex_values,
        hivex_values,
        output_path
    )

    # Static version for documentation
    if docs_dir:
        static_filename = f"{prefix}time.png" if prefix else "time.png"
        docs_path = docs_dir / static_filename
        create_horizontal_bar_chart(
            comparisons,
            "Performance Comparison (ns/op)",
            "gohivex vs hivex - Lower is Better",
            gohivex_values,
            hivex_values,
            docs_path
        )


def generate_memory_graph(comparisons: List[ComparisonResult], output_dir: Path,
                          timestamp: str, prefix: str = '', docs_dir: Path = None):
    """Generate memory comparison graph."""
    gohivex_values = [comp.gohivex_mem for comp in comparisons]
    hivex_values = [0 if comp.gohivex_only else comp.hivex_mem for comp in comparisons]

    # Timestamped version for history
    filename = f"{timestamp}_{prefix}memory.png" if prefix else f"{timestamp}_memory.png"
    output_path = output_dir / filename
    create_horizontal_bar_chart(
        comparisons,
        "Memory Usage Comparison (B/op)",
        "gohivex vs hivex - Lower is Better",
        gohivex_values,
        hivex_values,
        output_path
    )

    # Static version for documentation
    if docs_dir:
        static_filename = f"{prefix}memory.png" if prefix else "memory.png"
        docs_path = docs_dir / static_filename
        create_horizontal_bar_chart(
            comparisons,
            "Memory Usage Comparison (B/op)",
            "gohivex vs hivex - Lower is Better",
            gohivex_values,
            hivex_values,
            docs_path
        )


def generate_allocations_graph(comparisons: List[ComparisonResult], output_dir: Path,
                               timestamp: str, prefix: str = '', docs_dir: Path = None):
    """Generate allocations comparison graph."""
    gohivex_values = [comp.gohivex_allocs for comp in comparisons]
    hivex_values = [0 if comp.gohivex_only else comp.hivex_allocs for comp in comparisons]

    # Timestamped version for history
    filename = f"{timestamp}_{prefix}allocations.png" if prefix else f"{timestamp}_allocations.png"
    output_path = output_dir / filename
    create_horizontal_bar_chart(
        comparisons,
        "Allocations Comparison (allocs/op)",
        "gohivex vs hivex - Lower is Better",
        gohivex_values,
        hivex_values,
        output_path
    )

    # Static version for documentation
    if docs_dir:
        static_filename = f"{prefix}allocations.png" if prefix else "allocations.png"
        docs_path = docs_dir / static_filename
        create_horizontal_bar_chart(
            comparisons,
            "Allocations Comparison (allocs/op)",
            "gohivex vs hivex - Lower is Better",
            gohivex_values,
            hivex_values,
            docs_path
        )


def main():
    parser = argparse.ArgumentParser(
        description='Generate benchmark comparison graphs from Go benchmark output'
    )
    parser.add_argument('--input', type=str, default='',
                       help='Input file with benchmark JSON output (stdin if not specified)')
    parser.add_argument('--output', type=str, default='benchmarks/graphs/png',
                       help='Output directory for PNG graphs')
    parser.add_argument('--docs-dir', type=str, default='docs/images',
                       help='Output directory for static documentation images (default: docs/images)')
    parser.add_argument('--timestamp', type=str, default='',
                       help='Timestamp for output files (auto-generated if not specified)')
    parser.add_argument('--quiet', action='store_true',
                       help='Suppress progress output')

    args = parser.parse_args()

    # Read input
    if args.input:
        try:
            with open(args.input, 'r') as f:
                lines = f.readlines()
        except IOError as e:
            print(f"Error opening input file: {e}", file=sys.stderr)
            sys.exit(1)
    else:
        lines = sys.stdin.readlines()

    # Parse benchmarks
    results = parse_benchmarks(lines)
    if not args.quiet:
        print(f"Parsed {len(results)} benchmark results", file=sys.stderr)

    # Generate standard comparisons (read operations)
    standard_comparisons = generate_comparisons(results, filter_type='standard')
    if not args.quiet:
        print(f"Generated {len(standard_comparisons)} standard comparisons", file=sys.stderr)

    # Generate mutation comparisons (write/recursive operations)
    mutation_comparisons = generate_comparisons(results, filter_type='mutation')
    if not args.quiet:
        print(f"Generated {len(mutation_comparisons)} mutation comparisons", file=sys.stderr)

    # Generate timestamp if not provided
    timestamp = args.timestamp if args.timestamp else datetime.now().strftime('%Y-%m-%d_%H%M%S')

    # Create output directory
    output_dir = Path(args.output)
    output_dir.mkdir(parents=True, exist_ok=True)

    # Create docs directory for static images
    docs_dir = Path(args.docs_dir)
    docs_dir.mkdir(parents=True, exist_ok=True)

    # Generate standard graphs
    try:
        if standard_comparisons:
            generate_time_graph(standard_comparisons, output_dir, timestamp, docs_dir=docs_dir)
            generate_memory_graph(standard_comparisons, output_dir, timestamp, docs_dir=docs_dir)
            generate_allocations_graph(standard_comparisons, output_dir, timestamp, docs_dir=docs_dir)
            if not args.quiet:
                print(f"Generated standard graphs ({len(standard_comparisons)} ops)", file=sys.stderr)
    except Exception as e:
        print(f"Error generating standard graphs: {e}", file=sys.stderr)
        sys.exit(1)

    # Generate mutation graphs (separate set with different scales)
    try:
        if mutation_comparisons:
            generate_time_graph(mutation_comparisons, output_dir, timestamp, prefix='mutation_', docs_dir=docs_dir)
            generate_memory_graph(mutation_comparisons, output_dir, timestamp, prefix='mutation_', docs_dir=docs_dir)
            generate_allocations_graph(mutation_comparisons, output_dir, timestamp, prefix='mutation_', docs_dir=docs_dir)
            if not args.quiet:
                print(f"Generated mutation graphs ({len(mutation_comparisons)} ops)", file=sys.stderr)
    except Exception as e:
        print(f"Error generating mutation graphs: {e}", file=sys.stderr)
        sys.exit(1)

    if not args.quiet:
        print(f"All graphs generated in {output_dir}", file=sys.stderr)
        print(f"Static documentation images in {docs_dir}", file=sys.stderr)


if __name__ == '__main__':
    main()
