package walker

import (
	"errors"

	"github.com/joshuapare/hivekit/hive"
)

const (
	// riSublistEstimateSize is an estimate of the average number of entries per sublist in an RI.
	// Used for pre-allocating capacity when processing RI (indirect) lists.
	riSublistEstimateSize = 8
)

// ErrStopWalk is a sentinel error that can be returned from walk callbacks
// to stop the walk early without triggering an error condition.
var ErrStopWalk = errors.New("stop walk")

// extractSublistRefs extracts NK cell references from a sublist (LF, LH, or LI).
// Returns the slice of cell references.
func extractSublistRefs(payload []byte) []uint32 {
	kind := hive.DetectListKind(payload)
	switch kind {
	case hive.ListLF:
		lf, _ := hive.ParseLF(payload)
		count := lf.Count()
		refs := make([]uint32, count)
		for i := range count {
			refs[i] = lf.Entry(i).Cell()
		}
		return refs
	case hive.ListLH:
		lh, _ := hive.ParseLH(payload)
		count := lh.Count()
		refs := make([]uint32, count)
		for i := range count {
			refs[i] = lh.Entry(i).Cell()
		}
		return refs
	case hive.ListLI:
		li, _ := hive.ParseLI(payload)
		count := li.Count()
		refs := make([]uint32, count)
		for i := range count {
			refs[i] = li.CellIndexAt(i)
		}
		return refs
	case hive.ListRI:
		// RI within sublist is unusual - skip to avoid nested complexity
		return nil
	case hive.ListUnknown:
		// Unknown sublist type - skip
		return nil
	}
	return nil
}

// extractRIRefs extracts NK cell references from an RI (indirect list).
// Resolves all sublists and collects their references.
func extractRIRefs(h *hive.Hive, ri hive.RI) ([]uint32, error) {
	count := ri.Count()
	estimatedCap := count * riSublistEstimateSize
	refs := make([]uint32, 0, estimatedCap)

	for i := range count {
		sublistOffset := ri.LeafCellAt(i)
		sublistPayload, err := h.ResolveCellPayload(sublistOffset)
		if err != nil {
			return nil, err
		}
		sublistRefs := extractSublistRefs(sublistPayload)
		refs = append(refs, sublistRefs...)
	}

	return refs, nil
}

// WalkSubkeys walks all subkeys of the NK at the given offset and calls fn for each.
// If fn returns ErrStopWalk, the walk stops early and nil is returned.
// Any other error from fn is returned to the caller.
func WalkSubkeys(h *hive.Hive, nkOffset uint32, fn func(nk hive.NK, ref uint32) error) error {
	// Resolve the NK cell
	payload, err := h.ResolveCellPayload(nkOffset)
	if err != nil {
		return err
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return err
	}

	// Get subkey list
	subkeys, err := nk.ResolveSubkeyList(h)
	if err != nil {
		return err
	}

	// Get refs based on list kind
	var refs []uint32
	switch subkeys.Kind {
	case hive.ListLF:
		count := subkeys.LF.Count()
		refs = make([]uint32, count)
		for i := range count {
			refs[i] = subkeys.LF.Entry(i).Cell()
		}
	case hive.ListLH:
		count := subkeys.LH.Count()
		refs = make([]uint32, count)
		for i := range count {
			refs[i] = subkeys.LH.Entry(i).Cell()
		}
	case hive.ListLI:
		count := subkeys.LI.Count()
		refs = make([]uint32, count)
		for i := range count {
			refs[i] = subkeys.LI.CellIndexAt(i)
		}
	case hive.ListRI:
		var riErr error
		refs, riErr = extractRIRefs(h, subkeys.RI)
		if riErr != nil {
			return riErr
		}
	case hive.ListUnknown:
		// Unknown list type - skip
		return nil
	}

	// Walk each subkey
	for _, ref := range refs {
		// Resolve the subkey NK
		subkeyPayload, subkeyErr := h.ResolveCellPayload(ref)
		if subkeyErr != nil {
			return subkeyErr
		}

		subkeyNK, parseErr := hive.ParseNK(subkeyPayload)
		if parseErr != nil {
			return parseErr
		}

		// Call the callback
		if callbackErr := fn(subkeyNK, ref); callbackErr != nil {
			if errors.Is(callbackErr, ErrStopWalk) {
				return nil // Normal early termination
			}
			return callbackErr
		}
	}

	return nil
}

// WalkValues walks all values of the NK at the given offset and calls fn for each.
// If fn returns ErrStopWalk, the walk stops early and nil is returned.
// Any other error from fn is returned to the caller.
func WalkValues(h *hive.Hive, nkOffset uint32, fn func(vk hive.VK, ref uint32) error) error {
	// Resolve the NK cell
	payload, err := h.ResolveCellPayload(nkOffset)
	if err != nil {
		return err
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return err
	}

	// Get value list
	values, err := nk.ResolveValueList(h)
	if err != nil {
		return err
	}

	// Walk each value
	count := values.Count()
	for i := range count {
		ref, vkOffsetErr := values.VKOffsetAt(i)
		if vkOffsetErr != nil {
			return vkOffsetErr
		}

		// Resolve the value VK
		vkPayload, resolveErr := h.ResolveCellPayload(ref)
		if resolveErr != nil {
			return resolveErr
		}

		vk, parseErr := hive.ParseVK(vkPayload)
		if parseErr != nil {
			return parseErr
		}

		// Call the callback
		if callbackErr := fn(vk, ref); callbackErr != nil {
			if errors.Is(callbackErr, ErrStopWalk) {
				return nil // Normal early termination
			}
			return callbackErr
		}
	}

	return nil
}
