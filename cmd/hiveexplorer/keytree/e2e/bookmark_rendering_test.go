package e2e

import (
	"testing"

	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestE2E_Bookmarks_SingleBookmark verifies bookmarked items show star indicator
func TestE2E_Bookmarks_SingleBookmark(t *testing.T) {
	path := "SOFTWARE\\Test"
	item := newTestItem("Test", withPath(path))

	state := keytree.NewTreeState()
	state.SetAllItems([]keytree.Item{item})
	state.SetItems([]keytree.Item{item})
	state.SetBookmarks(map[string]bool{
		path: true,
	})

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	output := model.RenderItem(0, false, 80)

	// Should have bookmark indicator
	assertHasBookmark(t, output)
	assertContains(t, output, "Test")
}

// TestE2E_Bookmarks_NoBookmark verifies non-bookmarked items don't show star
func TestE2E_Bookmarks_NoBookmark(t *testing.T) {
	item := newTestItem("NotBookmarked")

	state := keytree.NewTreeState()
	state.SetAllItems([]keytree.Item{item})
	state.SetItems([]keytree.Item{item})
	state.SetBookmarks(map[string]bool{})

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	output := model.RenderItem(0, false, 80)

	// Should NOT have bookmark indicator
	assertNoBookmark(t, output)
	assertContains(t, output, "NotBookmarked")
}

// TestE2E_Bookmarks_MixedBookmarks verifies mix of bookmarked and non-bookmarked
func TestE2E_Bookmarks_MixedBookmarks(t *testing.T) {
	items := []keytree.Item{
		newTestItem("Bookmarked1", withPath("SOFTWARE\\Bookmarked1")),
		newTestItem("Normal", withPath("SOFTWARE\\Normal")),
		newTestItem("Bookmarked2", withPath("SOFTWARE\\Bookmarked2")),
		newTestItem("AnotherNormal", withPath("SOFTWARE\\AnotherNormal")),
	}

	state := keytree.NewTreeState()
	state.SetAllItems(items)
	state.SetItems(items)
	state.SetBookmarks(map[string]bool{
		"SOFTWARE\\Bookmarked1": true,
		"SOFTWARE\\Bookmarked2": true,
	})

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	// Render each
	outputs := make([]string, len(items))
	for i := range items {
		outputs[i] = model.RenderItem(i, false, 80)
	}

	// Verify bookmarked items have star
	assertHasBookmark(t, outputs[0])
	assertHasBookmark(t, outputs[2])

	// Verify non-bookmarked items don't have star
	assertNoBookmark(t, outputs[1])
	assertNoBookmark(t, outputs[3])
}

// TestE2E_Bookmarks_WithDiffStatus verifies bookmarks work with diff mode
func TestE2E_Bookmarks_WithDiffStatus(t *testing.T) {
	path := "SOFTWARE\\AddedAndBookmarked"
	item := newTestItem("AddedAndBookmarked",
		withPath(path),
		withDiffStatus(hive.DiffAdded),
	)

	state := keytree.NewTreeState()
	state.SetAllItems([]keytree.Item{item})
	state.SetItems([]keytree.Item{item})
	state.SetBookmarks(map[string]bool{
		path: true,
	})

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	output := model.RenderItem(0, false, 80)

	// Should have both: bookmark indicator AND diff prefix
	assertHasBookmark(t, output)
	assertHasPrefix(t, output, '+')
	assertContains(t, output, "AddedAndBookmarked")
}

// TestE2E_Bookmarks_ParentWithChildren verifies bookmarked parent renders correctly
func TestE2E_Bookmarks_ParentWithChildren(t *testing.T) {
	path := "SOFTWARE\\BookmarkedParent"
	item := newTestItem("BookmarkedParent",
		withPath(path),
		withChildren(5, false),
	)

	state := keytree.NewTreeState()
	state.SetAllItems([]keytree.Item{item})
	state.SetItems([]keytree.Item{item})
	state.SetBookmarks(map[string]bool{
		path: true,
	})

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	output := model.RenderItem(0, false, 80)

	// Should have bookmark, icon, and count
	assertHasBookmark(t, output)
	assertHasIcon(t, output, "▶")
	assertContains(t, output, "(5)")
	assertContains(t, output, "BookmarkedParent")
}

// TestE2E_Bookmarks_AtDepth verifies bookmarks work at various depths
func TestE2E_Bookmarks_AtDepth(t *testing.T) {
	depths := []int{0, 1, 2}

	for _, depth := range depths {
		t.Run("depth_"+string(rune('0'+depth)), func(t *testing.T) {
			path := "SOFTWARE\\BookmarkedAtDepth"
			item := newTestItem("BookmarkedAtDepth",
				withPath(path),
				withDepth(depth),
			)

			state := keytree.NewTreeState()
			state.SetAllItems([]keytree.Item{item})
			state.SetItems([]keytree.Item{item})
			state.SetBookmarks(map[string]bool{
				path: true,
			})

			model := &keytree.Model{}
			model.SetStateForTesting(state)

			output := model.RenderItem(0, false, 80)

			// Should have bookmark and correct indentation
			assertHasBookmark(t, output)
			assertIndentation(t, output, depth)
		})
	}
}

// TestE2E_Bookmarks_ComplexScenario verifies bookmark + diff + children + depth
func TestE2E_Bookmarks_ComplexScenario(t *testing.T) {
	path := "SOFTWARE\\ComplexBookmarked"
	item := newTestItem("ComplexBookmarked",
		withPath(path),
		withDiffStatus(hive.DiffModified),
		withChildren(8, true), // Expanded
		withDepth(1),
	)

	state := keytree.NewTreeState()
	state.SetAllItems([]keytree.Item{item})
	state.SetItems([]keytree.Item{item})
	state.SetBookmarks(map[string]bool{
		path: true,
	})

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	output := model.RenderItem(0, false, 80)

	// Should have ALL features:
	// - "★" bookmark indicator
	// - "~" diff prefix
	// - "▼" expanded icon
	// - "(8)" count
	// - Indentation for depth 1
	assertHasBookmark(t, output)
	assertHasPrefix(t, output, '~')
	assertHasIcon(t, output, "▼")
	assertContains(t, output, "(8)")
	assertIndentation(t, output, 1)
	assertContains(t, output, "ComplexBookmarked")
}

// TestE2E_Bookmarks_NilBookmarksMap verifies nil bookmarks map is handled
func TestE2E_Bookmarks_NilBookmarksMap(t *testing.T) {
	item := newTestItem("Key")

	state := keytree.NewTreeState()
	state.SetAllItems([]keytree.Item{item})
	state.SetItems([]keytree.Item{item})
	// Don't set bookmarks (nil)

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	// Should not panic
	output := model.RenderItem(0, false, 80)

	// Should render without bookmark indicator
	assertNoBookmark(t, output)
	assertContains(t, output, "Key")
}
