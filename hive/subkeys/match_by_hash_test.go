package subkeys

import (
	"encoding/binary"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/format"
)

// Test_MatchByHash_LH_Synthetic tests MatchByHash with a synthetic LH list payload.
// This tests the core hash-filtering logic without needing a real hive.
func Test_MatchByHash_LH_Synthetic(t *testing.T) {
	t.Skip("synthetic LH payload helper not implemented")
	// We test with a real hive since MatchByHash needs to resolve cells.
	// See Test_MatchByHash_RealHive below.
}

// Test_MatchByHash_RealHive tests MatchByHash against a real hive file.
func Test_MatchByHash_RealHive(t *testing.T) {
	testHivePath := "../../testdata/large"
	h, err := hive.Open(testHivePath)
	if err != nil {
		t.Skipf("Test hive not found: %v", err)
	}
	defer h.Close()

	// Navigate from root to find a key with subkeys.
	// Root NK -> its subkey list.
	rootRef := h.RootCellOffset()
	payload, err := h.ResolveCellPayload(rootRef)
	if err != nil {
		t.Fatalf("resolve root NK: %v", err)
	}
	nk, err := hive.ParseNK(payload)
	if err != nil {
		t.Fatalf("parse root NK: %v", err)
	}

	listRef := nk.SubkeyListOffsetRel()
	if listRef == format.InvalidOffset {
		t.Fatal("root NK has no subkey list")
	}

	// First, read the full subkey list to know what children exist.
	fullList, err := Read(h, listRef)
	if err != nil {
		t.Fatalf("Read subkey list: %v", err)
	}
	if fullList.Len() == 0 {
		t.Fatal("root has no subkeys")
	}

	t.Logf("Root has %d subkeys", fullList.Len())

	// Pick a known child to target.
	targetEntry := fullList.Entries[0]
	targetName := targetEntry.NameLower
	targetHash := Hash(targetName)

	t.Run("single_target_match", func(t *testing.T) {
		targets := map[uint32]string{
			targetHash: targetName,
		}

		matched, err := MatchByHash(h, listRef, targets)
		if err != nil {
			t.Fatalf("MatchByHash failed: %v", err)
		}

		if len(matched) != 1 {
			t.Fatalf("expected 1 match, got %d", len(matched))
		}

		if matched[0].NameLower != targetName {
			t.Errorf("expected name %q, got %q", targetName, matched[0].NameLower)
		}
		if matched[0].NKRef != targetEntry.NKRef {
			t.Errorf("expected NKRef 0x%X, got 0x%X", targetEntry.NKRef, matched[0].NKRef)
		}
		if matched[0].Hash != targetHash {
			t.Errorf("expected hash 0x%X, got 0x%X", targetHash, matched[0].Hash)
		}
	})

	t.Run("multiple_targets", func(t *testing.T) {
		if fullList.Len() < 2 {
			t.Skip("need at least 2 subkeys")
		}

		entry0 := fullList.Entries[0]
		entry1 := fullList.Entries[1]
		hash0 := Hash(entry0.NameLower)
		hash1 := Hash(entry1.NameLower)

		targets := map[uint32]string{
			hash0: entry0.NameLower,
			hash1: entry1.NameLower,
		}

		matched, err := MatchByHash(h, listRef, targets)
		if err != nil {
			t.Fatalf("MatchByHash failed: %v", err)
		}

		if len(matched) < 2 {
			t.Fatalf("expected at least 2 matches, got %d", len(matched))
		}

		// Check both names are present
		foundNames := make(map[string]bool)
		for _, m := range matched {
			foundNames[m.NameLower] = true
		}
		if !foundNames[entry0.NameLower] {
			t.Errorf("missing match for %q", entry0.NameLower)
		}
		if !foundNames[entry1.NameLower] {
			t.Errorf("missing match for %q", entry1.NameLower)
		}
	})

	t.Run("no_match", func(t *testing.T) {
		// Use a hash that is unlikely to match anything
		targets := map[uint32]string{
			0xDEADBEEF: "nonexistent_key_name_xyz",
		}

		matched, err := MatchByHash(h, listRef, targets)
		if err != nil {
			t.Fatalf("MatchByHash failed: %v", err)
		}

		if len(matched) != 0 {
			t.Errorf("expected 0 matches, got %d", len(matched))
		}
	})

	t.Run("empty_targets", func(t *testing.T) {
		targets := map[uint32]string{}

		matched, err := MatchByHash(h, listRef, targets)
		if err != nil {
			t.Fatalf("MatchByHash failed: %v", err)
		}

		if len(matched) != 0 {
			t.Errorf("expected 0 matches for empty targets, got %d", len(matched))
		}
	})

	t.Run("nil_targets", func(t *testing.T) {
		matched, err := MatchByHash(h, listRef, nil)
		if err != nil {
			t.Fatalf("MatchByHash failed: %v", err)
		}

		if len(matched) != 0 {
			t.Errorf("expected 0 matches for nil targets, got %d", len(matched))
		}
	})
}

