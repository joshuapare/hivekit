package keyselection

import (
	"context"
	"testing"
	"time"

	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestNewBus verifies that NewBus creates a properly initialized bus
func TestNewBus(t *testing.T) {
	bus := NewBus()

	if bus == nil {
		t.Fatal("NewBus() returned nil")
	}

	if bus.subscribers == nil {
		t.Error("subscribers slice should be initialized")
	}

	if len(bus.subscribers) != 0 {
		t.Errorf("expected 0 subscribers, got %d", len(bus.subscribers))
	}

	if bus.cancel != nil {
		t.Error("cancel should be nil for new bus")
	}
}

// TestSubscribe tests that Subscribe() returns a channel and adds to subscribers
func TestSubscribe(t *testing.T) {
	bus := NewBus()

	// Subscribe first channel
	ch1 := bus.Subscribe()
	if ch1 == nil {
		t.Fatal("Subscribe() returned nil channel")
	}

	if len(bus.subscribers) != 1 {
		t.Errorf("expected 1 subscriber, got %d", len(bus.subscribers))
	}

	// Subscribe second channel
	ch2 := bus.Subscribe()
	if ch2 == nil {
		t.Fatal("Subscribe() returned nil channel for second subscriber")
	}

	if len(bus.subscribers) != 2 {
		t.Errorf("expected 2 subscribers, got %d", len(bus.subscribers))
	}

	// Verify channels are different
	if &ch1 == &ch2 {
		t.Error("Subscribe() should return different channels for different subscribers")
	}
}

// TestNotify tests that Notify() sends events to all subscribers
func TestNotify(t *testing.T) {
	bus := NewBus()

	// Subscribe two channels
	ch1 := bus.Subscribe()
	ch2 := bus.Subscribe()

	// Notify with test data
	testNodeID := hive.NodeID(12345)
	testPath := "SOFTWARE\\Microsoft"
	testHivePath := "/path/to/hive"

	bus.Notify(testNodeID, testPath, testHivePath, false, 0, 0, 0, nil, nil)

	// Verify both subscribers receive the event
	select {
	case event := <-ch1:
		if event.NodeID != testNodeID {
			t.Errorf("ch1: expected NodeID %d, got %d", testNodeID, event.NodeID)
		}
		if event.Path != testPath {
			t.Errorf("ch1: expected Path %q, got %q", testPath, event.Path)
		}
		if event.HivePath != testHivePath {
			t.Errorf("ch1: expected HivePath %q, got %q", testHivePath, event.HivePath)
		}
		if event.Ctx == nil {
			t.Error("ch1: expected non-nil context")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("ch1: timeout waiting for event")
	}

	select {
	case event := <-ch2:
		if event.NodeID != testNodeID {
			t.Errorf("ch2: expected NodeID %d, got %d", testNodeID, event.NodeID)
		}
		if event.Path != testPath {
			t.Errorf("ch2: expected Path %q, got %q", testPath, event.Path)
		}
		if event.HivePath != testHivePath {
			t.Errorf("ch2: expected HivePath %q, got %q", testHivePath, event.HivePath)
		}
		if event.Ctx == nil {
			t.Error("ch2: expected non-nil context")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("ch2: timeout waiting for event")
	}
}

// TestNotifyCancellation tests that previous context is cancelled when Notify() is called again
func TestNotifyCancellation(t *testing.T) {
	bus := NewBus()
	ch := bus.Subscribe()

	// First notification
	bus.Notify(hive.NodeID(1), "path1", "hive1", false, 0, 0, 0, nil, nil)

	var firstCtx context.Context
	select {
	case event := <-ch:
		firstCtx = event.Ctx
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for first event")
	}

	// Second notification should cancel the first context
	bus.Notify(hive.NodeID(2), "path2", "hive2", false, 0, 0, 0, nil, nil)

	// Verify first context is cancelled
	select {
	case <-firstCtx.Done():
		// Context was cancelled as expected
	case <-time.After(100 * time.Millisecond):
		t.Error("first context should have been cancelled")
	}

	// Verify we receive the second event
	select {
	case event := <-ch:
		if event.NodeID != hive.NodeID(2) {
			t.Errorf("expected NodeID 2, got %d", event.NodeID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for second event")
	}
}

// TestNotifyNonBlockingOnFullChannel tests that slow subscribers don't block (channel full case)
func TestNotifyNonBlockingOnFullChannel(t *testing.T) {
	bus := NewBus()
	ch := bus.Subscribe()

	// Fill the channel (buffer size is 1)
	bus.Notify(hive.NodeID(1), "path1", "hive1", false, 0, 0, 0, nil, nil)

	// Receive first event to verify channel works
	select {
	case <-ch:
		// Successfully received
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for first event")
	}

	// Fill the channel again
	bus.Notify(hive.NodeID(2), "path2", "hive2", false, 0, 0, 0, nil, nil)

	// Try to send another event without draining the channel
	// This should not block because of the select/default in Notify()
	done := make(chan bool)
	go func() {
		bus.Notify(hive.NodeID(3), "path3", "hive3", false, 0, 0, 0, nil, nil)
		done <- true
	}()

	select {
	case <-done:
		// Notify returned without blocking
	case <-time.After(100 * time.Millisecond):
		t.Error("Notify() blocked on full channel")
	}
}

// TestClose tests that Close() closes all subscriber channels and cancels context
func TestClose(t *testing.T) {
	bus := NewBus()

	ch1 := bus.Subscribe()
	ch2 := bus.Subscribe()

	// Send an event to create a context
	bus.Notify(hive.NodeID(1), "path1", "hive1", false, 0, 0, 0, nil, nil)

	var ctx context.Context
	select {
	case event := <-ch1:
		ctx = event.Ctx
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event")
	}

	// Drain ch2
	select {
	case <-ch2:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for ch2 event")
	}

	// Close the bus
	bus.Close()

	// Verify context is cancelled
	select {
	case <-ctx.Done():
		// Context was cancelled as expected
	case <-time.After(100 * time.Millisecond):
		t.Error("context should have been cancelled on Close()")
	}

	// Verify channels are closed
	select {
	case _, ok := <-ch1:
		if ok {
			t.Error("ch1 should be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("ch1 should be closed immediately")
	}

	select {
	case _, ok := <-ch2:
		if ok {
			t.Error("ch2 should be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("ch2 should be closed immediately")
	}
}

// TestMultipleSubscribers tests multiple subscribers receive the same event
func TestMultipleSubscribers(t *testing.T) {
	bus := NewBus()

	// Subscribe multiple channels
	channels := make([]<-chan Event, 5)
	for i := 0; i < 5; i++ {
		channels[i] = bus.Subscribe()
	}

	// Notify with test data
	testNodeID := hive.NodeID(99999)
	testPath := "SOFTWARE\\Test\\Path"
	testHivePath := "/test/hive/path"

	bus.Notify(testNodeID, testPath, testHivePath, false, 0, 0, 0, nil, nil)

	// Verify all subscribers receive the same event
	for i, ch := range channels {
		select {
		case event := <-ch:
			if event.NodeID != testNodeID {
				t.Errorf("subscriber %d: expected NodeID %d, got %d", i, testNodeID, event.NodeID)
			}
			if event.Path != testPath {
				t.Errorf("subscriber %d: expected Path %q, got %q", i, testPath, event.Path)
			}
			if event.HivePath != testHivePath {
				t.Errorf("subscriber %d: expected HivePath %q, got %q", i, testHivePath, event.HivePath)
			}
			if event.Ctx == nil {
				t.Errorf("subscriber %d: expected non-nil context", i)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("subscriber %d: timeout waiting for event", i)
		}
	}
}

// TestEventContent tests that Event contains correct NodeID, Path, HivePath
func TestEventContent(t *testing.T) {
	tests := []struct {
		name     string
		nodeID   hive.NodeID
		path     string
		hivePath string
	}{
		{
			name:     "basic path",
			nodeID:   hive.NodeID(100),
			path:     "SOFTWARE",
			hivePath: "/path/to/SOFTWARE.hive",
		},
		{
			name:     "nested path",
			nodeID:   hive.NodeID(200),
			path:     "SOFTWARE\\Microsoft\\Windows\\CurrentVersion",
			hivePath: "/path/to/SYSTEM.hive",
		},
		{
			name:     "root key",
			nodeID:   hive.NodeID(1),
			path:     "",
			hivePath: "/hive",
		},
		{
			name:     "unicode path",
			nodeID:   hive.NodeID(300),
			path:     "SOFTWARE\\Test\\中文",
			hivePath: "/path/中文.hive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := NewBus()
			ch := bus.Subscribe()

			bus.Notify(tt.nodeID, tt.path, tt.hivePath, false, 0, 0, 0, nil, nil)

			select {
			case event := <-ch:
				if event.NodeID != tt.nodeID {
					t.Errorf("expected NodeID %d, got %d", tt.nodeID, event.NodeID)
				}
				if event.Path != tt.path {
					t.Errorf("expected Path %q, got %q", tt.path, event.Path)
				}
				if event.HivePath != tt.hivePath {
					t.Errorf("expected HivePath %q, got %q", tt.hivePath, event.HivePath)
				}
				if event.Ctx == nil {
					t.Error("expected non-nil context")
				}
				if err := event.Ctx.Err(); err != nil {
					t.Errorf("expected active context, got error: %v", err)
				}
			case <-time.After(100 * time.Millisecond):
				t.Error("timeout waiting for event")
			}
		})
	}
}
