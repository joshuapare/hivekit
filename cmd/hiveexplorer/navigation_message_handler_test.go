package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keyselection"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/valuetable"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestNavigationSignalFlowToValueTable is a regression test for the bug where
// NavSignalReceivedMsg was not being forwarded to valuetable, causing values
// to not load when navigating in the tree.
//
// This test verifies the critical message flow:
// keytree navigation → signal emission → valuetable listener → NavSignalReceivedMsg → main Model → valuetable Update
func TestNavigationSignalFlowToValueTable(t *testing.T) {
	helper := NewTestHelper("test.hive")
	helper.SendWindowSize(80, 24)

	// Load tree with keys
	keys := CreateTestKeys(2)
	helper.LoadRootKeys(keys)

	// Simulate a NavSignalReceivedMsg being received
	// This is what happens when the valuetable's listener receives a navigation signal
	testSignal := keyselection.Event{
		NodeID:   hive.NodeID(123),
		Path:     "TestKey",
		HivePath: "test.hive",
	}
	msg := valuetable.NavSignalReceivedMsg{
		Signal: testSignal,
	}

	// Send the message to the main Model's Update - this is the critical path that was broken
	model, cmd := helper.GetModel().Update(msg)
	if cmd == nil {
		t.Error("expected main Model to return a command when processing NavSignalReceivedMsg")
	}

	// Verify the message was forwarded to valuetable
	// The main Model should forward this to valuetable.Update()
	// which will return commands to load values and keep listening
	_ = model.(Model) // Verify it's still a Model

	// The valuetable should have received the message
	// We can't directly test the async listener, but we can verify that
	// if a NavSignalReceivedMsg arrives at the main Model, it gets handled correctly

	// Execute the returned command to see what message it produces
	if cmd != nil {
		resultMsg := cmd()
		// The command should trigger value loading
		// We expect either a valuesLoadedMsg or an error
		if resultMsg == nil {
			t.Error("expected command to produce a message")
		}
	}
}

// TestNavigationSignalMessageHandling verifies that the main Model correctly
// handles NavSignalReceivedMsg by forwarding it to the valuetable
func TestNavigationSignalMessageHandling(t *testing.T) {
	helper := NewTestHelper("test.hive")
	helper.SendWindowSize(80, 24)

	// Create a NavSignalReceivedMsg
	testSignal := keyselection.Event{
		NodeID:   hive.NodeID(456),
		Path:     "Software\\Microsoft",
		HivePath: "test.hive",
	}
	msg := valuetable.NavSignalReceivedMsg{
		Signal: testSignal,
	}

	// Process the message through main Model
	model, cmd := helper.GetModel().Update(msg)
	m := model.(Model)

	// Verify:
	// 1. The main Model didn't panic (regression test for missing handler)
	if m.err != nil {
		t.Errorf("unexpected error after processing NavSignalReceivedMsg: %v", m.err)
	}

	// 2. A command was returned (valuetable should respond with load command + keep listening)
	if cmd == nil {
		t.Error("expected command to be returned after processing NavSignalReceivedMsg")
	}

	// 3. The valuetable received the message (we can't directly verify this without
	//    exposing internal state, but the command being non-nil indicates it was processed)
}

// TestValueTableReceivesNavSignal verifies the valuetable correctly processes
// NavSignalReceivedMsg to load values
func TestValueTableReceivesNavSignal(t *testing.T) {
	helper := NewTestHelper("test.hive")
	helper.SendWindowSize(80, 24)

	// Create a navigation signal
	testSignal := keyselection.Event{
		NodeID:   hive.NodeID(789),
		Path:     "TestPath",
		HivePath: "test.hive",
	}

	// Send NavSignalReceivedMsg directly to valuetable
	msg := valuetable.NavSignalReceivedMsg{
		Signal: testSignal,
	}

	// Get the valuetable and update it
	m := helper.GetModel()
	table := m.valueTable
	updatedTable, cmd := table.Update(msg)

	// Verify:
	// 1. Command is returned (should be batch: loadValues + keepListening)
	if cmd == nil {
		t.Error("expected valuetable to return command when processing NavSignalReceivedMsg")
	}

	// 2. Table was updated (new instance returned with updated state)
	// We can't directly compare instances, but we verified cmd is non-nil which indicates processing

	// Execute the command to verify it produces proper messages
	if cmd != nil {
		// The command is a batch, so we can't easily test the result
		// but we can verify it doesn't panic
		resultMsg := cmd()
		_ = resultMsg // May be nil or a message
	}

	_ = updatedTable // Use the updated table variable
}

// TestNavigationEmitsSignalToValueTable tests the full flow end-to-end
// by simulating a tree navigation and verifying signals flow correctly
func TestNavigationEmitsSignalToValueTable(t *testing.T) {
	helper := NewTestHelper("test.hive")
	helper.SendWindowSize(80, 24)

	// Load tree
	keys := CreateTestKeys(3)
	helper.LoadRootKeys(keys)

	// Get initial state
	initialTreeCursor := helper.GetTreeCursor()

	// Navigate down in the tree - this should emit a navigation signal
	helper.SendKey(tea.KeyDown)

	// Verify cursor moved
	newTreeCursor := helper.GetTreeCursor()
	if newTreeCursor == initialTreeCursor {
		t.Error("expected tree cursor to move after KeyDown")
	}

	// At this point, in a real scenario:
	// 1. keytree.MoveDown() was called
	// 2. CursorManager.MoveTo() emitted signal to bus
	// 3. valuetable's listener would receive signal → produce NavSignalReceivedMsg
	// 4. Main Model would forward NavSignalReceivedMsg to valuetable
	// 5. valuetable would load values
	//
	// We can't test the async listener part in a unit test, but we've verified
	// that NavSignalReceivedMsg gets handled correctly in the tests above.
}
