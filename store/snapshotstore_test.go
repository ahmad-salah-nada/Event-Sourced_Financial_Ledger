package store_test

import (
	"encoding/json"
	"testing"
	"time"

	"financial-ledger/domain"
	"financial-ledger/store"
)

func newSnapshot(aggID string, version int, stateData map[string]interface{}) *domain.Snapshot {
	stateBytes, _ := json.Marshal(stateData)
	return &domain.Snapshot{
		AggregateID: aggID,
		Version:     version,
		State:       stateBytes,
		Timestamp:   time.Now().UTC(),
	}
}

func TestInMemorySnapshotStore_SaveAndGetSnapshot(t *testing.T) {
	ss := store.NewInMemorySnapshotStore()
	aggID := "snap-agg-1"

	t.Run("GetNotFound", func(t *testing.T) {
		snap, found, err := ss.GetLatestSnapshot(aggID)
		if err != nil {
			t.Fatalf("GetLatestSnapshot failed: %v", err)
		}
		if found {
			t.Errorf("Expected snapshot not found, but found one: %v", snap)
		}
		if snap != nil {
			t.Errorf("Expected nil snapshot, got %v", snap)
		}
	})

	t.Run("SaveAndGet", func(t *testing.T) {
		snap1 := newSnapshot(aggID, 5, map[string]interface{}{"balance": 100})
		err := ss.SaveSnapshot(snap1)
		if err != nil {
			t.Fatalf("SaveSnapshot failed: %v", err)
		}

		retrievedSnap, found, err := ss.GetLatestSnapshot(aggID)
		if err != nil {
			t.Fatalf("GetLatestSnapshot failed after save: %v", err)
		}
		if !found {
			t.Fatalf("Expected snapshot to be found, but wasn't")
		}
		if retrievedSnap == nil {
			t.Fatalf("Expected non-nil snapshot, got nil")
		}
		if retrievedSnap.AggregateID != aggID {
			t.Errorf("Expected AggregateID %s, got %s", aggID, retrievedSnap.AggregateID)
		}
		if retrievedSnap.Version != 5 {
			t.Errorf("Expected Version 5, got %d", retrievedSnap.Version)
		}
		// Check state content (basic check)
		var state map[string]interface{}
		if json.Unmarshal(retrievedSnap.State, &state) != nil || state["balance"] == nil || int(state["balance"].(float64)) != 100 {
			t.Errorf("Snapshot state content mismatch: %s", string(retrievedSnap.State))
		}

		// Check if it's a copy by modifying retrieved state
		retrievedSnap.State[0] = '{' // Modify the first byte
		retrievedSnap.Version = 99

		// Get again and check if original is unchanged
		originalSnap, _, _ := ss.GetLatestSnapshot(aggID)
		if originalSnap.Version == 99 {
			t.Errorf("GetLatestSnapshot did not return a copy (version modified)")
		}
		var originalState map[string]interface{}
		if json.Unmarshal(originalSnap.State, &originalState) != nil || originalState["balance"] == nil || int(originalState["balance"].(float64)) != 100 {
			t.Errorf("GetLatestSnapshot did not return a copy (state modified): %s", string(originalSnap.State))
		}
	})

	t.Run("OverwriteSnapshot", func(t *testing.T) {
		snap2 := newSnapshot(aggID, 10, map[string]interface{}{"balance": 200})
		err := ss.SaveSnapshot(snap2)
		if err != nil {
			t.Fatalf("SaveSnapshot (overwrite) failed: %v", err)
		}

		retrievedSnap, found, err := ss.GetLatestSnapshot(aggID)
		if err != nil {
			t.Fatalf("GetLatestSnapshot failed after overwrite: %v", err)
		}
		if !found {
			t.Fatalf("Expected snapshot to be found after overwrite")
		}
		if retrievedSnap.Version != 10 {
			t.Errorf("Expected overwritten Version 10, got %d", retrievedSnap.Version)
		}
		var state map[string]interface{}
		if json.Unmarshal(retrievedSnap.State, &state) != nil || state["balance"] == nil || int(state["balance"].(float64)) != 200 {
			t.Errorf("Snapshot state content mismatch after overwrite: %s", string(retrievedSnap.State))
		}
	})

	t.Run("SaveNilSnapshot", func(t *testing.T) {
		err := ss.SaveSnapshot(nil)
		if err == nil {
			t.Errorf("Expected error when saving nil snapshot, got nil")
		}
	})
}
