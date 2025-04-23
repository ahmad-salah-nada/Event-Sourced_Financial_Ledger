package domain

import (
	"encoding/json"
	"financial-ledger/events"
	"financial-ledger/shared"
	"fmt"
	"log"
	"time"

	"github.com/shopspring/decimal"
)

type Snapshot struct {
	AggregateID string    `json:"aggregateId"`
	Version     int       `json:"version"`
	State       []byte    `json:"state"`
	Timestamp   time.Time `json:"timestamp"`
}

func CreateSnapshot(account *Account) (*Snapshot, error) {
	stateJSON, err := json.Marshal(account)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal account state for snapshot (ID: %s, Version: %d): %w", account.ID, account.Version, err)
	}

	return &Snapshot{
		AggregateID: account.ID,
		Version:     account.Version,
		State:       stateJSON,
		Timestamp:   time.Now().UTC(),
	}, nil
}

func ApplySnapshot(snap *Snapshot) (*Account, error) {
	var account Account
	err := json.Unmarshal(snap.State, &account)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal snapshot state into account (ID: %s, Version: %d): %w", snap.AggregateID, snap.Version, err)
	}

	if account.ID != snap.AggregateID || account.Version != snap.Version {
		log.Printf("Warning: Snapshot/State mismatch after unmarshal for %s. Snapshot Version: %d, State Version: %d. Snapshot ID: %s, State ID: %s. Overwriting state with snapshot metadata.",
			snap.AggregateID, snap.Version, account.Version, snap.AggregateID, account.ID)

		account.ID = snap.AggregateID
		account.Version = snap.Version
	}

	account.changes = make([]events.Event, 0)

	if account.Balances == nil {
		log.Printf("Warning: Account balances map was nil after unmarshalling snapshot for %s (v%d). Initializing.", account.ID, account.Version)
		account.Balances = make(map[shared.Currency]decimal.Decimal)
	}

	return &account, nil
}
