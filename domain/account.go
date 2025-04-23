package domain

import (
	"fmt"
	"log"

	"github.com/shopspring/decimal"

	"financial-ledger/events"
	"financial-ledger/shared"
)

// Account is the central entity in our domain, acting as the Aggregate Root.
// It encapsulates the state (balances) and enforces business rules (invariants)
// through command handlers and event application.
type Account struct {
	ID       string                              `json:"id"`
	Balances map[shared.Currency]decimal.Decimal `json:"balances"`
	Version  int                                 `json:"version"`

	changes []events.Event
}

func NewAccount(id string) *Account {
	return &Account{
		ID:       id,
		Balances: make(map[shared.Currency]decimal.Decimal),
		Version:  0,
		changes:  make([]events.Event, 0),
	}
}

func (a *Account) GetUncommitedChanges() []events.Event {
	unCommittedChanges := a.changes
	a.changes = make([]events.Event, 0)
	return unCommittedChanges
}

func (a *Account) handleChange(event events.Event) error {
	if err := a.ApplyEvent(event); err != nil {
		log.Printf("ERROR: Internal Apply failed for event %T on account %s: %v", event, a.ID, err)
		return fmt.Errorf("internal error applying event %T: %w", event, err)
	}
	a.changes = append(a.changes, event)
	return nil
}

// --- Command Handlers ---
// These methods are the public interface for mutating the Account aggregate.
// They receive command data, validate business rules (invariants), and if valid,
// create and track domain events representing the change.

func (a *Account) HandleCreateAccount(id string, initialBalances map[shared.Currency]decimal.Decimal) error {
	if a.Version > 0 {
		return fmt.Errorf("%w: account %s (current version %d)", ErrAccountExists, a.ID, a.Version)
	}
	if id == "" {
		return NewDomainError("account ID cannot be empty")
	}

	balanceEntries := make([]shared.Balance, 0, len(initialBalances))
	for cur, amt := range initialBalances {
		if amt.IsNegative() {
			return NewDomainError("initial balance for %s cannot be negative: %s", cur, amt.String())
		}
		balanceEntries = append(balanceEntries, shared.Balance{Currency: cur, Amount: amt})
	}

	event := events.AccountCreatedEvent{
		BaseEvent:       events.NewBaseEvent(id, a.Version+1, events.AccountCreatedType),
		InitialBalances: balanceEntries,
	}

	return a.handleChange(event)
}

func (a *Account) HandleDeposit(amount decimal.Decimal, currency shared.Currency) error {
	if a.ID == "" || a.Version == 0 {
		return NewDomainError("cannot deposit to uninitialized account")
	}

	if !amount.IsPositive() {
		return NewDomainError("deposit amount must be positive: %s", amount.String())
	}

	event := events.DepositMadeEvent{
		BaseEvent: events.NewBaseEvent(a.ID, a.Version+1, events.DepositMadeType),
		Amount:    amount,
		Currency:  currency,
	}
	return a.handleChange(event)
}

func (a *Account) HandleWithdraw(amount decimal.Decimal, currency shared.Currency) error {
	if a.ID == "" || a.Version == 0 {
		return NewDomainError("cannot withdraw from uninitialized account")
	}
	if !amount.IsPositive() {
		return NewDomainError("withdrawal amount must be positive: %s", amount.String())
	}

	currentBalance := a.getBalance(currency)
	required := NewMoney(amount, currency)
	available := NewMoney(currentBalance, currency)

	sufficient, _ := available.GreaterThanOrEqual(required)
	if !sufficient {
		return fmt.Errorf("%w: requested %s %s, available %s %s",
			ErrInsufficientFunds, amount.String(), currency, currentBalance.String(), currency)
	}

	event := events.WithdrawalMadeEvent{
		BaseEvent: events.NewBaseEvent(a.ID, a.Version+1, events.WithdrawalMadeType),
		Amount:    amount,
		Currency:  currency,
	}
	return a.handleChange(event)
}

