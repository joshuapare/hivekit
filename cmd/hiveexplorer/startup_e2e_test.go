package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestStartupWithTestdataHives ensures the TUI can start with real hive files
// from the testdata directory without panicking. This is a regression test
// for the virtual scrolling and lazy initialization changes.
func TestStartupWithTestdataHives(t *testing.T) {
	// Find all hive files in testdata/suite (files without extensions)
	testdataDir := filepath.Join("..", "..", "..", "testdata", "suite")

	entries, err := os.ReadDir(testdataDir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("testdata/suite directory not found")
		}
		t.Fatalf("failed to read testdata directory: %v", err)
	}

	hiveFiles := []string{}
	for _, entry := range entries {
		// Skip directories, .xz files, .reg files, and other non-hive files
		if entry.IsDir() || filepath.Ext(entry.Name()) != "" {
			continue
		}
		hiveFiles = append(hiveFiles, filepath.Join(testdataDir, entry.Name()))
	}

	if len(hiveFiles) == 0 {
		t.Skip("no hive files found in testdata/suite")
	}

	for _, hivePath := range hiveFiles {
		t.Run(filepath.Base(hivePath), func(t *testing.T) {
			// Verify file exists
			if _, err := os.Stat(hivePath); err != nil {
				t.Skipf("hive file not found: %s", hivePath)
			}

			// Create model - this should not panic
			m := NewModel(hivePath)
			if m.hivePath != hivePath {
				t.Errorf("expected hivePath %q, got %q", hivePath, m.hivePath)
			}

			// Send window size to initialize renderers - this should not panic
			model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			m = model.(Model)

			// Call View() - this should not panic and should return non-empty string
			view := m.View()
			if view == "" {
				t.Error("View() returned empty string")
			}

			// Verify view contains expected elements
			if len(view) < 100 {
				t.Errorf("View() returned suspiciously short output: %d characters", len(view))
			}
		})
	}
}

// TestValueTableRendering specifically tests that the value table
// doesn't get stuck showing "Loading..." when values are present
func TestValueTableRendering(t *testing.T) {
	helper := NewTestHelper("test.hive")
	helper.SendWindowSize(120, 40)

	// Load some test values
	values := []ValueInfo{
		{Name: "TestValue1", Type: "REG_SZ", StringVal: "test", Size: 4},
		{Name: "TestValue2", Type: "REG_DWORD", DWordVal: 42, Size: 4},
		{Name: "TestValue3", Type: "REG_QWORD", QWordVal: 123456, Size: 8},
	}
	helper.LoadValues("TestKey", values)

	model := helper.GetModel()

	// Verify items are loaded
	items := model.valueTable.GetItems()
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	// Get the view
	view := model.valueTable.View()

	// Should not show "Loading..."
	if view == "Loading..." {
		t.Error("value table showing 'Loading...' despite having items")
	}

	// Should not show "No values"
	if view == "No values" {
		t.Error("value table showing 'No values' despite having 3 items")
	}

	// View should have reasonable length (header + separator + items)
	if len(view) < 50 {
		t.Errorf("value table view is suspiciously short: %d characters, view: %q", len(view), view)
	}
}

// TestKeyTreeRendering tests that the key tree renders items correctly
func TestKeyTreeRendering(t *testing.T) {
	helper := NewTestHelper("test.hive")
	helper.SendWindowSize(120, 40)

	// Load some test keys
	keys := CreateTestKeys(5)
	helper.LoadRootKeys(keys)

	model := helper.GetModel()

	// Verify items are loaded
	if helper.GetTreeItemCount() != 5 {
		t.Fatalf("expected 5 items, got %d", helper.GetTreeItemCount())
	}

	// Get the key tree view
	view := model.keyTree.View()

	// Should not show "Loading..."
	if view == "Loading..." {
		t.Error("key tree showing 'Loading...' despite having items")
	}

	// View should have reasonable length
	if len(view) < 20 {
		t.Errorf("key tree view is suspiciously short: %d characters, view: %q", len(view), view)
	}
}
