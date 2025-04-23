package store

import (
	"fmt"
	"sync"
	"time"

	"financial-ledger/domain"
)

type SnapshotStore interface {
	SaveSnapshot(snapshot *domain.Snapshot) error

	GetLatestSnapshot(aggregateID string) (snapshot *domain.Snapshot, found bool, err error)
}

type InMemorySnapshotStore struct {
	sync.RWMutex
	snapshots map[string]*domain.Snapshot
}

func NewInMemorySnapshotStore() *InMemorySnapshotStore {
	return &InMemorySnapshotStore{
		snapshots: make(map[string]*domain.Snapshot),
	}
}

func (s *InMemorySnapshotStore) SaveSnapshot(snapshot *domain.Snapshot) error {
	if snapshot == nil {
		return fmt.Errorf("cannot save nil snapshot")
	}
	s.Lock()
	defer s.Unlock()

	s.snapshots[snapshot.AggregateID] = snapshot
	snapshot.Timestamp = time.Now().UTC()
	return nil
}

func (s *InMemorySnapshotStore) GetLatestSnapshot(aggregateID string) (*domain.Snapshot, bool, error) {
	s.RLock()
	defer s.RUnlock()

	snapshot, found := s.snapshots[aggregateID]
	if !found {
		return nil, false, nil
	}

	stateCopy := make([]byte, len(snapshot.State))
	copy(stateCopy, snapshot.State)

	snapCopy := &domain.Snapshot{
		AggregateID: snapshot.AggregateID,
		Version:     snapshot.Version,
		State:       stateCopy,
		Timestamp:   snapshot.Timestamp,
	}

	return snapCopy, true, nil
}
