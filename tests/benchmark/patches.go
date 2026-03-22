package benchmark

import (
	"encoding/binary"
	"fmt"
	"math/rand"

	"github.com/joshuapare/hivekit/hive/merge"
	"github.com/joshuapare/hivekit/internal/format"
)

// GenerateCreateSparse generates OpEnsureKey ops for new keys scattered across
// the hive. Each key uses a unique top-level prefix, producing a wide, flat
// distribution that exercises hive-wide allocation and index growth.
func GenerateCreateSparse(count int, seed int64) []merge.Op {
	rng := rand.New(rand.NewSource(seed))
	ops := make([]merge.Op, 0, count)

	for range count {
		// Each key gets a unique top-level parent to maximize scatter.
		depth := 2 + rng.Intn(4) // 2-5 levels deep
		path := make([]string, depth)
		path[0] = fmt.Sprintf("Sparse%d", rng.Intn(count*10))
		for j := 1; j < depth; j++ {
			path[j] = fmt.Sprintf("Node%d", rng.Intn(1000))
		}
		ops = append(ops, merge.Op{
			Type:    merge.OpEnsureKey,
			KeyPath: path,
		})
	}

	return ops
}

// GenerateCreateDense generates OpEnsureKey ops for new keys with heavy prefix
// sharing. All keys are placed under a small number of parent keys, exercising
// subkey list growth and cell splitting within concentrated subtrees.
func GenerateCreateDense(count int, seed int64) []merge.Op {
	rng := rand.New(rand.NewSource(seed))
	ops := make([]merge.Op, 0, count)

	// Use only 3 top-level parents to maximize prefix sharing.
	parents := []string{"DenseA", "DenseB", "DenseC"}

	for range count {
		parent := parents[rng.Intn(len(parents))]
		depth := 2 + rng.Intn(4) // 2-5 levels total
		path := make([]string, depth)
		path[0] = parent
		// Reuse a small set of intermediate names to share prefixes.
		for j := 1; j < depth; j++ {
			path[j] = fmt.Sprintf("Branch%d", rng.Intn(10))
		}
		ops = append(ops, merge.Op{
			Type:    merge.OpEnsureKey,
			KeyPath: path,
		})
	}

	return ops
}

// GenerateCreateDeep generates OpEnsureKey ops for keys at 10-15 levels of
// depth. This exercises deep tree traversal and path resolution during merge.
func GenerateCreateDeep(count int, seed int64) []merge.Op {
	rng := rand.New(rand.NewSource(seed))
	ops := make([]merge.Op, 0, count)

	for range count {
		depth := 10 + rng.Intn(6) // 10-15 levels
		path := make([]string, depth)
		for j := range depth {
			path[j] = fmt.Sprintf("L%d_%d", j, rng.Intn(50))
		}
		ops = append(ops, merge.Op{
			Type:    merge.OpEnsureKey,
			KeyPath: path,
		})
	}

	return ops
}

// GenerateUpdateExisting generates OpSetValue ops that update values on keys
// known to exist. The updates use the same value type and similar size as the
// originals, exercising in-place value replacement without cell reallocation.
func GenerateUpdateExisting(count int, existingKeys [][]string, seed int64) []merge.Op {
	rng := rand.New(rand.NewSource(seed))
	ops := make([]merge.Op, 0, count)

	for range count {
		keyPath := pickKey(rng, existingKeys)
		// Use DWORD type for same-type/same-size updates.
		data := make([]byte, format.DWORDSize)
		binary.LittleEndian.PutUint32(data, rng.Uint32())
		ops = append(ops, merge.Op{
			Type:      merge.OpSetValue,
			KeyPath:   keyPath,
			ValueName: fmt.Sprintf("Value%d", rng.Intn(5)),
			ValueType: format.REGDWORD,
			Data:      data,
		})
	}

	return ops
}

// GenerateUpdateResize generates OpSetValue ops that change the type and size
// of existing values. This exercises cell reallocation: the value was originally
// a DWORD (4 bytes) and is replaced with a string value of variable length.
func GenerateUpdateResize(count int, existingKeys [][]string, seed int64) []merge.Op {
	rng := rand.New(rand.NewSource(seed))
	ops := make([]merge.Op, 0, count)

	for range count {
		keyPath := pickKey(rng, existingKeys)
		// Replace with a string value (larger than DWORD) to force resize.
		str := randomString(rng, 20+rng.Intn(100))
		data := encodeUTF16LE(str)
		ops = append(ops, merge.Op{
			Type:      merge.OpSetValue,
			KeyPath:   keyPath,
			ValueName: fmt.Sprintf("Value%d", rng.Intn(5)),
			ValueType: format.REGSZ,
			Data:      data,
		})
	}

	return ops
}

