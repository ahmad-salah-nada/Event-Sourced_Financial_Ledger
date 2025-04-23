package app

import (
	"github.com/shopspring/decimal"

	"financial-ledger/shared"
)

// --- Command Struct Definitions ---
// Commands represent the intent to perform an action or change state in the system.

type CreateAccountCommand struct {
	AccountID       string
	InitialBalances map[shared.Currency]decimal.Decimal
}

type DepositMoneyCommand struct {
	AccountID string
	Amount    decimal.Decimal
	Currency  shared.Currency
}

type WithdrawMoneyCommand struct {
	AccountID string
	Amount    decimal.Decimal
	Currency  shared.Currency
}

type TransferMoneyCommand struct {
	SourceAccountID string
	TargetAccountID string
	Amount          decimal.Decimal
	Currency        shared.Currency
}

type ConvertCurrencyCommand struct {
	AccountID    string
	FromAmount   decimal.Decimal
	FromCurrency shared.Currency
	ToCurrency   shared.Currency
}

// --- Query Structures (Input for Read Operations) ---

type GetBalanceQuery struct {
	AccountID string
	Currency  *shared.Currency
}

type GetHistoryQuery struct {
	AccountID string
	Limit     int
	Skip      int
}
