package benchmark

import (
	"fmt"
	"math/rand"
	"path/filepath"

	"github.com/joshuapare/hivekit/hive/builder"
)

// FixtureSet holds paths to all generated benchmark fixture hive files.
type FixtureSet struct {
	SmallFlat      string
	SmallDeep      string
	MediumMixed    string
	LargeWide      string
	LargeRealistic string
}

// Deterministic seeds for reproducibility.
const (
	seedSmallFlat      = 42
	seedSmallDeep      = 43
	seedMediumMixed    = 44
	seedLargeWide      = 45
	seedLargeRealistic = 46
)

// GenerateSmallFlat generates a ~5MB hive with a flat structure.
// Structure: 50-200 subkeys per parent, 3-5 depth levels, ~2K total keys.
// Each key gets 2-4 values of varying types.
func GenerateSmallFlat(path string) error {
	rng := rand.New(rand.NewSource(seedSmallFlat))

	opts := builder.DefaultOptions()
	opts.AutoFlushThreshold = 500

	b, err := builder.New(path, opts)
	if err != nil {
		return fmt.Errorf("create builder: %w", err)
	}
	defer b.Close()

	// Generate a flat tree: few top-level parents with many children.
	// Target: ~2K keys across 3-5 levels with 50-200 subkeys per parent.
	topLevelCount := 10
	for i := 0; i < topLevelCount; i++ {
		parentPath := []string{fmt.Sprintf("Parent%d", i)}
		if err := b.EnsureKey(parentPath); err != nil {
			return fmt.Errorf("ensure parent key: %w", err)
		}

		// Each parent gets 50-200 subkeys
		subkeyCount := 50 + rng.Intn(151)
		for j := 0; j < subkeyCount; j++ {
			childPath := append([]string{}, parentPath...)
			childPath = append(childPath, fmt.Sprintf("Child%d", j))

			if err := b.EnsureKey(childPath); err != nil {
				return fmt.Errorf("ensure child key: %w", err)
			}

			// Add 3-5 values per key
			if err := addRandomValues(b, childPath, 3+rng.Intn(3), rng); err != nil {
				return fmt.Errorf("add values: %w", err)
			}

			// Some children get their own subkeys for depth 3-5
			if rng.Intn(4) == 0 {
				grandchildCount := 5 + rng.Intn(10)
				for k := 0; k < grandchildCount; k++ {
					gcPath := append([]string{}, childPath...)
					gcPath = append(gcPath, fmt.Sprintf("Sub%d", k))

					if err := b.EnsureKey(gcPath); err != nil {
						return fmt.Errorf("ensure grandchild key: %w", err)
					}

					if err := addRandomValues(b, gcPath, 3+rng.Intn(3), rng); err != nil {
						return fmt.Errorf("add values: %w", err)
					}

					// Occasional level 4-5 depth
					if rng.Intn(3) == 0 {
						deepPath := append([]string{}, gcPath...)
						deepPath = append(deepPath, fmt.Sprintf("Deep%d", k))
						if err := b.EnsureKey(deepPath); err != nil {
							return fmt.Errorf("ensure deep key: %w", err)
						}
						if err := addRandomValues(b, deepPath, 3+rng.Intn(2), rng); err != nil {
							return fmt.Errorf("add values: %w", err)
						}
					}
				}
			}
		}
	}

	return b.Commit()
}

// GenerateSmallDeep generates a ~5MB hive with a deep structure.
// Structure: 3-5 subkeys per parent, 10-15 depth levels, ~2K total keys.
// Each key gets 3-5 values of varying types.
func GenerateSmallDeep(path string) error {
	rng := rand.New(rand.NewSource(seedSmallDeep))

	opts := builder.DefaultOptions()
	opts.AutoFlushThreshold = 500

	b, err := builder.New(path, opts)
	if err != nil {
		return fmt.Errorf("create builder: %w", err)
	}
	defer b.Close()

	// Generate deep chains: many independent chains, each going 10-15 deep.
	// At each level, one child continues the deep chain, while 2-4 siblings are
	// leaf keys. This produces depth without exponential key count growth.
	// Target: ~2K keys with 3-5 subkeys per parent, 10-15 depth.
	chainCount := 40
	for i := 0; i < chainCount; i++ {
		rootPath := []string{fmt.Sprintf("Root%d", i)}
		if err := generateDeepChain(b, rootPath, 10+rng.Intn(6), rng); err != nil {
			return fmt.Errorf("generate deep chain %d: %w", i, err)
		}
	}

	return b.Commit()
}

