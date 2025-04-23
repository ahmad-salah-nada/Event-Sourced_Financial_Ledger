package events

import (
	"time"

	"github.com/google/uuid"
)

type EventType string

type BaseEvent struct {
	EventID     uuid.UUID `json:"eventId"`
	AggregateID string    `json:"aggregateId"`
	Version     int       `json:"version"` // Version of the aggregate *after* this event is applied.
	Timestamp   time.Time `json:"timestamp"`
	Type        EventType `json:"type"`
}

type Event interface {
	GetBase() BaseEvent
}

func (e BaseEvent) GetBase() BaseEvent {
	return e
}

const (
	AccountCreatedType    EventType = "AccountCreated"
	DepositMadeType       EventType = "DepositMade"
	WithdrawalMadeType    EventType = "WithdrawalMade"
	MoneyTransferredType  EventType = "MoneyTransferred"
	CurrencyConvertedType EventType = "CurrencyConverted"
)

func NewBaseEvent(aggregateID string, version int, eventType EventType) BaseEvent {
	return BaseEvent{
		EventID:     uuid.New(),
		AggregateID: aggregateID,
		Version:     version, // The version *after* this event.
		Timestamp:   time.Now().UTC(),
		Type:        eventType,
	}
}
