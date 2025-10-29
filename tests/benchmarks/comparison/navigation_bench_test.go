package comparison

import (
	"testing"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// BenchmarkRoot compares performance of getting root node
// Measures: hivex_root vs Reader.Root().
func BenchmarkRoot(b *testing.B) {
	for _, hf := range BenchmarkHives {
		// Benchmark gohivex
		b.Run("gohivex/"+hf.Name, func(b *testing.B) {
			// Open once (not benchmarked)
			r, err := reader.Open(hf.Path, hive.OpenOptions{})
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer r.Close()

			var root hive.NodeID

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				root, err = r.Root()
				if err != nil {
					b.Fatalf("Root failed: %v", err)
				}
			}

			benchGoNodeID = root
		})

		// Benchmark hivex
		b.Run("hivex/"+hf.Name, func(b *testing.B) {
			// Open once (not benchmarked)
			h, err := bindings.Open(hf.Path, 0)
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer h.Close()

			var root bindings.NodeHandle

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				root = h.Root()
			}

			benchHivexNode = root
		})
	}
}

// BenchmarkNodeChildren compares performance of enumerating children
// Measures: hivex_node_children vs Reader.Subkeys().
func BenchmarkNodeChildren(b *testing.B) {
	for _, hf := range BenchmarkHives {
		// Benchmark gohivex
		b.Run("gohivex/"+hf.Name, func(b *testing.B) {
			// Open and get root once (not benchmarked)
			r, err := reader.Open(hf.Path, hive.OpenOptions{})
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer r.Close()

			root, err := r.Root()
			if err != nil {
				b.Fatalf("Root failed: %v", err)
			}

			var children []hive.NodeID

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				children, err = r.Subkeys(root)
				if err != nil {
					b.Fatalf("Subkeys failed: %v", err)
				}
			}

			benchGoNodeIDs = children
		})

		// Benchmark hivex
		b.Run("hivex/"+hf.Name, func(b *testing.B) {
			// Open and get root once (not benchmarked)
			h, err := bindings.Open(hf.Path, 0)
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer h.Close()

			root := h.Root()

			var children []bindings.NodeHandle

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				children = h.NodeChildren(root)
			}

			benchHivexNodes = children
		})
	}
}

// BenchmarkNodeGetChild compares performance of looking up child by name
// Measures: hivex_node_get_child vs Reader.Lookup()
// Only benchmarks hives with known children.
func BenchmarkNodeGetChild(b *testing.B) {
	// Use special hive which has known children
	testCases := []struct {
		hiveName  string
		hivePath  string
		childName string
	}{
		{BenchmarkHives[1].Name, BenchmarkHives[1].Path, "abcd_äöüß"},   // special hive
		{BenchmarkHives[1].Name, BenchmarkHives[1].Path, "zero\x00key"}, // special hive
		{BenchmarkHives[1].Name, BenchmarkHives[1].Path, "weird™"},      // special hive
	}

	for _, tc := range testCases {
		// Benchmark gohivex
		b.Run("gohivex/"+tc.hiveName+"/"+tc.childName, func(b *testing.B) {
			// Open and get root once (not benchmarked)
			r, err := reader.Open(tc.hivePath, hive.OpenOptions{})
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer r.Close()

			root, err := r.Root()
			if err != nil {
				b.Fatalf("Root failed: %v", err)
			}

			var child hive.NodeID

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				child, err = r.Lookup(root, tc.childName)
				if err != nil {
					b.Fatalf("Lookup failed: %v", err)
				}
			}

			benchGoNodeID = child
		})

		// Benchmark hivex
		b.Run("hivex/"+tc.hiveName+"/"+tc.childName, func(b *testing.B) {
			// Open and get root once (not benchmarked)
			h, err := bindings.Open(tc.hivePath, 0)
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer h.Close()

			root := h.Root()

			var child bindings.NodeHandle

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				child = h.NodeGetChild(root, tc.childName)
			}

			benchHivexNode = child
		})
	}
}

// BenchmarkNodeName compares performance of getting node name
// Measures: hivex_node_name vs Reader.StatKey().Name.
func BenchmarkNodeName(b *testing.B) {
	for _, hf := range BenchmarkHives {
		// Benchmark gohivex
		b.Run("gohivex/"+hf.Name, func(b *testing.B) {
			// Open and get root once (not benchmarked)
			r, err := reader.Open(hf.Path, hive.OpenOptions{})
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer r.Close()

			root, err := r.Root()
			if err != nil {
				b.Fatalf("Root failed: %v", err)
			}

			var name string

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				name, err = r.KeyName(root)
				if err != nil {
					b.Fatalf("KeyName failed: %v", err)
				}
			}

			benchGoString = name
		})

		// Benchmark hivex
		b.Run("hivex/"+hf.Name, func(b *testing.B) {
			// Open and get root once (not benchmarked)
			h, err := bindings.Open(hf.Path, 0)
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer h.Close()

			root := h.Root()

			var name string

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				name = h.NodeName(root)
			}

			benchHivexString = name
		})
	}
}

