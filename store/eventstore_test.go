package store_test

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"financial-ledger/events"
	"financial-ledger/store"
)

// Helper to create decimals
func dec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

// Simple mock event for testing
type TestEvent struct {
	events.BaseEvent
	Data string
}

func newTestEvent(aggID string, version int, data string) events.Event {
	return TestEvent{
		BaseEvent: events.NewBaseEvent(aggID, version, "TestEvent"),
		Data:      data,
	}
}

func TestInMemoryEventStore_SaveEvents(t *testing.T) {
	es := store.NewInMemoryEventStore()
	aggID := "agg-save-1"

	t.Run("SaveFirstEvent", func(t *testing.T) {
		event1 := newTestEvent(aggID, 1, "one")
		err := es.SaveEvents(aggID, 0, []events.Event{event1})
		if err != nil {
			t.Fatalf("SaveEvents failed for first event: %v", err)
		}
		stream, _ := es.GetEvents(aggID)
		if len(stream) != 1 {
			t.Fatalf("Expected 1 event, got %d", len(stream))
		}
		if stream[0].GetBase().Version != 1 {
			t.Errorf("Expected event version 1, got %d", stream[0].GetBase().Version)
		}
	})

	t.Run("SaveSubsequentEvent", func(t *testing.T) {
		event2 := newTestEvent(aggID, 2, "two")
		err := es.SaveEvents(aggID, 1, []events.Event{event2}) // Expect version 1
		if err != nil {
			t.Fatalf("SaveEvents failed for second event: %v", err)
		}
		stream, _ := es.GetEvents(aggID)
		if len(stream) != 2 {
			t.Fatalf("Expected 2 events, got %d", len(stream))
		}
		if stream[1].GetBase().Version != 2 {
			t.Errorf("Expected event version 2, got %d", stream[1].GetBase().Version)
		}
	})

	t.Run("SaveMultipleEvents", func(t *testing.T) {
		aggID2 := "agg-multi-1"
		event1 := newTestEvent(aggID2, 1, "m-one")
		event2 := newTestEvent(aggID2, 2, "m-two")
		err := es.SaveEvents(aggID2, 0, []events.Event{event1, event2})
		if err != nil {
			t.Fatalf("SaveEvents failed for multiple events: %v", err)
		}
		stream, _ := es.GetEvents(aggID2)
		if len(stream) != 2 {
			t.Fatalf("Expected 2 events, got %d", len(stream))
		}
		if stream[0].GetBase().Version != 1 || stream[1].GetBase().Version != 2 {
			t.Errorf("Expected versions 1 and 2, got %d and %d", stream[0].GetBase().Version, stream[1].GetBase().Version)
		}
	})

	t.Run("FailOnOptimisticLock", func(t *testing.T) {
		event3 := newTestEvent(aggID, 3, "three")
		err := es.SaveEvents(aggID, 1, []events.Event{event3}) // Wrong expected version (current is 2)
		if !errors.Is(err, store.ErrOptimisticLock) {
			t.Errorf("Expected ErrOptimisticLock, got %v", err)
		}
		stream, _ := es.GetEvents(aggID)
		if len(stream) != 2 { // Should not have saved event3
			t.Fatalf("Expected 2 events after optimistic lock failure, got %d", len(stream))
		}
	})

	t.Run("FailOnSequenceError", func(t *testing.T) {
		aggID3 := "agg-seq-err"
		event1 := newTestEvent(aggID3, 1, "s-one")
		event3 := newTestEvent(aggID3, 3, "s-three") // Version gap
		err := es.SaveEvents(aggID3, 0, []events.Event{event1, event3})
		if err == nil {
			t.Fatalf("Expected sequence error, got nil")
		}
		if errors.Is(err, store.ErrOptimisticLock) {
			t.Errorf("Expected sequence error, not optimistic lock error")
		}
		stream, _ := es.GetEvents(aggID3)
		if len(stream) != 0 {
			t.Fatalf("Expected 0 events after sequence error, got %d", len(stream))
		}
	})

	t.Run("FailOnAggregateIDMismatch", func(t *testing.T) {
		aggID4 := "agg-id-match"
		eventOK := newTestEvent(aggID4, 1, "id-ok")
		eventBad := newTestEvent("different-agg-id", 2, "id-bad")
		err := es.SaveEvents(aggID4, 0, []events.Event{eventOK, eventBad})
		if err == nil {
			t.Fatalf("Expected aggregate ID mismatch error, got nil")
		}
		stream, _ := es.GetEvents(aggID4)
		if len(stream) != 0 {
			t.Fatalf("Expected 0 events after ID mismatch error, got %d", len(stream))
		}
	})
}