// generateDeepChain creates a deep key hierarchy using a linear-with-siblings
// approach: at each level, one child continues the deep chain while 2-4 siblings
// are leaf keys. This produces depth without exponential key count growth.
func generateDeepChain(b *builder.Builder, currentPath []string, remainingDepth int, rng *rand.Rand) error {
	if err := b.EnsureKey(currentPath); err != nil {
		return err
	}

	// Add values at every key
	if err := addRandomValues(b, currentPath, 3+rng.Intn(3), rng); err != nil {
		return err
	}

	if remainingDepth <= 0 {
		return nil
	}

	// Create 3-5 children: only the first one continues the chain.
	// The rest are leaf keys (no further depth), providing width at each level.
	siblingCount := 3 + rng.Intn(3)
	for i := 0; i < siblingCount; i++ {
		childPath := make([]string, len(currentPath)+1)
		copy(childPath, currentPath)
		childPath[len(currentPath)] = fmt.Sprintf("Level%d_%d", len(currentPath), i)

		if i == 0 {
			// First child continues the deep chain
			if err := generateDeepChain(b, childPath, remainingDepth-1, rng); err != nil {
				return err
			}
		} else {
			// Siblings are leaf keys
			if err := b.EnsureKey(childPath); err != nil {
				return err
			}
			if err := addRandomValues(b, childPath, 3+rng.Intn(3), rng); err != nil {
				return err
			}
		}
	}

	return nil
}

// GenerateMediumMixed generates a ~50MB hive with a mixed structure.
// Structure: Realistic tree with 5-8 values per key, ~20K total keys.
// Uses a mix of flat and deep patterns to simulate a typical software hive.
func GenerateMediumMixed(path string) error {
	rng := rand.New(rand.NewSource(seedMediumMixed))

	opts := builder.DefaultOptions()
	opts.AutoFlushThreshold = 1000

	b, err := builder.New(path, opts)
	if err != nil {
		return fmt.Errorf("create builder: %w", err)
	}
	defer b.Close()

	// Create a realistic mixed structure with ~20K keys.
	// Top level: 30 "application" keys, each with varied substructure.
	appCount := 30
	for i := 0; i < appCount; i++ {
		appPath := []string{"Software", fmt.Sprintf("Application%d", i)}
		if err := b.EnsureKey(appPath); err != nil {
			return fmt.Errorf("ensure app key: %w", err)
		}

		// Each app gets 40-120 subkeys with mixed structure
		subkeyCount := 40 + rng.Intn(81)
		for j := 0; j < subkeyCount; j++ {
			componentPath := append([]string{}, appPath...)
			componentPath = append(componentPath, fmt.Sprintf("Component%d", j))

			if err := b.EnsureKey(componentPath); err != nil {
				return fmt.Errorf("ensure component key: %w", err)
			}

			// 5-8 values per key
			if err := addRandomValues(b, componentPath, 5+rng.Intn(4), rng); err != nil {
				return fmt.Errorf("add component values: %w", err)
			}

			// Many components have settings subkeys
			if rng.Intn(2) == 0 {
				settingsCount := 5 + rng.Intn(20)
				for k := 0; k < settingsCount; k++ {
					settingsPath := append([]string{}, componentPath...)
					settingsPath = append(settingsPath, fmt.Sprintf("Setting%d", k))

					if err := b.EnsureKey(settingsPath); err != nil {
						return fmt.Errorf("ensure settings key: %w", err)
					}

					if err := addRandomValues(b, settingsPath, 5+rng.Intn(4), rng); err != nil {
						return fmt.Errorf("add settings values: %w", err)
					}

					// Occasional deeper nesting
					if rng.Intn(3) == 0 {
						for d := 0; d < 3+rng.Intn(5); d++ {
							deepPath := append([]string{}, settingsPath...)
							deepPath = append(deepPath, fmt.Sprintf("Detail%d", d))

							if err := b.EnsureKey(deepPath); err != nil {
								return fmt.Errorf("ensure detail key: %w", err)
							}

							if err := addRandomValues(b, deepPath, 5+rng.Intn(4), rng); err != nil {
								return fmt.Errorf("add detail values: %w", err)
							}
						}
					}
				}
			}
		}
	}

	// Add a "Classes" section with many flat entries (mimics CLSID)
	for i := 0; i < 2000; i++ {
		clsPath := []string{"Classes", fmt.Sprintf("{%08X-%04X-%04X-%04X-%012X}",
			rng.Uint32(), rng.Intn(0xFFFF), rng.Intn(0xFFFF),
			rng.Intn(0xFFFF), rng.Int63n(0xFFFFFFFFFFFF))}

		if err := b.EnsureKey(clsPath); err != nil {
			return fmt.Errorf("ensure CLSID key: %w", err)
		}
		if err := addRandomValues(b, clsPath, 4+rng.Intn(4), rng); err != nil {
			return fmt.Errorf("add CLSID values: %w", err)
		}

		// Some CLSIDs have InprocServer32 subkeys
		if rng.Intn(3) == 0 {
			serverPath := append([]string{}, clsPath...)
			serverPath = append(serverPath, "InprocServer32")
			if err := b.EnsureKey(serverPath); err != nil {
				return fmt.Errorf("ensure server key: %w", err)
			}
			if err := b.SetString(serverPath, "", fmt.Sprintf("C:\\Windows\\System32\\component%d.dll", i)); err != nil {
				return fmt.Errorf("set server path: %w", err)
			}
			if err := b.SetString(serverPath, "ThreadingModel", "Both"); err != nil {
				return fmt.Errorf("set threading model: %w", err)
			}
		}
	}

	return b.Commit()
}

