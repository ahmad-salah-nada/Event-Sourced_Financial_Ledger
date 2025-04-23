package store

import (
	"errors"
	"fmt"
	"log"
	"sync"

	"financial-ledger/events"
)

var (
	ErrOptimisticLock = errors.New("optimistic lock error: version conflict")
	ErrNotFound       = errors.New("aggregate not found")
)

type EventStore interface {
	SaveEvents(aggregateID string, expectedVersion int, eventsToSave []events.Event) error

	GetEvents(aggregateID string) ([]events.Event, error)

	GetEventsAfterVersion(aggregateID string, version int) ([]events.Event, error)
}

type InMemoryEventStore struct {
	sync.RWMutex
	streams map[string][]events.Event
}

func NewInMemoryEventStore() *InMemoryEventStore {
	return &InMemoryEventStore{
		streams: make(map[string][]events.Event),
	}
}

func (s *InMemoryEventStore) SaveEvents(aggregateID string, expectedVersion int, newEvents []events.Event) error {
	s.Lock()
	defer s.Unlock()

	if len(newEvents) == 0 {
		log.Printf("Warning: SaveEvents called with zero events for aggregate %s", aggregateID)
		return nil
	}

	stream, streamExists := s.streams[aggregateID]
	currentVersion := 0
	if streamExists && len(stream) > 0 {
		currentVersion = stream[len(stream)-1].GetBase().Version
	}

	if currentVersion != expectedVersion {
		return fmt.Errorf("%w: expected version %d, but current version is %d for aggregate %s",
			ErrOptimisticLock, expectedVersion, currentVersion, aggregateID)
	}

	nextVersion := expectedVersion
	for _, event := range newEvents {
		base := event.GetBase()
		nextVersion++
		if base.Version != nextVersion {
			return fmt.Errorf("event sequence error for aggregate %s: expected version %d for event %T (%s), but got %d",
				aggregateID, nextVersion, event, base.EventID, base.Version)
		}
		if base.AggregateID != aggregateID {
			return fmt.Errorf("event aggregate ID mismatch: stream is for %s, but event %T (%s) has ID %s",
				aggregateID, event, base.EventID, base.AggregateID)
		}
	}

	if !streamExists {
		s.streams[aggregateID] = make([]events.Event, 0, len(newEvents))
	}
	s.streams[aggregateID] = append(s.streams[aggregateID], newEvents...)

	return nil
}

func (s *InMemoryEventStore) GetEvents(aggregateID string) ([]events.Event, error) {
	s.RLock()
	defer s.RUnlock()

	streamData, ok := s.streams[aggregateID]
	if !ok {
		return []events.Event{}, nil
	}

	copiedStream := make([]events.Event, len(streamData))
	copy(copiedStream, streamData)
	return copiedStream, nil
}

func (s *InMemoryEventStore) GetEventsAfterVersion(aggregateID string, version int) ([]events.Event, error) {
	s.RLock()
	defer s.RUnlock()

	streamData, ok := s.streams[aggregateID]
	if !ok {
		return []events.Event{}, nil
	}

	startIndex := -1
	for i, event := range streamData {
		if event.GetBase().Version > version {
			startIndex = i
			break
		}
	}

	if startIndex == -1 {
		return []events.Event{}, nil
	}

	result := make([]events.Event, len(streamData)-startIndex)
	copy(result, streamData[startIndex:])
	return result, nil
}

// --- Test Helpers ---
// These methods are primarily for testing purposes, allowing manipulation of the store's state.

// GetStreamCopy returns a copy of the raw event stream for a given aggregate ID.
// Useful for inspecting the store's state in tests.
func (s *InMemoryEventStore) GetStreamCopy(aggregateID string) []events.Event {
	s.RLock()
	defer s.RUnlock()
	streamData, ok := s.streams[aggregateID]
	if !ok {
		return nil
	}
	copiedStream := make([]events.Event, len(streamData))
	copy(copiedStream, streamData)
	return copiedStream
}

// SetStream forcefully replaces the event stream for a given aggregate ID.
// WARNING: Use ONLY in tests to simulate specific scenarios (like event pruning for snapshot testing).
func (s *InMemoryEventStore) SetStream(aggregateID string, stream []events.Event) {
	s.Lock()
	defer s.Unlock()
	if stream == nil {
		delete(s.streams, aggregateID) // Allow setting to nil to clear stream
	} else {
		// Store a copy to avoid external modification issues if the test reuses the slice
		streamCopy := make([]events.Event, len(stream))
		copy(streamCopy, stream)
		s.streams[aggregateID] = streamCopy
	}
}
