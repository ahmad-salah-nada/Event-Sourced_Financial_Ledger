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
	TargetAccountID  string          `json:"targetAccountId"`
	DebitedAmount    decimal.Decimal `json:"debitedAmount"`
	DebitedCurrency  shared.Currency `json:"debitedCurrency"`
	CreditedAmount   decimal.Decimal `json:"creditedAmount"`
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
