package integration

import (
	"errors"
	"os"
	"testing"

	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestParent tests the Parent() navigation function.
func TestParent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow integration test in short mode")
	}
	for _, tc := range suiteHives {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(tc.hivePath)
			if err != nil {
				t.Skipf("Hive not found: %s", tc.hivePath)
			}

			r, err := reader.OpenBytes(data, hive.OpenOptions{})
			if err != nil {
				t.Fatalf("Failed to open hive: %v", err)
			}
			defer r.Close()

			rootID, err := r.Root()
			if err != nil {
				t.Fatalf("Failed to get root: %v", err)
			}

			// Test 1: Root node should have no parent
			_, err = r.Parent(rootID)
			if !errors.Is(err, hive.ErrNotFound) {
				t.Errorf("Expected ErrNotFound for root parent, got: %v", err)
			}

			// Test 2: Child nodes should have parents
			children, err := r.Subkeys(rootID)
			if err != nil {
				t.Fatalf("Failed to get subkeys: %v", err)
			}

			if len(children) == 0 {
				t.Skip("No children to test")
			}

			// Test first child
			childID := children[0]
			parentID, err := r.Parent(childID)
			if err != nil {
				t.Fatalf("Failed to get parent of child: %v", err)
			}

			// Parent should be root
			if parentID != rootID {
				t.Errorf("Parent of child should be root, got NodeID %d instead of %d", parentID, rootID)
			}

			// Test 3: Grandchild should navigate back to child
			grandchildren, err := r.Subkeys(childID)
			if err != nil {
				t.Fatalf("Failed to get grandchildren: %v", err)
			}

			if len(grandchildren) > 0 {
				grandchildID := grandchildren[0]
				grandparentID, parentErr := r.Parent(grandchildID)
				if parentErr != nil {
					t.Fatalf("Failed to get parent of grandchild: %v", parentErr)
				}

				if grandparentID != childID {
					t.Errorf(
						"Parent of grandchild should be child, got NodeID %d instead of %d",
						grandparentID,
						childID,
					)
				}
			}
		})
	}
}

// TestGetChild tests the GetChild() lookup function.
func TestGetChild(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow integration test in short mode")
	}
	for _, tc := range suiteHives {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(tc.hivePath)
			if err != nil {
				t.Skipf("Hive not found: %s", tc.hivePath)
			}

			r, err := reader.OpenBytes(data, hive.OpenOptions{})
			if err != nil {
				t.Fatalf("Failed to open hive: %v", err)
			}
			defer r.Close()

			rootID, err := r.Root()
			if err != nil {
				t.Fatalf("Failed to get root: %v", err)
			}

			// Get all children
			children, err := r.Subkeys(rootID)
			if err != nil {
				t.Fatalf("Failed to get subkeys: %v", err)
			}

			if len(children) == 0 {
				t.Skip("No children to test")
			}

			// Test 1: Look up first child by name
			firstChild := children[0]
			firstChildMeta, err := r.StatKey(firstChild)
			if err != nil {
				t.Fatalf("Failed to get child metadata: %v", err)
			}

			foundChild, err := r.GetChild(rootID, firstChildMeta.Name)
			if err != nil {
				t.Fatalf("GetChild failed for existing child '%s': %v", firstChildMeta.Name, err)
			}

			if foundChild != firstChild {
				t.Errorf("GetChild returned wrong NodeID: expected %d, got %d", firstChild, foundChild)
			}

			// Test 2: Case-insensitive lookup
			upperName := toUpperCase(firstChildMeta.Name)
			foundChildUpper, err := r.GetChild(rootID, upperName)
			if err != nil {
				t.Fatalf("GetChild failed for uppercase name '%s': %v", upperName, err)
			}

			if foundChildUpper != firstChild {
				t.Errorf("Case-insensitive GetChild failed: expected %d, got %d", firstChild, foundChildUpper)
			}

			// Test 3: Non-existent child
			_, err = r.GetChild(rootID, "ThisChildDoesNotExist_123456789")
			if !errors.Is(err, hive.ErrNotFound) {
				t.Errorf("Expected ErrNotFound for non-existent child, got: %v", err)
			}
		})
	}
}

