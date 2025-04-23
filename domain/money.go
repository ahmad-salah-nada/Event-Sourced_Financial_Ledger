package domain

import (
	"fmt"

	"github.com/shopspring/decimal"

	"financial-ledger/shared"
)

type Money struct {
	Amount   decimal.Decimal `json:"amount"`
	Currency shared.Currency `json:"currency"`
}

func NewMoney(amount decimal.Decimal, currency shared.Currency) Money {
	return Money{Amount: amount, Currency: currency}
}

func (m Money) Add(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("currency mismatch: cannot add %s and %s", m.Currency, other.Currency)
	}
	return NewMoney(m.Amount.Add(other.Amount), m.Currency), nil
}

func (m Money) Subtract(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, fmt.Errorf("currency mismatch: cannot subtract %s from %s", other.Currency, m.Currency)
	}
	return NewMoney(m.Amount.Sub(other.Amount), m.Currency), nil
}

func (m Money) IsZero() bool {
	return m.Amount.IsZero()
}

func (m Money) IsNegative() bool {
	return m.Amount.IsNegative()
}

func (m Money) IsPositive() bool {
	return m.Amount.IsPositive()
}

func (m Money) GreaterThan(other Money) (bool, error) {
	if m.Currency != other.Currency {
		return false, fmt.Errorf("currency mismatch: cannot compare %s and %s", m.Currency, other.Currency)
	}
	return m.Amount.GreaterThan(other.Amount), nil
}

func (m Money) GreaterThanOrEqual(other Money) (bool, error) {
	if m.Currency != other.Currency {
		return false, fmt.Errorf("currency mismatch: cannot compare %s and %s", m.Currency, other.Currency)
	}
	return m.Amount.GreaterThanOrEqual(other.Amount), nil
}
