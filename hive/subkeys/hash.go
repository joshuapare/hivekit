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
//
// Uses a fast ASCII path for the common case (99%+ of registry key names).
// Falls back to unicode.ToUpper for non-ASCII characters.
func Hash(name string) uint32 {
	var hash uint32
	for i := 0; i < len(name); i++ {
		b := name[i]
		if b > 0x7F {
			// Non-ASCII byte encountered; fall back to full Unicode path
			// which re-hashes from the beginning using rune iteration.
			return hashUnicode(name)
		}
		if b >= 'a' && b <= 'z' {
			b -= 32
		}
		hash = hash*hashMultiplier + uint32(b)
	}
	return hash
}

// hashUnicode is the fallback for names containing non-ASCII characters.
func hashUnicode(name string) uint32 {
	var hash uint32
	for _, r := range name {
		hash = hash*hashMultiplier + uint32(unicode.ToUpper(r))
	}
	return hash
}