// TestGetValue tests the GetValue() lookup function.
func TestGetValue(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow integration test in short mode")
	}
	for _, tc := range suiteHives {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(tc.hivePath)
			if err != nil {
				t.Skipf("Hive not found: %s", tc.hivePath)
			}

			r, err := reader.OpenBytes(data, hive.OpenOptions{})
			if err != nil {
				t.Fatalf("Failed to open hive: %v", err)
			}
			defer r.Close()

			// Find a node with values
			rootID, err := r.Root()
			if err != nil {
				t.Fatalf("Failed to get root: %v", err)
			}

			var nodeWithValues hive.NodeID
			var testValue hive.ValueID
			var testValueName string

			// Walk tree to find a node with at least one value
			walkErr := r.Walk(rootID, func(nodeID hive.NodeID) error {
				values, _ := r.Values(nodeID)

				if len(values) > 0 {
					// Found a node with values
					nodeWithValues = nodeID
					testValue = values[0]

					// Get value name
					valueMeta, statErr := r.StatValue(testValue)
					if statErr != nil {
						return statErr
					}

					testValueName = valueMeta.Name
					return hive.ErrNotFound // Stop walking
				}

				return nil
			})

			// ErrNotFound means we found a node and stopped walking (success)
			if walkErr != nil && !errors.Is(walkErr, hive.ErrNotFound) {
				t.Fatalf("Walk failed: %v", walkErr)
			}

			if nodeWithValues == 0 {
				t.Skip("No nodes with values found in hive")
			}

			// Test 1: Look up value by name
			foundValue, err := r.GetValue(nodeWithValues, testValueName)
			if err != nil {
				t.Fatalf("GetValue failed for existing value '%s': %v", testValueName, err)
			}

			if foundValue != testValue {
				t.Errorf("GetValue returned wrong ValueID: expected %d, got %d", testValue, foundValue)
			}

			// Test 2: Case-insensitive lookup
			upperName := toUpperCase(testValueName)
			foundValueUpper, err := r.GetValue(nodeWithValues, upperName)
			if err != nil {
				t.Fatalf("GetValue failed for uppercase name '%s': %v", upperName, err)
			}

			if foundValueUpper != testValue {
				t.Errorf("Case-insensitive GetValue failed: expected %d, got %d", testValue, foundValueUpper)
			}

			// Test 3: Non-existent value
			_, err = r.GetValue(nodeWithValues, "ThisValueDoesNotExist_123456789")
			if !errors.Is(err, hive.ErrNotFound) {
				t.Errorf("Expected ErrNotFound for non-existent value, got: %v", err)
			}
		})
	}
}

// TestNavigationRoundTrip tests that Parent and GetChild work together.
func TestNavigationRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow integration test in short mode")
	}
	for _, tc := range suiteHives {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(tc.hivePath)
			if err != nil {
				t.Skipf("Hive not found: %s", tc.hivePath)
			}

			r, err := reader.OpenBytes(data, hive.OpenOptions{})
			if err != nil {
				t.Fatalf("Failed to open hive: %v", err)
			}
			defer r.Close()

			rootID, err := r.Root()
			if err != nil {
				t.Fatalf("Failed to get root: %v", err)
			}

			// Get a child
			children, err := r.Subkeys(rootID)
			if err != nil {
				t.Fatalf("Failed to get subkeys: %v", err)
			}

			if len(children) == 0 {
				t.Skip("No children to test")
			}

			childID := children[0]
			childMeta, err := r.StatKey(childID)
			if err != nil {
				t.Fatalf("Failed to get child metadata: %v", err)
			}

			// Navigate child -> parent -> child
			parentID, err := r.Parent(childID)
			if err != nil {
				t.Fatalf("Failed to get parent: %v", err)
			}

			foundChildID, err := r.GetChild(parentID, childMeta.Name)
			if err != nil {
				t.Fatalf("Failed to get child by name: %v", err)
			}

			if foundChildID != childID {
				t.Errorf("Round-trip navigation failed: started with %d, ended with %d", childID, foundChildID)
			}
		})
	}
}

// Helper function to convert string to uppercase.
func toUpperCase(s string) string {
	result := make([]byte, len(s))
	for i := range len(s) {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c = c - 'a' + 'A'
		}
		result[i] = c
	}
	return string(result)
}