func (a *Account) HandleConvertCurrency(fromAmount decimal.Decimal, fromCurrency, toCurrency shared.Currency, exchangeRate decimal.Decimal) error {
	if a.ID == "" || a.Version == 0 {
		return NewDomainError("cannot convert currency for uninitialized account")
	}
	if !fromAmount.IsPositive() {
		return NewDomainError("conversion amount must be positive: %s", fromAmount.String())
	}
	if fromCurrency == toCurrency {
		return NewDomainError("cannot convert currency %s to itself", fromCurrency)
	}
	if !exchangeRate.IsPositive() {
		return NewDomainError("exchange rate must be positive: %s", exchangeRate.String())
	}

	currentFromBalance := a.getBalance(fromCurrency)
	required := NewMoney(fromAmount, fromCurrency)
	available := NewMoney(currentFromBalance, fromCurrency)

	sufficient, _ := available.GreaterThanOrEqual(required)
	if !sufficient {
		return fmt.Errorf("%w: requested %s %s, available %s %s for conversion",
			ErrInsufficientFunds, fromAmount.String(), fromCurrency, currentFromBalance.String(), fromCurrency)
	}

	toAmount := fromAmount.Mul(exchangeRate)

	event := events.CurrencyConvertedEvent{
		BaseEvent:    events.NewBaseEvent(a.ID, a.Version+1, events.CurrencyConvertedType),
		FromAmount:   fromAmount,
		FromCurrency: fromCurrency,
		ToAmount:     toAmount,
		ToCurrency:   toCurrency,
		ExchangeRate: exchangeRate,
	}
	return a.handleChange(event)
}

func (a *Account) HandleTransferMoney(targetAccountID string, debitAmount decimal.Decimal, debitCurrency shared.Currency, creditAmount decimal.Decimal, creditCurrency shared.Currency, rate decimal.Decimal) error {
	if a.ID == "" || a.Version == 0 {
		return NewDomainError("cannot transfer from uninitialized account")
	}
	if !debitAmount.IsPositive() {
		return NewDomainError("transfer amount must be positive: %s", debitAmount.String())
	}
	if targetAccountID == "" {
		return NewDomainError("target account ID cannot be empty")
	}
	if targetAccountID == a.ID {
		return NewDomainError("cannot transfer funds to the same account")
	}

	currentBalance := a.getBalance(debitCurrency)
	required := NewMoney(debitAmount, debitCurrency)
	available := NewMoney(currentBalance, debitCurrency)

	sufficient, _ := available.GreaterThanOrEqual(required)
	if !sufficient {
		return fmt.Errorf("%w: requested %s %s, available %s %s for transfer",
			ErrInsufficientFunds, debitAmount.String(), debitCurrency, currentBalance.String(), debitCurrency)
	}

	if debitCurrency != creditCurrency {
		if !rate.IsPositive() {
			return NewDomainError("exchange rate must be positive for cross-currency transfer: %s", rate.String())
		}
		calculatedCredit := debitAmount.Mul(rate)
		if !calculatedCredit.Equal(creditAmount) {
			log.Printf("Warning: HandleTransferMoney - Provided credit amount %s %s differs from calculation %s %s using rate %s for account %s",
				creditAmount.String(), creditCurrency, calculatedCredit.String(), creditCurrency, rate.String(), a.ID)

			// return NewDomainError("provided credited amount %s does not match calculated amount %s using rate %s", creditAmount.String(), calculatedCredit.String(), rate.String())
		}
	} else {
		if !creditAmount.Equal(debitAmount) {
			return NewDomainError("debit (%s) and credit (%s) amounts must match for same-currency transfer (%s)", debitAmount.String(), creditAmount.String(), debitCurrency)
		}
		if !rate.Equal(decimal.NewFromInt(1)) {
			log.Printf("Warning: HandleTransferMoney - Rate for same-currency transfer (%s) was %s, expected 1. Using 1.", debitCurrency, rate.String())
			rate = decimal.NewFromInt(1)
		}
	}

	event := events.MoneyTransferredEvent{
		BaseEvent:        events.NewBaseEvent(a.ID, a.Version+1, events.MoneyTransferredType),
		TargetAccountID:  targetAccountID,
		DebitedAmount:    debitAmount,
		DebitedCurrency:  debitCurrency,
		CreditedAmount:   creditAmount,
		CreditedCurrency: creditCurrency,
		ExchangeRate:     rate,
	}
	return a.handleChange(event)
}