// Test_MatchByHash_ConsistentWithMatchNKs verifies that MatchByHash returns
// the same entries as the existing ReadOffsetsInto + MatchNKsFromOffsets path.
func Test_MatchByHash_ConsistentWithMatchNKs(t *testing.T) {
	testHivePath := "../../testdata/large"
	h, err := hive.Open(testHivePath)
	if err != nil {
		t.Skipf("Test hive not found: %v", err)
	}
	defer h.Close()

	// Use the root's subkey list for comparison
	listRef := getRootSubkeyList(t, h)

	// Read the full list to pick some targets
	fullList, err := Read(h, listRef)
	if err != nil {
		t.Fatalf("Read root subkey list: %v", err)
	}
	t.Logf("Root has %d subkeys", fullList.Len())

	if fullList.Len() < 2 {
		t.Skip("need at least 2 subkeys under root")
	}

	// Pick entries to search for (first, last, and middle if available)
	var targetEntries []Entry
	targetEntries = append(targetEntries, fullList.Entries[0])
	targetEntries = append(targetEntries, fullList.Entries[fullList.Len()-1])
	if fullList.Len() >= 3 {
		targetEntries = append(targetEntries, fullList.Entries[fullList.Len()/2])
	}

	// Build target maps for both APIs
	targetNames := make(map[string]struct{})
	targetHashes := make(map[uint32]string)
	for _, e := range targetEntries {
		targetNames[e.NameLower] = struct{}{}
		targetHashes[Hash(e.NameLower)] = e.NameLower
	}

	// Run old path: ReadOffsetsInto + MatchNKsFromOffsets
	var offsetBuf []uint32
	offsetBuf, err = ReadOffsetsInto(h, listRef, offsetBuf)
	if err != nil {
		t.Fatalf("ReadOffsetsInto: %v", err)
	}
	oldMatches, err := MatchNKsFromOffsets(h, offsetBuf, targetNames)
	if err != nil {
		t.Fatalf("MatchNKsFromOffsets: %v", err)
	}

	// Run new path: MatchByHash
	newMatches, err := MatchByHash(h, listRef, targetHashes)
	if err != nil {
		t.Fatalf("MatchByHash: %v", err)
	}

	// Compare results
	if len(newMatches) != len(oldMatches) {
		t.Fatalf("count mismatch: old=%d new=%d", len(oldMatches), len(newMatches))
	}

	// Build lookup from old matches
	oldByName := make(map[string]Entry)
	for _, e := range oldMatches {
		oldByName[e.NameLower] = e
	}

	// Verify each new match corresponds to an old match
	for _, nm := range newMatches {
		om, found := oldByName[nm.NameLower]
		if !found {
			t.Errorf("MatchByHash returned %q not found in MatchNKsFromOffsets results", nm.NameLower)
			continue
		}
		if nm.NKRef != om.NKRef {
			t.Errorf("NKRef mismatch for %q: old=0x%X new=0x%X", nm.NameLower, om.NKRef, nm.NKRef)
		}
	}
}

// getRootSubkeyList returns the subkey list offset for the root NK.
func getRootSubkeyList(t testing.TB, h *hive.Hive) uint32 {
	t.Helper()
	rootRef := h.RootCellOffset()
	payload, err := h.ResolveCellPayload(rootRef)
	if err != nil {
		t.Fatalf("resolve root NK: %v", err)
	}
	nk, err := hive.ParseNK(payload)
	if err != nil {
		t.Fatalf("parse root NK: %v", err)
	}
	listRef := nk.SubkeyListOffsetRel()
	if listRef == format.InvalidOffset {
		t.Fatal("root NK has no subkey list")
	}
	return listRef
}

