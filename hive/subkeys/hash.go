package subkeys

import "unicode"

const (
	// hashMultiplier is the multiplier used in the Windows Registry hash algorithm.
	// The hash algorithm is: hash = hash * 37 + toupper(char) for each character.
	hashMultiplier = 37
)

// Hash computes the Windows Registry hash for a key name.
// This hash is used in LH (Leaf-Hash) subkey list entries.
//
// Algorithm: hash = 0; for each char: hash = hash * 37 + toupper(char)
// Note: The input name should already be lowercased, but the hash
// algorithm uppercases each character during computation.
func Hash(name string) uint32 {
	var hash uint32
	for _, r := range name {
		// Windows uses uppercase characters for hashing
		hash = hash*hashMultiplier + uint32(unicode.ToUpper(r))
	}
	return hash
}