// GenerateDeleteValues generates OpDeleteValue ops targeting individual values
// on keys known to exist. This exercises value list manipulation without
// affecting the key tree structure.
func GenerateDeleteValues(count int, existingKeys [][]string, seed int64) []merge.Op {
	rng := rand.New(rand.NewSource(seed))
	ops := make([]merge.Op, 0, count)

	for range count {
		keyPath := pickKey(rng, existingKeys)
		ops = append(ops, merge.Op{
			Type:      merge.OpDeleteValue,
			KeyPath:   keyPath,
			ValueName: fmt.Sprintf("Value%d", rng.Intn(5)),
		})
	}

	return ops
}

// GenerateDeleteKeysLeaf generates OpDeleteKey ops targeting leaf keys (keys
// with no children). This exercises simple key removal from subkey lists.
func GenerateDeleteKeysLeaf(count int, leafKeys [][]string, seed int64) []merge.Op {
	rng := rand.New(rand.NewSource(seed))
	ops := make([]merge.Op, 0, count)

	for range count {
		keyPath := pickKey(rng, leafKeys)
		ops = append(ops, merge.Op{
			Type:    merge.OpDeleteKey,
			KeyPath: keyPath,
		})
	}

	return ops
}

// GenerateDeleteKeysSubtree generates OpDeleteKey ops targeting keys that
// would have 10-50 descendants each. These paths are synthetic (not drawn from
// existingKeys) and exercise recursive subtree deletion.
func GenerateDeleteKeysSubtree(count int, seed int64) []merge.Op {
	rng := rand.New(rand.NewSource(seed))
	ops := make([]merge.Op, 0, count)

	// Use prefixes that match real fixture naming conventions.
	// small-flat: Parent0..Parent9, small-deep: Root0..Root199,
	// medium-mixed: Software, large-wide: Wide0..Wide199,
	// large-realistic: Microsoft, Software
	fixtureRoots := []string{
		"Parent0", "Parent1", "Parent2", "Parent3", "Parent4",
		"Root0", "Root1", "Root2", "Root3",
		"Wide0", "Wide1", "Wide2", "Wide3",
		"Software", "Microsoft",
	}

	for range count {
		root := fixtureRoots[rng.Intn(len(fixtureRoots))]
		depth := 2 + rng.Intn(3)
		path := make([]string, depth)
		path[0] = root
		for j := 1; j < depth; j++ {
			path[j] = fmt.Sprintf("Branch%d", rng.Intn(50))
		}
		ops = append(ops, merge.Op{
			Type:    merge.OpDeleteKey,
			KeyPath: path,
		})
	}
	return ops
}

// GenerateDeleteHeavyMixed generates a mix of 60% delete ops and 40% create
// ops. Delete ops target existing keys and values; create ops add new keys.
// This pattern stresses concurrent allocation and deallocation.
func GenerateDeleteHeavyMixed(count int, existingKeys [][]string, seed int64) []merge.Op {
	rng := rand.New(rand.NewSource(seed))
	ops := make([]merge.Op, 0, count)

	for range count {
		r := rng.Float64()
		switch {
		case r < 0.30:
			// 30%: delete key
			keyPath := pickKey(rng, existingKeys)
			ops = append(ops, merge.Op{
				Type:    merge.OpDeleteKey,
				KeyPath: keyPath,
			})
		case r < 0.60:
			// 30%: delete value
			keyPath := pickKey(rng, existingKeys)
			ops = append(ops, merge.Op{
				Type:      merge.OpDeleteValue,
				KeyPath:   keyPath,
				ValueName: fmt.Sprintf("Value%d", rng.Intn(5)),
			})
		default:
			// 40%: create new key
			depth := 2 + rng.Intn(4)
			path := make([]string, depth)
			path[0] = fmt.Sprintf("New%d", rng.Intn(count*10))
			for j := 1; j < depth; j++ {
				path[j] = fmt.Sprintf("Child%d", rng.Intn(100))
			}
			ops = append(ops, merge.Op{
				Type:    merge.OpEnsureKey,
				KeyPath: path,
			})
		}
	}

	return ops
}