// GenerateLargeWide generates a ~200MB hive with a wide structure.
// Structure: 200-500 subkeys per parent, ~100K total keys.
// Each key gets 2-3 values.
func GenerateLargeWide(path string) error {
	rng := rand.New(rand.NewSource(seedLargeWide))

	opts := builder.DefaultOptions()
	opts.AutoFlushThreshold = 2000

	b, err := builder.New(path, opts)
	if err != nil {
		return fmt.Errorf("create builder: %w", err)
	}
	defer b.Close()

	// Wide structure: 20 parent keys, each with 200-500 direct children.
	// Some children have their own wide subtrees.
	parentCount := 20
	for i := 0; i < parentCount; i++ {
		parentPath := []string{fmt.Sprintf("Wide%d", i)}
		if err := b.EnsureKey(parentPath); err != nil {
			return fmt.Errorf("ensure parent key: %w", err)
		}

		childCount := 200 + rng.Intn(301)
		for j := 0; j < childCount; j++ {
			childPath := append([]string{}, parentPath...)
			childPath = append(childPath, fmt.Sprintf("Entry%d", j))

			if err := b.EnsureKey(childPath); err != nil {
				return fmt.Errorf("ensure child key: %w", err)
			}

			if err := addRandomValues(b, childPath, 2+rng.Intn(2), rng); err != nil {
				return fmt.Errorf("add values: %w", err)
			}

			// 10% of children get their own wide subtree
			if rng.Intn(10) == 0 {
				subChildCount := 50 + rng.Intn(100)
				for k := 0; k < subChildCount; k++ {
					subPath := append([]string{}, childPath...)
					subPath = append(subPath, fmt.Sprintf("Sub%d", k))

					if err := b.EnsureKey(subPath); err != nil {
						return fmt.Errorf("ensure sub key: %w", err)
					}

					if err := addRandomValues(b, subPath, 2+rng.Intn(2), rng); err != nil {
						return fmt.Errorf("add sub values: %w", err)
					}
				}
			}
		}
	}

	return b.Commit()
}