func (a *Account) ApplyEvent(event events.Event) error {
	base := event.GetBase()

	if base.Version != a.Version+1 {
		return fmt.Errorf("apply failed: event version mismatch for account %s: expected %d, got %d for event %T (%s)",
			a.ID, a.Version+1, base.Version, event, base.EventID)
	}

	switch e := event.(type) {
	case events.AccountCreatedEvent:
		a.ID = e.AggregateID
		a.Balances = make(map[shared.Currency]decimal.Decimal)
		for _, balance := range e.InitialBalances {
			a.Balances[balance.Currency] = balance.Amount
		}
	case events.DepositMadeEvent:
		currentBalance := a.getBalance(e.Currency)
		a.Balances[e.Currency] = currentBalance.Add(e.Amount)
	case events.WithdrawalMadeEvent:
		currentBalance := a.getBalance(e.Currency)
		newBalance := currentBalance.Sub(e.Amount)
		if newBalance.IsNegative() {
			log.Printf("CRITICAL: Invariant Violation! Account %s balance for %s negative after applying %T (v%d): %s - %s = %s",
				a.ID, e.Currency, event, base.Version, currentBalance.String(), e.Amount.String(), newBalance.String())
			return fmt.Errorf("invariant violation: negative balance applying %T (v%d)", event, base.Version)
		}
		a.Balances[e.Currency] = newBalance
	case events.CurrencyConvertedEvent:
		currentFrom := a.getBalance(e.FromCurrency)
		newFrom := currentFrom.Sub(e.FromAmount)
		if newFrom.IsNegative() {
			log.Printf("CRITICAL: Invariant Violation! Account %s balance for %s negative after applying debit part of %T (v%d): %s - %s = %s",
				a.ID, e.FromCurrency, event, base.Version, currentFrom.String(), e.FromAmount.String(), newFrom.String())
			return fmt.Errorf("invariant violation: negative balance applying debit of %T (v%d)", event, base.Version)
		}
		a.Balances[e.FromCurrency] = newFrom

		currentTo := a.getBalance(e.ToCurrency)
		a.Balances[e.ToCurrency] = currentTo.Add(e.ToAmount)
	case events.MoneyTransferredEvent:
		currentBalance := a.getBalance(e.DebitedCurrency)
		newBalance := currentBalance.Sub(e.DebitedAmount)
		if newBalance.IsNegative() {
			log.Printf("CRITICAL: Invariant Violation! Account %s balance for %s negative after applying debit part of %T (v%d): %s - %s = %s",
				a.ID, e.DebitedCurrency, event, base.Version, currentBalance.String(), e.DebitedAmount.String(), newBalance.String())
			return fmt.Errorf("invariant violation: negative balance applying debit of %T (v%d)", event, base.Version)
		}
		a.Balances[e.DebitedCurrency] = newBalance
	default:
		return fmt.Errorf("apply failed: unknown event type %T for account %s", event, a.ID)
	}

	a.Version = base.Version
	return nil
}

func (a *Account) ApplyEvents(history []events.Event) error {
	for _, event := range history {
		if err := a.ApplyEvent(event); err != nil {
			base := event.GetBase()
			log.Printf("Error applying event during reconstruction: ID=%s, Type=%T, Version=%d, AggregateID=%s\n", base.EventID, event, base.Version, base.AggregateID)
			return fmt.Errorf("failed to apply event %s (%T) at version %d during reconstruction: %w", base.EventID, event, base.Version, err)
		}
	}
	return nil
}

func (a *Account) getBalance(currency shared.Currency) decimal.Decimal {
	balance, ok := a.Balances[currency]
	if !ok {
		return decimal.Zero
	}
	return balance
}