func TestInMemoryEventStore_GetEvents(t *testing.T) {
	es := store.NewInMemoryEventStore()
	aggID := "agg-get-1"
	event1 := newTestEvent(aggID, 1, "g-one")
	event2 := newTestEvent(aggID, 2, "g-two")
	_ = es.SaveEvents(aggID, 0, []events.Event{event1, event2})

	t.Run("GetExistingStream", func(t *testing.T) {
		stream, err := es.GetEvents(aggID)
		if err != nil {
			t.Fatalf("GetEvents failed: %v", err)
		}
		if len(stream) != 2 {
			t.Fatalf("Expected 2 events, got %d", len(stream))
		}
		// Modify returned slice and check if original is affected (it shouldn't be)
		if len(stream) > 0 {
			stream[0] = newTestEvent("modified", 99, "modified")
		}
		originalStream, _ := es.GetEvents(aggID)
		if originalStream[0].GetBase().Version == 99 {
			t.Errorf("GetEvents did not return a copy, original stream modified")
		}
	})

	t.Run("GetNonExistentStream", func(t *testing.T) {
		stream, err := es.GetEvents("agg-nonexistent")
		if err != nil {
			t.Fatalf("GetEvents failed for non-existent stream: %v", err)
		}
		if len(stream) != 0 {
			t.Fatalf("Expected 0 events for non-existent stream, got %d", len(stream))
		}
	})
}

func TestInMemoryEventStore_GetEventsAfterVersion(t *testing.T) {
	es := store.NewInMemoryEventStore()
	aggID := "agg-after-1"
	event1 := newTestEvent(aggID, 1, "a-one")
	event2 := newTestEvent(aggID, 2, "a-two")
	event3 := newTestEvent(aggID, 3, "a-three")
	_ = es.SaveEvents(aggID, 0, []events.Event{event1, event2, event3})

	tests := []struct {
		name          string
		version       int
		expectedCount int
		expectedFirst int // Expected version of the first event in result, 0 if empty
	}{
		{"After 0", 0, 3, 1},
		{"After 1", 1, 2, 2},
		{"After 2", 2, 1, 3},
		{"After 3", 3, 0, 0},
		{"After 4", 4, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream, err := es.GetEventsAfterVersion(aggID, tt.version)
			if err != nil {
				t.Fatalf("GetEventsAfterVersion failed: %v", err)
			}
			if len(stream) != tt.expectedCount {
				t.Fatalf("Expected %d events, got %d", tt.expectedCount, len(stream))
			}
			if tt.expectedFirst > 0 {
				if stream[0].GetBase().Version != tt.expectedFirst {
					t.Errorf("Expected first event version %d, got %d", tt.expectedFirst, stream[0].GetBase().Version)
				}
				// Check copy
				stream[0] = newTestEvent("modified", 99, "modified")
				originalStream, _ := es.GetEvents(aggID)
				found := false
				for _, ev := range originalStream {
					if ev.GetBase().Version == 99 {
						found = true
						break
					}
				}
				if found {
					t.Errorf("GetEventsAfterVersion did not return a copy")
				}
			}
		})
	}

	t.Run("NonExistentStream", func(t *testing.T) {
		stream, err := es.GetEventsAfterVersion("agg-nonexistent", 0)
		if err != nil {
			t.Fatalf("GetEventsAfterVersion failed for non-existent stream: %v", err)
		}
		if len(stream) != 0 {
			t.Fatalf("Expected 0 events, got %d", len(stream))
		}
	})
}