// GenerateMixedRealistic generates a realistic mix of all four operation types:
// 30% create key, 40% set value, 20% delete value, 10% delete key.
// This simulates a typical merge workload from a policy or configuration update.
func GenerateMixedRealistic(count int, existingKeys [][]string, seed int64) []merge.Op {
	rng := rand.New(rand.NewSource(seed))
	ops := make([]merge.Op, 0, count)

	for range count {
		r := rng.Float64()
		switch {
		case r < 0.30:
			// 30%: create new key
			depth := 2 + rng.Intn(4)
			path := make([]string, depth)
			path[0] = fmt.Sprintf("New%d", rng.Intn(count*10))
			for j := 1; j < depth; j++ {
				path[j] = fmt.Sprintf("Sub%d", rng.Intn(100))
			}
			ops = append(ops, merge.Op{
				Type:    merge.OpEnsureKey,
				KeyPath: path,
			})

		case r < 0.70:
			// 40%: update value on existing key
			keyPath := pickKey(rng, existingKeys)
			data := make([]byte, format.DWORDSize)
			binary.LittleEndian.PutUint32(data, rng.Uint32())
			ops = append(ops, merge.Op{
				Type:      merge.OpSetValue,
				KeyPath:   keyPath,
				ValueName: fmt.Sprintf("Value%d", rng.Intn(8)),
				ValueType: format.REGDWORD,
				Data:      data,
			})

		case r < 0.90:
			// 20%: delete value
			keyPath := pickKey(rng, existingKeys)
			ops = append(ops, merge.Op{
				Type:      merge.OpDeleteValue,
				KeyPath:   keyPath,
				ValueName: fmt.Sprintf("Value%d", rng.Intn(8)),
			})

		default:
			// 10%: delete key
			keyPath := pickKey(rng, existingKeys)
			ops = append(ops, merge.Op{
				Type:    merge.OpDeleteKey,
				KeyPath: keyPath,
			})
		}
	}

	return ops
}

// GenerateIdempotentReplay generates OpEnsureKey ops that match existing state.
// When applied to a hive that already contains these keys, the ops are no-ops.
// This measures the overhead of merge path resolution when nothing changes.
func GenerateIdempotentReplay(count int, existingKeys [][]string, seed int64) []merge.Op {
	rng := rand.New(rand.NewSource(seed))
	ops := make([]merge.Op, 0, count)

	for range count {
		keyPath := pickKey(rng, existingKeys)
		ops = append(ops, merge.Op{
			Type:    merge.OpEnsureKey,
			KeyPath: keyPath,
		})
	}

	return ops
}

// GenerateLargeValues generates OpSetValue ops with binary data between 4KB and
// 64KB. This exercises big-data cell allocation and tests how the merge engine
// handles values that exceed the inline storage threshold.
func GenerateLargeValues(count int, seed int64) []merge.Op {
	rng := rand.New(rand.NewSource(seed))
	ops := make([]merge.Op, 0, count)

	for range count {
		// 4KB to 64KB
		size := 4*1024 + rng.Intn(60*1024+1)
		data := make([]byte, size)
		rng.Read(data)

		depth := 2 + rng.Intn(3)
		path := make([]string, depth)
		path[0] = fmt.Sprintf("LargeVal%d", rng.Intn(count*5))
		for j := 1; j < depth; j++ {
			path[j] = fmt.Sprintf("Node%d", rng.Intn(50))
		}

		ops = append(ops, merge.Op{
			Type:      merge.OpSetValue,
			KeyPath:   path,
			ValueName: fmt.Sprintf("BigData%d", rng.Intn(5)),
			ValueType: format.REGBinary,
			Data:      data,
		})
	}

	return ops
}

// GenerateWorstCaseFragmented generates a workload that alternates between
// create and delete cycles, followed by a final batch of creates. The first
// half of ops alternate between OpEnsureKey and OpDeleteKey on the same paths
// to maximize cell fragmentation. The second half creates new keys into the
// fragmented space.
func GenerateWorstCaseFragmented(count int, seed int64) []merge.Op {
	rng := rand.New(rand.NewSource(seed))
	ops := make([]merge.Op, 0, count)

	// First half: paired create/delete cycles on the same paths.
	// Each pair is 2 ops (EnsureKey + DeleteKey on the same path).
	half := count / 2
	pairCount := half / 2
	cyclePaths := make([][]string, pairCount)
	for i := range pairCount {
		depth := 2 + rng.Intn(3)
		path := make([]string, depth)
		path[0] = fmt.Sprintf("Frag%d", rng.Intn(pairCount*5))
		for j := 1; j < depth; j++ {
			path[j] = fmt.Sprintf("Cycle%d", rng.Intn(30))
		}
		cyclePaths[i] = path
	}

	// Emit paired create+delete on the same path to produce fragmentation.
	for i := range pairCount {
		ops = append(ops,
			merge.Op{Type: merge.OpEnsureKey, KeyPath: cyclePaths[i]},
			merge.Op{Type: merge.OpDeleteKey, KeyPath: cyclePaths[i]},
		)
	}

	// Second half: new creates into fragmented space.
	remaining := count - pairCount*2
	for range remaining {
		depth := 2 + rng.Intn(4)
		path := make([]string, depth)
		path[0] = fmt.Sprintf("Fill%d", rng.Intn(remaining*5))
		for j := 1; j < depth; j++ {
			path[j] = fmt.Sprintf("New%d", rng.Intn(100))
		}
		ops = append(ops, merge.Op{
			Type:    merge.OpEnsureKey,
			KeyPath: path,
		})
	}

	return ops
}