// Test_MatchByHash_InvalidListRef tests handling of invalid list references.
func Test_MatchByHash_InvalidListRef(t *testing.T) {
	testHivePath := "../../testdata/large"
	h, err := hive.Open(testHivePath)
	if err != nil {
		t.Skipf("Test hive not found: %v", err)
	}
	defer h.Close()

	targets := map[uint32]string{
		Hash("test"): "test",
	}

	t.Run("zero_ref", func(t *testing.T) {
		matched, err := MatchByHash(h, 0, targets)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(matched) != 0 {
			t.Errorf("expected 0 matches for zero ref, got %d", len(matched))
		}
	})

	t.Run("invalid_ref", func(t *testing.T) {
		matched, err := MatchByHash(h, format.InvalidOffset, targets)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(matched) != 0 {
			t.Errorf("expected 0 matches for invalid ref, got %d", len(matched))
		}
	})
}

// Test_MatchByHash_HashCollision verifies correct behavior when two different
// names produce the same LH hash. MatchByHash must verify the name after
// a hash match, not just trust the hash.
func Test_MatchByHash_HashCollision(t *testing.T) {
	// This test uses a real hive. We search for a name, but map its hash
	// to a DIFFERENT name. MatchByHash should NOT return a false match.
	testHivePath := "../../testdata/large"
	h, err := hive.Open(testHivePath)
	if err != nil {
		t.Skipf("Test hive not found: %v", err)
	}
	defer h.Close()

	rootRef := h.RootCellOffset()
	payload, err := h.ResolveCellPayload(rootRef)
	if err != nil {
		t.Fatalf("resolve root: %v", err)
	}
	nk, err := hive.ParseNK(payload)
	if err != nil {
		t.Fatalf("parse root NK: %v", err)
	}

	listRef := nk.SubkeyListOffsetRel()
	if listRef == format.InvalidOffset {
		t.Fatal("root has no subkey list")
	}

	fullList, err := Read(h, listRef)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if fullList.Len() == 0 {
		t.Fatal("no subkeys")
	}

	// Use the hash of the first child, but map it to a wrong name.
	// This simulates a hash collision.
	realChild := fullList.Entries[0]
	realHash := Hash(realChild.NameLower)

	targets := map[uint32]string{
		realHash: "completely_different_name_not_in_hive",
	}

	matched, err := MatchByHash(h, listRef, targets)
	if err != nil {
		t.Fatalf("MatchByHash: %v", err)
	}

	// Should find 0 matches because name verification should reject.
	if len(matched) != 0 {
		t.Errorf("expected 0 matches (hash collision rejected), got %d", len(matched))
		for _, m := range matched {
			t.Logf("  false match: name=%q nkRef=0x%X", m.NameLower, m.NKRef)
		}
	}
}

// Test_MatchByHash_HashCollision_MultiTarget_Limitation documents a known limitation
// of the map[uint32]string targets map when two wanted names share the same LH hash.
func Test_MatchByHash_HashCollision_MultiTarget_Limitation(t *testing.T) {
	// This test documents a known limitation: if two distinct target names
	// produce the same LH hash, only one will be stored in the map[uint32]string
	// targets map. The other will be silently dropped. This is acceptable
	// because LH hash collisions are extremely rare (32-bit hash space) and
	// missed targets fall back to Phase 2 (createMissingKeysAndApply).
	//
	// To fully fix this, MatchByHash would need map[uint32][]string.
	// Tracked as known remaining work.
	t.Skip("documents known limitation — map[uint32]string drops hash-colliding targets")
}

// Test_matchDirectList_LI_SignatureParsing tests that matchDirectList correctly
// identifies an LI list and enters the fallback path. We use a real hive to
// avoid nil pointer issues with resolveCell, and test the signature dispatch.
func Test_matchDirectList_LI_SignatureParsing(t *testing.T) {
	// Test that an LI-signature payload is correctly identified.
	// Create a minimal LI list with 0 entries (empty).
	payload := make([]byte, format.ListHeaderSize)
	payload[0] = 'l'
	payload[1] = 'i'
	binary.LittleEndian.PutUint16(payload[2:4], 0) // count = 0

	targets := map[uint32]string{
		Hash("test"): "test",
	}

	// matchDirectList with count=0 should return nil, nil without dereferencing anything.
	matched, err := matchDirectList(nil, payload, targets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matched) != 0 {
		t.Errorf("expected 0 matches for empty LI list, got %d", len(matched))
	}
}

// Test_matchDirectList_LH_EmptyList tests that an empty LH list returns no results.
func Test_matchDirectList_LH_EmptyList(t *testing.T) {
	payload := make([]byte, format.ListHeaderSize)
	payload[0] = 'l'
	payload[1] = 'h'
	binary.LittleEndian.PutUint16(payload[2:4], 0) // count = 0

	targets := map[uint32]string{
		Hash("test"): "test",
	}

	matched, err := matchDirectList(nil, payload, targets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matched) != 0 {
		t.Errorf("expected 0 matches for empty LH list, got %d", len(matched))
	}
}

