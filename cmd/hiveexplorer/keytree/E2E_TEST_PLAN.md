# E2E Test Plan for Component Architecture

## Overview

This document outlines end-to-end tests for the new component architecture (Display + Adapter + Domain). These tests verify the complete rendering pipeline from domain data to visual output.

## Current Test Coverage

### Existing E2E Tests (in `/cmd/gohivex/tui/e2e/`)
- ✅ User interactions (navigation, search)
- ✅ Cursor sync
- ✅ Scroll behavior
- ✅ Tree loading

### Missing: Component Architecture E2E Tests
- ❌ Complete rendering pipeline (Item → Adapter → Display → Output)
- ❌ Diff mode rendering verification
- ❌ Bookmark rendering verification
- ❌ Visual marker presence (+, -, ~, ★)
- ❌ Complex scenarios (deep nesting, long names, timestamps)
- ❌ Cross-cutting integration tests

---

## Proposed E2E Tests

### 1. Complete Rendering Pipeline Tests

These tests verify the full flow: Domain Item → Adapter → Display → String Output

#### Test: `TestE2E_RenderingPipeline_BasicItem`
**Purpose**: Verify basic item renders correctly end-to-end

**Test Flow**:
1. Create a simple Item with name, depth, no children
2. Call Model.RenderItem()
3. Verify output contains:
   - Item name
   - Leaf icon (•)
   - Proper indentation based on depth
   - No diff prefix (space)

**Expected Output Pattern**:
```
  • ItemName
```

#### Test: `TestE2E_RenderingPipeline_ItemWithChildren`
**Purpose**: Verify parent items render correctly

**Test Flow**:
1. Create Item with HasChildren=true, Expanded=false, SubkeyCount=5
2. Call Model.RenderItem()
3. Verify output contains:
   - Collapsed icon (▶)
   - Count text "(5)"
   - Item name

**Expected Output Pattern**:
```
  ▶ ParentName (5)
```

#### Test: `TestE2E_RenderingPipeline_ItemWithTimestamp`
**Purpose**: Verify timestamp formatting end-to-end

**Test Flow**:
1. Create Item with LastWrite timestamp
2. Call Model.RenderItem()
3. Verify output contains:
   - Item name
   - Formatted timestamp (YYYY-MM-DD HH:MM)

**Expected Output Pattern**:
```
  • ItemName                           2024-01-15 10:30
```

---

### 2. Diff Mode Rendering Tests

These tests verify diff-specific rendering with the new architecture

#### Test: `TestE2E_DiffMode_AddedItem`
**Purpose**: Verify added items render with correct styling

**Test Flow**:
1. Create Item with DiffStatus=DiffAdded
2. Call Model.RenderItem()
3. Verify output contains:
   - "+" prefix
   - Green styling (check ANSI codes)
   - Item name

**Expected Visual**:
```
+ • AddedKey
```

#### Test: `TestE2E_DiffMode_RemovedItem`
**Purpose**: Verify removed items render with correct styling

**Test Flow**:
1. Create Item with DiffStatus=DiffRemoved
2. Call Model.RenderItem()
3. Verify output contains:
   - "-" prefix
   - Red styling + strikethrough (check ANSI codes)
   - Item name

**Expected Visual**:
```
- • RemovedKey (with strikethrough)
```

#### Test: `TestE2E_DiffMode_ModifiedItem`
**Purpose**: Verify modified items render with correct styling

**Test Flow**:
1. Create Item with DiffStatus=DiffModified
2. Call Model.RenderItem()
3. Verify output contains:
   - "~" prefix
   - Orange/warning styling
   - Item name

**Expected Visual**:
```
~ • ModifiedKey
```

#### Test: `TestE2E_DiffMode_MixedTree`
**Purpose**: Verify a tree with mixed diff statuses renders correctly

**Test Flow**:
1. Create Model with multiple items of different DiffStatus
2. Render each item
3. Verify each has correct prefix and styling
4. Verify visual consistency

**Expected Tree**:
```
+ ▶ NewKey (3)
  ~ • ModifiedKey
- • RemovedKey
  • UnchangedKey
```

---

### 3. Bookmark Rendering Tests

#### Test: `TestE2E_Bookmarks_SingleBookmark`
**Purpose**: Verify bookmarked items show star indicator

**Test Flow**:
1. Create Item with path "SOFTWARE\\Test"
2. Set bookmark for that path
3. Call Model.RenderItem() with isBookmarked=true
4. Verify output contains "★" indicator

**Expected Visual**:
```
★ • BookmarkedKey
```

#### Test: `TestE2E_Bookmarks_MixedBookmarks`
**Purpose**: Verify mix of bookmarked and non-bookmarked items

**Test Flow**:
1. Create multiple items, some bookmarked
2. Render each
3. Verify bookmarked items have ★, others have space

**Expected Tree**:
```
★ • BookmarkedKey1
  • NormalKey
★ • BookmarkedKey2
  • AnotherNormalKey
```

---

### 4. Complex Scenario Tests

#### Test: `TestE2E_Complex_DeepNesting`
**Purpose**: Verify deep nesting renders correctly

**Test Flow**:
1. Create items at depths 0, 1, 2, 5, 10
2. Render each
3. Verify indentation is correct (depth * 2 spaces)

**Expected Output**:
```
• Level0
  • Level1
    • Level2
          • Level5
                    • Level10
```

#### Test: `TestE2E_Complex_LongNames`
**Purpose**: Verify long names are handled correctly

**Test Flow**:
1. Create Item with very long name (150+ chars)
2. Render with limited width
3. Verify:
   - No panic
   - Name is present
   - Padding is clamped correctly