// GenerateLargeRealistic generates a ~500MB hive mimicking a real Windows SOFTWARE hive.
// Structure: Mixed depths and widths, 200K+ total keys with varied value types.
// This fixture mirrors the structure of a real Windows SOFTWARE registry hive with
// typical key hierarchies found in production systems.
func GenerateLargeRealistic(path string) error {
	rng := rand.New(rand.NewSource(seedLargeRealistic))

	opts := builder.DefaultOptions()
	opts.AutoFlushThreshold = 2000

	b, err := builder.New(path, opts)
	if err != nil {
		return fmt.Errorf("create builder: %w", err)
	}
	defer b.Close()

	// Mimic a real SOFTWARE hive structure
	// Section 1: Classes with CLSID entries (very wide, flat)
	for i := 0; i < 5000; i++ {
		clsPath := []string{"Classes", fmt.Sprintf("{%08X-%04X-%04X-%04X-%012X}",
			rng.Uint32(), rng.Intn(0xFFFF), rng.Intn(0xFFFF),
			rng.Intn(0xFFFF), rng.Int63n(0xFFFFFFFFFFFF))}

		if err := b.EnsureKey(clsPath); err != nil {
			return fmt.Errorf("ensure CLSID key: %w", err)
		}
		if err := addRandomValues(b, clsPath, 2+rng.Intn(4), rng); err != nil {
			return fmt.Errorf("add CLSID values: %w", err)
		}

		// Some CLSIDs have InprocServer32, LocalServer32 subkeys
		if rng.Intn(3) == 0 {
			serverPath := append([]string{}, clsPath...)
			serverPath = append(serverPath, "InprocServer32")
			if err := b.EnsureKey(serverPath); err != nil {
				return fmt.Errorf("ensure server key: %w", err)
			}
			if err := b.SetString(serverPath, "", fmt.Sprintf("C:\\Windows\\System32\\component%d.dll", i)); err != nil {
				return fmt.Errorf("set server path: %w", err)
			}
			if err := b.SetString(serverPath, "ThreadingModel", "Both"); err != nil {
				return fmt.Errorf("set threading model: %w", err)
			}
		}
	}

	// Section 2: Microsoft subtree (deep, mixed)
	msProducts := []string{
		"Windows", "Office", "VisualStudio", "SQLServer", "Exchange",
		"Azure", "Edge", "Teams", "OneDrive", "Defender",
	}
	for _, product := range msProducts {
		productPath := []string{"Microsoft", product}
		if err := b.EnsureKey(productPath); err != nil {
			return fmt.Errorf("ensure product key: %w", err)
		}

		// Each product has 50-200 components
		componentCount := 50 + rng.Intn(151)
		for j := 0; j < componentCount; j++ {
			compPath := append([]string{}, productPath...)
			compPath = append(compPath, fmt.Sprintf("Component%d", j))

			if err := b.EnsureKey(compPath); err != nil {
				return fmt.Errorf("ensure component: %w", err)
			}
			if err := addRandomValues(b, compPath, 5+rng.Intn(6), rng); err != nil {
				return fmt.Errorf("add component values: %w", err)
			}

			// Deep settings hierarchy
			if rng.Intn(4) == 0 {
				if err := generateMixedSubtree(b, compPath, 3+rng.Intn(4), rng); err != nil {
					return fmt.Errorf("generate subtree: %w", err)
				}
			}
		}
	}

	// Section 3: Third-party applications
	for i := 0; i < 100; i++ {
		vendorPath := []string{"Software", fmt.Sprintf("Vendor%d", i)}
		if err := b.EnsureKey(vendorPath); err != nil {
			return fmt.Errorf("ensure vendor key: %w", err)
		}

		// Each vendor has 10-50 products
		productCount := 10 + rng.Intn(41)
		for j := 0; j < productCount; j++ {
			prodPath := append([]string{}, vendorPath...)
			prodPath = append(prodPath, fmt.Sprintf("Product%d", j))

			if err := b.EnsureKey(prodPath); err != nil {
				return fmt.Errorf("ensure product key: %w", err)
			}
			if err := addRandomValues(b, prodPath, 4+rng.Intn(5), rng); err != nil {
				return fmt.Errorf("add product values: %w", err)
			}

			// Settings subtree
			if rng.Intn(3) == 0 {
				if err := generateMixedSubtree(b, prodPath, 2+rng.Intn(3), rng); err != nil {
					return fmt.Errorf("generate product subtree: %w", err)
				}
			}
		}
	}

	// Section 4: Policies (moderately deep)
	policyTypes := []string{"Machine", "User", "Default"}
	for _, pType := range policyTypes {
		policyPath := []string{"Policies", pType}
		if err := b.EnsureKey(policyPath); err != nil {
			return fmt.Errorf("ensure policy key: %w", err)
		}

		// Each policy type has 100-300 entries
		entryCount := 100 + rng.Intn(201)
		for j := 0; j < entryCount; j++ {
			entryPath := append([]string{}, policyPath...)
			entryPath = append(entryPath, fmt.Sprintf("Policy%d", j))

			if err := b.EnsureKey(entryPath); err != nil {
				return fmt.Errorf("ensure policy entry: %w", err)
			}
			if err := addRandomValues(b, entryPath, 3+rng.Intn(4), rng); err != nil {
				return fmt.Errorf("add policy values: %w", err)
			}
		}
	}

	return b.Commit()
}

