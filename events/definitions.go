package events

import (
	"github.com/shopspring/decimal"

	"financial-ledger/shared"
)

type AccountCreatedEvent struct {
	BaseEvent
	InitialBalances []shared.Balance `json:"initialBalances"`
}

type DepositMadeEvent struct {
	BaseEvent
	Amount   decimal.Decimal `json:"amount"`
	Currency shared.Currency `json:"currency"`
}

type WithdrawalMadeEvent struct {
	BaseEvent
	Amount   decimal.Decimal `json:"amount"`
	Currency shared.Currency `json:"currency"`
}

type MoneyTransferredEvent struct {
	BaseEvent
	TransferID       string          `json:"transferId,omitempty"` // Unique ID for the entire transfer operation
	SourceAccountID  string          `json:"sourceAccountId"`      // Account that was debited
	TargetAccountID  string          `json:"targetAccountId"`      // Account that was credited
	DebitedAmount    decimal.Decimal `json:"debitedAmount"`        // Amount taken from SourceAccountID
	DebitedCurrency  shared.Currency `json:"debitedCurrency"`
	CreditedAmount   decimal.Decimal `json:"creditedAmount"` // Amount given to TargetAccountID
	CreditedCurrency shared.Currency `json:"creditedCurrency"`
	ExchangeRate     decimal.Decimal `json:"exchangeRate"`
}

type CurrencyConvertedEvent struct {
	BaseEvent
	FromAmount   decimal.Decimal `json:"fromAmount"`
	FromCurrency shared.Currency `json:"fromCurrency"`
	ToAmount     decimal.Decimal `json:"toAmount"`
	ToCurrency   shared.Currency `json:"toCurrency"`
	ExchangeRate decimal.Decimal `json:"exchangeRate"`
}

type ExchangeRateUpdatedEvent struct {
	BaseEvent
	CurrencyPair         string          `json:"currencyPair"`
	ExchangeRate         decimal.Decimal `json:"exchangeRate"`
	PreviousExchangeRate decimal.Decimal `json:"previousExchangeRate"`
}
