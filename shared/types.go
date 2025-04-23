package shared

import "github.com/shopspring/decimal"

type Currency string

const (
	USD Currency = "USD"
	EUR Currency = "EUR"
	GBP Currency = "GBP"
)

type Balance struct {
	Currency Currency        `json:"currency"`
	Amount   decimal.Decimal `json:"amount"`
}