// generateMixedSubtree creates a subtree with mixed depth and width.
func generateMixedSubtree(b *builder.Builder, basePath []string, depth int, rng *rand.Rand) error {
	if depth <= 0 {
		return nil
	}

	childCount := 3 + rng.Intn(8)
	for i := 0; i < childCount; i++ {
		childPath := make([]string, len(basePath)+1)
		copy(childPath, basePath)
		childPath[len(basePath)] = fmt.Sprintf("Node%d", i)

		if err := b.EnsureKey(childPath); err != nil {
			return err
		}
		if err := addRandomValues(b, childPath, 3+rng.Intn(5), rng); err != nil {
			return err
		}

		// Recurse with decreasing probability
		if rng.Intn(3) < 2 {
			if err := generateMixedSubtree(b, childPath, depth-1, rng); err != nil {
				return err
			}
		}
	}

	return nil
}

// addRandomValues adds a specified number of random values of varying types to a key.
func addRandomValues(b *builder.Builder, path []string, count int, rng *rand.Rand) error {
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("Value%d", i)

		switch rng.Intn(3) {
		case 0: // String value
			if err := b.SetString(path, name, randomString(rng, 10+rng.Intn(90))); err != nil {
				return err
			}
		case 1: // DWORD value
			if err := b.SetDWORD(path, name, rng.Uint32()); err != nil {
				return err
			}
		case 2: // Binary value
			data := make([]byte, 16+rng.Intn(240))
			rng.Read(data)
			if err := b.SetBinary(path, name, data); err != nil {
				return err
			}
		}
	}
	return nil
}

// randomString generates a deterministic random ASCII string of the given length.
func randomString(rng *rand.Rand, length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-."
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[rng.Intn(len(charset))]
	}
	return string(result)
}

// GenerateAll generates all benchmark fixtures in the specified directory.
// Returns a FixtureSet with paths to all generated files.
func GenerateAll(dir string) (*FixtureSet, error) {
	fs := &FixtureSet{
		SmallFlat:      filepath.Join(dir, "small-flat.hive"),
		SmallDeep:      filepath.Join(dir, "small-deep.hive"),
		MediumMixed:    filepath.Join(dir, "medium-mixed.hive"),
		LargeWide:      filepath.Join(dir, "large-wide.hive"),
		LargeRealistic: filepath.Join(dir, "large-realistic.hive"),
	}

	generators := []struct {
		name string
		fn   func(string) error
		path string
	}{
		{"small-flat", GenerateSmallFlat, fs.SmallFlat},
		{"small-deep", GenerateSmallDeep, fs.SmallDeep},
		{"medium-mixed", GenerateMediumMixed, fs.MediumMixed},
		{"large-wide", GenerateLargeWide, fs.LargeWide},
		{"large-realistic", GenerateLargeRealistic, fs.LargeRealistic},
	}

	for _, g := range generators {
		if err := g.fn(g.path); err != nil {
			return nil, fmt.Errorf("generate %s: %w", g.name, err)
		}
	}

	return fs, nil
}