// CollectExistingKeys generates key paths matching a fixture's naming pattern.
// It does not read actual hive files; instead, it reproduces the key naming
// conventions used by the fixture generators so that patch generators can
// target paths known to exist.
//
// Supported fixture names: "small-flat", "small-deep", "medium-mixed",
// "large-wide", "large-realistic".
func CollectExistingKeys(fixtureName string, count int, seed int64) [][]string {
	rng := rand.New(rand.NewSource(seed))
	keys := make([][]string, 0, count)

	switch fixtureName {
	case "small-flat":
		// Matches GenerateSmallFlat: Parent{0-9}/Child{0-200}
		for range count {
			parentIdx := rng.Intn(10)
			childIdx := rng.Intn(200)
			keys = append(keys, []string{
				fmt.Sprintf("Parent%d", parentIdx),
				fmt.Sprintf("Child%d", childIdx),
			})
		}

	case "small-deep":
		// Matches GenerateSmallDeep: Root{0-39}/Level1_0/Level2_0/...
		for range count {
			rootIdx := rng.Intn(40)
			depth := 3 + rng.Intn(5) // 3-7 levels into the chain
			path := make([]string, depth)
			path[0] = fmt.Sprintf("Root%d", rootIdx)
			for j := 1; j < depth; j++ {
				path[j] = fmt.Sprintf("Level%d_0", j)
			}
			keys = append(keys, path)
		}

	case "medium-mixed":
		// Matches GenerateMediumMixed: Software/Application{0-29}/Component{0-120}
		for range count {
			appIdx := rng.Intn(30)
			compIdx := rng.Intn(120)
			keys = append(keys, []string{
				"Software",
				fmt.Sprintf("Application%d", appIdx),
				fmt.Sprintf("Component%d", compIdx),
			})
		}

	case "large-wide":
		// Matches GenerateLargeWide: Wide{0-19}/Entry{0-500}
		for range count {
			wideIdx := rng.Intn(20)
			entryIdx := rng.Intn(500)
			keys = append(keys, []string{
				fmt.Sprintf("Wide%d", wideIdx),
				fmt.Sprintf("Entry%d", entryIdx),
			})
		}

	case "large-realistic":
		// Matches GenerateLargeRealistic: mixed sections
		for range count {
			section := rng.Intn(3)
			switch section {
			case 0:
				// Microsoft products
				products := []string{
					"Windows", "Office", "VisualStudio", "SQLServer", "Exchange",
					"Azure", "Edge", "Teams", "OneDrive", "Defender",
				}
				product := products[rng.Intn(len(products))]
				compIdx := rng.Intn(200)
				keys = append(keys, []string{
					"Microsoft",
					product,
					fmt.Sprintf("Component%d", compIdx),
				})
			case 1:
				// Third-party vendors
				vendorIdx := rng.Intn(100)
				prodIdx := rng.Intn(50)
				keys = append(keys, []string{
					"Software",
					fmt.Sprintf("Vendor%d", vendorIdx),
					fmt.Sprintf("Product%d", prodIdx),
				})
			default:
				// Policies
				policyTypes := []string{"Machine", "User", "Default"}
				pType := policyTypes[rng.Intn(len(policyTypes))]
				policyIdx := rng.Intn(300)
				keys = append(keys, []string{
					"Policies",
					pType,
					fmt.Sprintf("Policy%d", policyIdx),
				})
			}
		}

	default:
		// Unknown fixture: generate generic paths.
		for range count {
			keys = append(keys, []string{
				fmt.Sprintf("Key%d", rng.Intn(count*10)),
				fmt.Sprintf("Sub%d", rng.Intn(100)),
			})
		}
	}

	return keys
}

// pickKey selects a random key path from the provided set.
// Returns nil if keys is empty.
func pickKey(rng *rand.Rand, keys [][]string) []string {
	if len(keys) == 0 {
		return nil
	}
	idx := rng.Intn(len(keys))
	// Return a copy to avoid mutation.
	result := make([]string, len(keys[idx]))
	copy(result, keys[idx])
	return result
}

// encodeUTF16LE encodes an ASCII string as NUL-terminated UTF-16LE.
// Only handles ASCII characters. Sufficient for benchmark fixture generation.
func encodeUTF16LE(s string) []byte {
	// Simple ASCII-to-UTF16LE: each byte becomes 2 bytes (low, 0x00).
	// Add null terminator (2 bytes).
	buf := make([]byte, (len(s)+1)*2)
	for i := range len(s) {
		buf[i*2] = s[i]
		buf[i*2+1] = 0
	}
	// Null terminator is already zero from make.
	return buf
}