#### Test: `TestE2E_Complex_AllFeaturesCombined`
**Purpose**: Verify all features work together

**Test Flow**:
1. Create Item with:
   - DiffStatus=DiffAdded
   - Bookmarked
   - HasChildren=true, Expanded=true
   - Timestamp
   - Depth=2
2. Render
3. Verify output contains:
   - "+" prefix
   - "★" bookmark indicator
   - "▼" expanded icon
   - Count text
   - Timestamp
   - Correct indentation

**Expected Visual**:
```
+★   ▼ AddedBookmarkedParent (5)           2024-01-15 10:30
```

---

### 5. Regression Tests

#### Test: `TestE2E_Regression_NoDiffPrefix`
**Purpose**: Verify items without diff mode don't show prefixes

**Test Flow**:
1. Create Item with DiffStatus=DiffUnchanged (or 0)
2. Render
3. Verify no visible prefix (just space)

#### Test: `TestE2E_Regression_NoBookmarkIndicator`
**Purpose**: Verify non-bookmarked items don't show star

**Test Flow**:
1. Create Item, don't bookmark it
2. Render
3. Verify no "★" in output (just space)

#### Test: `TestE2E_Regression_ConsistentWidth`
**Purpose**: Verify all items at same depth have consistent layout

**Test Flow**:
1. Create multiple items at depth 0
2. Render all with same width
3. Verify consistent spacing and alignment

---

### 6. Integration with Model Tests

#### Test: `TestE2E_Model_RenderItemIntegration`
**Purpose**: Verify Model.RenderItem() uses the new architecture

**Test Flow**:
1. Create Model with real TreeState and Items
2. Call RenderItem() for various indices
3. Verify:
   - Calls adapter.ItemToDisplayProps()
   - Calls display.RenderTreeItemDisplay()
   - Returns expected output

#### Test: `TestE2E_Model_RenderItemOutOfBounds`
**Purpose**: Verify RenderItem() handles invalid indices

**Test Flow**:
1. Create Model with 5 items
2. Call RenderItem(-1, ...)
3. Call RenderItem(10, ...)
4. Verify returns empty string, no panic

#### Test: `TestE2E_Model_RenderWithCursor`
**Purpose**: Verify cursor selection flows through pipeline

**Test Flow**:
1. Create Model with items
2. Call RenderItem(index, isCursor=true, ...)
3. Verify output has selection styling (ANSI codes for highlight)

---

## Test Organization

### File Structure

```
cmd/gohivex/tui/keytree/
├── e2e/
│   ├── README.md                        # Test documentation
│   ├── rendering_pipeline_test.go       # Tests 1: Basic rendering
│   ├── diff_mode_rendering_test.go      # Tests 2: Diff scenarios
│   ├── bookmark_rendering_test.go       # Tests 3: Bookmark scenarios
│   ├── complex_scenarios_test.go        # Tests 4: Complex cases
│   ├── regression_test.go               # Tests 5: Regression checks
│   └── model_integration_test.go        # Tests 6: Model integration
```

### Test Helpers

Create helper functions for common assertions:

```go
// assertContains verifies output contains expected substring
func assertContains(t *testing.T, output, expected string)

// assertHasIcon verifies output contains specific icon
func assertHasIcon(t *testing.T, output, icon string)

// assertHasPrefix verifies output has diff prefix
func assertHasPrefix(t *testing.T, output string, prefix rune)

// assertHasBookmark verifies output has bookmark indicator
func assertHasBookmark(t *testing.T, output string)

// assertIndentation verifies indentation level
func assertIndentation(t *testing.T, output string, depth int)

// assertHasANSI verifies ANSI styling is present
func assertHasANSI(t *testing.T, output string, colorCode string)
```

---

## Test Data Requirements

### Mock Items

Create builder functions for test items:

```go
func newTestItem(name string, opts ...ItemOption) Item
func withDepth(d int) ItemOption
func withChildren(count int, expanded bool) ItemOption
func withDiffStatus(status hivex.DiffStatus) ItemOption
func withTimestamp(t time.Time) ItemOption
```

### Mock Model

Create minimal Model setup for testing:

```go
func newTestModel(items []Item) *Model
func newTestModelWithBookmarks(items []Item, bookmarks map[string]bool) *Model
```

---

## Success Criteria

### Coverage Goals
- ✅ 100% of rendering paths tested end-to-end
- ✅ All visual markers verified (+, -, ~, ★, icons)
- ✅ All DiffStatus values tested
- ✅ Bookmark rendering verified
- ✅ Complex scenarios covered

### Quality Goals
- ✅ Tests are deterministic (no flaky tests)
- ✅ Tests run quickly (< 1s total)
- ✅ Tests are easy to understand and maintain
- ✅ Test failures clearly indicate what broke

### Integration Goals
- ✅ Tests verify the actual integration points (Model.RenderItem)
- ✅ Tests don't mock the adapter or display layers
- ✅ Tests verify the complete pipeline
- ✅ Tests catch regressions in visual output

---

## Implementation Priority

1. **High Priority** (Must Have):
   - Basic rendering pipeline tests
   - Diff mode rendering tests
   - Model integration tests

2. **Medium Priority** (Should Have):
   - Bookmark rendering tests
   - Complex scenario tests
   - Regression tests

3. **Low Priority** (Nice to Have):
   - Performance benchmarks
   - Visual snapshot testing
   - Fuzzing for edge cases

---

## Next Steps

1. ✅ Create test plan (this document)
2. ⏳ Implement high-priority tests
3. ⏳ Run tests and verify coverage
4. ⏳ Document test results
5. ⏳ Add medium-priority tests as time permits