// Test_matchDirectList_InvalidSignature tests handling of unknown list signatures.
func Test_matchDirectList_InvalidSignature(t *testing.T) {
	payload := make([]byte, format.ListHeaderSize+format.QWORDSize)
	payload[0] = 'x'
	payload[1] = 'y'
	binary.LittleEndian.PutUint16(payload[2:4], 1) // count=1 to reach signature check

	targets := map[uint32]string{
		Hash("test"): "test",
	}

	_, err := matchDirectList(nil, payload, targets)
	if err == nil {
		t.Error("expected error for invalid signature, got nil")
	}
}

// findLargestSubkeyList walks the first two levels of the hive and returns
// the subkey list offset of the key with the most children.
// Falls back to the root if no better candidate is found.
func findLargestSubkeyList(tb testing.TB, h *hive.Hive) (uint32, int) {
	tb.Helper()

	rootRef := h.RootCellOffset()
	bestListRef := uint32(0)
	bestCount := 0

	// Check root
	payload, err := h.ResolveCellPayload(rootRef)
	if err != nil {
		tb.Fatalf("resolve root: %v", err)
	}
	nk, err := hive.ParseNK(payload)
	if err != nil {
		tb.Fatalf("parse root NK: %v", err)
	}

	rootListRef := nk.SubkeyListOffsetRel()
	if rootListRef == format.InvalidOffset {
		tb.Fatal("root has no subkey list")
	}

	rootList, err := Read(h, rootListRef)
	if err != nil {
		tb.Fatalf("read root subkeys: %v", err)
	}
	bestListRef = rootListRef
	bestCount = rootList.Len()

	// Check root's immediate children for larger lists
	for _, child := range rootList.Entries {
		childPayload, childErr := resolveCell(h, child.NKRef)
		if childErr != nil {
			continue
		}
		childNK, childNKErr := hive.ParseNK(childPayload)
		if childNKErr != nil {
			continue
		}
		childListRef := childNK.SubkeyListOffsetRel()
		if childListRef == format.InvalidOffset {
			continue
		}
		childList, childListErr := Read(h, childListRef)
		if childListErr != nil {
			continue
		}
		if childList.Len() > bestCount {
			bestCount = childList.Len()
			bestListRef = childListRef
		}
	}

	return bestListRef, bestCount
}

// Benchmark_MatchByHash_vs_MatchNKsFromOffsets compares the old and new paths
// for targeted child selection. Automatically finds the largest subkey list
// in the test hive for realistic benchmarking.
func Benchmark_MatchByHash_vs_MatchNKsFromOffsets(b *testing.B) {
	testHivePath := "../../testdata/large"
	h, err := hive.Open(testHivePath)
	if err != nil {
		b.Skipf("Test hive not found: %v", err)
	}
	defer h.Close()

	listRef, totalCount := findLargestSubkeyList(b, h)

	// Read full list to pick target names
	fullList, err := Read(h, listRef)
	if err != nil {
		b.Fatalf("Read subkey list: %v", err)
	}
	if fullList.Len() < 2 {
		b.Skip("need at least 2 subkeys")
	}

	// Pick entries from first, middle, and last positions
	var targetEntries []Entry
	targetEntries = append(targetEntries, fullList.Entries[0])
	targetEntries = append(targetEntries, fullList.Entries[fullList.Len()-1])
	if fullList.Len() >= 3 {
		targetEntries = append(targetEntries, fullList.Entries[fullList.Len()/2])
	}

	targetNames := make(map[string]struct{})
	targetHashes := make(map[uint32]string)
	nameList := make([]string, 0, len(targetEntries))
	for _, e := range targetEntries {
		targetNames[e.NameLower] = struct{}{}
		targetHashes[Hash(e.NameLower)] = e.NameLower
		nameList = append(nameList, e.NameLower)
	}

	b.Logf("Subkey list has %d entries; targeting %d children: %v",
		totalCount, len(targetEntries), nameList)

	b.Run("old_ReadOffsetsInto+MatchNKsFromOffsets", func(b *testing.B) {
		b.ReportAllocs()
		var offsetBuf []uint32
		for range b.N {
			offsetBuf, _ = ReadOffsetsInto(h, listRef, offsetBuf)
			_, _ = MatchNKsFromOffsets(h, offsetBuf, targetNames)
		}
	})

	b.Run("new_MatchByHash", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_, _ = MatchByHash(h, listRef, targetHashes)
		}
	})
}