// BenchmarkFullTreeWalk compares performance of walking entire tree
// Measures: Complete recursive traversal.
func BenchmarkFullTreeWalk(b *testing.B) {
	for _, hf := range BenchmarkHives {
		// Benchmark gohivex
		b.Run("gohivex/"+hf.Name, func(b *testing.B) {
			// Open and get root once (not benchmarked)
			r, err := reader.Open(hf.Path, hive.OpenOptions{})
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer r.Close()

			root, err := r.Root()
			if err != nil {
				b.Fatalf("Root failed: %v", err)
			}

			var nodeCount int

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				nodeCount = walkTreeGohivex(b, r, root)
			}

			benchGoInt = nodeCount
		})

		// Benchmark hivex
		b.Run("hivex/"+hf.Name, func(b *testing.B) {
			// Open and get root once (not benchmarked)
			h, err := bindings.Open(hf.Path, 0)
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer h.Close()

			root := h.Root()

			var nodeCount int

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				nodeCount = walkTreeHivex(b, h, root)
			}

			benchHivexInt = nodeCount
		})
	}
}

// walkTreeGohivex recursively walks tree and counts nodes.
func walkTreeGohivex(b *testing.B, r hive.Reader, node hive.NodeID) int {
	count := 1

	children, err := r.Subkeys(node)
	if err != nil {
		b.Fatalf("Subkeys failed: %v", err)
	}

	for _, child := range children {
		count += walkTreeGohivex(b, r, child)
	}

	return count
}

// walkTreeHivex recursively walks tree and counts nodes.
func walkTreeHivex(b *testing.B, h *bindings.Hive, node bindings.NodeHandle) int {
	_ = b // Parameter kept for signature consistency with walkTreeGohivex
	count := 1

	children := h.NodeChildren(node)

	for _, child := range children {
		count += walkTreeHivex(b, h, child)
	}

	return count
}

// BenchmarkNodeChildrenWithNames benchmarks getting children and reading all names
// More realistic workload than just getting children IDs.
func BenchmarkNodeChildrenWithNames(b *testing.B) {
	for _, hf := range BenchmarkHives {
		// Benchmark gohivex
		b.Run("gohivex/"+hf.Name, func(b *testing.B) {
			// Open and get root once (not benchmarked)
			r, err := reader.Open(hf.Path, hive.OpenOptions{})
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer r.Close()

			root, err := r.Root()
			if err != nil {
				b.Fatalf("Root failed: %v", err)
			}

			var names []string

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				children, err := r.Subkeys(root)
				if err != nil {
					b.Fatalf("Subkeys failed: %v", err)
				}

				names = make([]string, len(children))
				for j, child := range children {
					name, err := r.KeyName(child)
					if err != nil {
						b.Fatalf("KeyName failed: %v", err)
					}
					names[j] = name
				}
			}

			benchGoStrings = names
		})

		// Benchmark hivex
		b.Run("hivex/"+hf.Name, func(b *testing.B) {
			// Open and get root once (not benchmarked)
			h, err := bindings.Open(hf.Path, 0)
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer h.Close()

			root := h.Root()

			var names []string

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				children := h.NodeChildren(root)

				names = make([]string, len(children))
				for j, child := range children {
					names[j] = h.NodeName(child)
				}
			}

			benchHivexStrings = names
		})
	}
}

// BenchmarkNodeParent compares performance of getting parent node
// Measures: hivex_node_parent vs Reader.Parent().
func BenchmarkNodeParent(b *testing.B) {
	// Use special hive which has known children to test parent relationship
	hf := BenchmarkHives[1] // special hive

	// Benchmark gohivex
	b.Run("gohivex/"+hf.Name, func(b *testing.B) {
		// Open and navigate to a child once (not benchmarked)
		r, err := reader.Open(hf.Path, hive.OpenOptions{})
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer r.Close()

		root, err := r.Root()
		if err != nil {
			b.Fatalf("Root failed: %v", err)
		}

		// Get a child node to find its parent
		child, err := r.Lookup(root, "abcd_äöüß")
		if err != nil {
			b.Fatalf("Lookup failed: %v", err)
		}

		var parent hive.NodeID

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			parent, err = r.Parent(child)
			if err != nil {
				b.Fatalf("Parent failed: %v", err)
			}
		}

		benchGoNodeID = parent
	})

	// Benchmark hivex
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		// Open and navigate to a child once (not benchmarked)
		h, err := bindings.Open(hf.Path, 0)
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer h.Close()

		root := h.Root()
		child := h.NodeGetChild(root, "abcd_äöüß")

		var parent bindings.NodeHandle

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			parent = h.NodeParent(child)
		}

		benchHivexNode = parent
	})
}
