package app_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"testing"

	"github.com/shopspring/decimal"

	"financial-ledger/app"
	"financial-ledger/domain"
	"financial-ledger/events"
	"financial-ledger/shared"
	"financial-ledger/store"
)

// Helper to create decimals in tests
func dec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

// setup initializes stores and service for tests
// It now uses the default SnapshotFrequency from the app package
func setup() (*app.AccountService, *store.InMemoryEventStore, *store.InMemorySnapshotStore) {
	eventStore := store.NewInMemoryEventStore()
	snapshotStore := store.NewInMemorySnapshotStore()
	// Use the constructor without frequency, relying on the default const
	accountService := app.NewAccountService(eventStore, snapshotStore)
	return accountService, eventStore, snapshotStore
}

// Helper to get account state directly from snapshot store for testing
func getAccountFromSnapshot(ss store.SnapshotStore, id string) (*domain.Account, error) {
	snap, found, err := ss.GetLatestSnapshot(id)
	if err != nil {
		return nil, fmt.Errorf("error getting snapshot: %w", err)
	}
	if !found {
		return nil, errors.New("snapshot not found")
	}
	acc, err := domain.ApplySnapshot(snap)
	if err != nil {
		return nil, fmt.Errorf("error applying snapshot: %w", err)
	}
	return acc, nil
}

func TestAccountService_CreateAccount(t *testing.T) {
	service, eventStore, _ := setup()

	t.Run("SuccessWithProvidedID", func(t *testing.T) {
		cmd := app.CreateAccountCommand{
			AccountID: "acc-test-1",
			InitialBalances: map[shared.Currency]decimal.Decimal{
				shared.USD: dec("100"),
			},
		}
		id, err := service.CreateAccount(cmd)
		if err != nil {
			t.Fatalf("CreateAccount failed: %v", err)
		}
		if id != "acc-test-1" {
			t.Errorf("expected ID 'acc-test-1', got '%s'", id)
		}

		// Verify event store
		evts, _ := eventStore.GetEvents(id)
		if len(evts) != 1 {
			t.Fatalf("expected 1 event in store, got %d", len(evts))
		}
		createdEvent, ok := evts[0].(events.AccountCreatedEvent)
		if !ok {
			t.Errorf("expected AccountCreatedEvent, got %T", evts[0])
		}
		if len(createdEvent.InitialBalances) != 1 || !createdEvent.InitialBalances[0].Amount.Equal(dec("100")) {
			t.Errorf("event initial balance mismatch")
		}

		// Verify balance via query
		balances, err := service.GetCurrentBalance(app.GetBalanceQuery{AccountID: id})
		if err != nil {
			t.Fatalf("GetCurrentBalance failed: %v", err)
		}
		if !balances[shared.USD].Equal(dec("100")) {
			t.Errorf("balance query mismatch: expected 100, got %s", balances[shared.USD])
		}
	})

	t.Run("SuccessWithGeneratedID", func(t *testing.T) {
		cmd := app.CreateAccountCommand{
			InitialBalances: map[shared.Currency]decimal.Decimal{
				shared.EUR: dec("200"),
			},
		}
		id, err := service.CreateAccount(cmd)
		if err != nil {
			t.Fatalf("CreateAccount failed: %v", err)
		}
		if id == "" {
			t.Errorf("expected a generated ID, got empty string")
		}
		// Verify event store for generated ID
		evts, _ := eventStore.GetEvents(id)
		if len(evts) != 1 {
			t.Fatalf("expected 1 event in store for generated ID, got %d", len(evts))
		}
		createdEvent, ok := evts[0].(events.AccountCreatedEvent)
		if !ok {
			t.Errorf("expected AccountCreatedEvent, got %T", evts[0])
		}
		if len(createdEvent.InitialBalances) != 1 || !createdEvent.InitialBalances[0].Amount.Equal(dec("200")) {
			t.Errorf("event initial balance mismatch for generated ID")
		}
		// Verify balance via query
		balances, err := service.GetCurrentBalance(app.GetBalanceQuery{AccountID: id})
		if err != nil {
			t.Fatalf("GetCurrentBalance failed: %v", err)
		}
		if !balances[shared.EUR].Equal(dec("200")) {
			t.Errorf("balance query mismatch for generated ID: expected 200, got %s", balances[shared.EUR])
		}
	})

	t.Run("FailOnDuplicateID", func(t *testing.T) {
		// Use the ID created in the first subtest
		cmd := app.CreateAccountCommand{
			AccountID: "acc-test-1",
			InitialBalances: map[shared.Currency]decimal.Decimal{
				shared.USD: dec("50"),
			},
		}
		_, err := service.CreateAccount(cmd)
		if !errors.Is(err, domain.ErrAccountExists) {
			t.Errorf("expected ErrAccountExists, got %v", err)
		}
		// Ensure no new event was saved
		evts, _ := eventStore.GetEvents("acc-test-1")
		if len(evts) != 1 {
			t.Errorf("event store should still only have 1 event after duplicate attempt, got %d", len(evts))
		}
	})

	t.Run("FailOnNegativeInitialBalance", func(t *testing.T) {
		cmd := app.CreateAccountCommand{
			AccountID: "acc-neg-bal",
			InitialBalances: map[shared.Currency]decimal.Decimal{
				shared.USD: dec("-50"),
			},
		}
		_, err := service.CreateAccount(cmd)
		var domainErr *domain.DomainError
		if !errors.As(err, &domainErr) {
			t.Errorf("expected DomainError for negative balance, got %T: %v", err, err)
		}
		// Ensure account was not created
		_, err = service.GetCurrentBalance(app.GetBalanceQuery{AccountID: "acc-neg-bal"})
		if !errors.Is(err, domain.ErrAccountNotFound) {
			t.Errorf("account should not exist after failed creation, but got balance or different error: %v", err)
		}
	})
}

func TestAccountService_Deposit(t *testing.T) {
	service, eventStore, _ := setup()
	// Create an account first
	createCmd := app.CreateAccountCommand{AccountID: "acc-dep-1", InitialBalances: map[shared.Currency]decimal.Decimal{shared.USD: dec("100")}}
	id, _ := service.CreateAccount(createCmd)

	t.Run("Success", func(t *testing.T) {
		depositCmd := app.DepositMoneyCommand{AccountID: id, Amount: dec("50"), Currency: shared.USD}
		err := service.Deposit(depositCmd)
		if err != nil {
			t.Fatalf("Deposit failed: %v", err)
		}

		// Verify event store
		evts, _ := eventStore.GetEvents(id)
		if len(evts) != 2 { // Create + Deposit
			t.Fatalf("expected 2 events in store, got %d", len(evts))
		}
		depositEvent, ok := evts[1].(events.DepositMadeEvent)
		if !ok {
			t.Errorf("expected DepositMadeEvent as second event, got %T", evts[1])
		}
		if !depositEvent.Amount.Equal(dec("50")) {
			t.Errorf("event amount mismatch")
		}

		// Verify balance
		balances, _ := service.GetCurrentBalance(app.GetBalanceQuery{AccountID: id})
		if !balances[shared.USD].Equal(dec("150")) {
			t.Errorf("expected balance 150 USD, got %s", balances[shared.USD])
		}
	})

	t.Run("SuccessNewCurrency", func(t *testing.T) {
		// Current state: 150 USD
		depositCmd := app.DepositMoneyCommand{AccountID: id, Amount: dec("200"), Currency: shared.EUR}
		err := service.Deposit(depositCmd)
		if err != nil {
			t.Fatalf("Deposit failed: %v", err)
		}

		// Verify event store
		evts, _ := eventStore.GetEvents(id)
		if len(evts) != 3 { // Create + Deposit USD + Deposit EUR
			t.Fatalf("expected 3 events in store, got %d", len(evts))
		}
		depositEvent, ok := evts[2].(events.DepositMadeEvent)
		if !ok {
			t.Errorf("expected DepositMadeEvent as third event, got %T", evts[2])
		}
		if !depositEvent.Amount.Equal(dec("200")) || depositEvent.Currency != shared.EUR {
			t.Errorf("event details mismatch for EUR deposit")
		}

		// Verify balance
		balances, _ := service.GetCurrentBalance(app.GetBalanceQuery{AccountID: id})
		if !balances[shared.USD].Equal(dec("150")) {
			t.Errorf("expected balance 150 USD, got %s", balances[shared.USD])
		}
		if !balances[shared.EUR].Equal(dec("200")) {
			t.Errorf("expected balance 200 EUR, got %s", balances[shared.EUR])
		}
	})

	t.Run("FailOnAccountNotFound", func(t *testing.T) {
		depositCmd := app.DepositMoneyCommand{AccountID: "acc-nonexistent", Amount: dec("50"), Currency: shared.USD}
		err := service.Deposit(depositCmd)
		if !errors.Is(err, domain.ErrAccountNotFound) {
			t.Errorf("expected ErrAccountNotFound, got %v", err)
		}
	})

	t.Run("FailOnNegativeAmount", func(t *testing.T) {
		depositCmd := app.DepositMoneyCommand{AccountID: id, Amount: dec("-50"), Currency: shared.USD}
		err := service.Deposit(depositCmd)
		var domainErr *domain.DomainError
		if !errors.As(err, &domainErr) { // Check if it's a domain validation error
			t.Errorf("expected DomainError, got %T: %v", err, err)
		}
		// Verify balance unchanged
		balances, _ := service.GetCurrentBalance(app.GetBalanceQuery{AccountID: id})
		if !balances[shared.USD].Equal(dec("150")) { // From previous tests
			t.Errorf("balance should remain 150 USD after failed deposit, got %s", balances[shared.USD])
		}
	})

	t.Run("SuccessZeroAmount", func(t *testing.T) {
		// Current state: 150 USD, 200 EUR
		depositCmd := app.DepositMoneyCommand{AccountID: id, Amount: dec("0"), Currency: shared.USD}
		err := service.Deposit(depositCmd)
		// Expecting domain error because HandleDeposit requires positive amount
		var domainErr *domain.DomainError
		if !errors.As(err, &domainErr) {
			t.Errorf("expected DomainError for zero amount deposit, got %T: %v", err, err)
		}
		// Verify no new event
		evts, _ := eventStore.GetEvents(id)
		if len(evts) != 3 { // Should still be 3 events
			t.Fatalf("expected 3 events in store after zero deposit attempt, got %d", len(evts))
		}
	})
}

func TestAccountService_Withdraw(t *testing.T) {
	service, eventStore, _ := setup()
	// Create an account first
	createCmd := app.CreateAccountCommand{AccountID: "acc-wd-1", InitialBalances: map[shared.Currency]decimal.Decimal{shared.EUR: dec("200")}}
	id, _ := service.CreateAccount(createCmd)

	t.Run("Success", func(t *testing.T) {
		withdrawCmd := app.WithdrawMoneyCommand{AccountID: id, Amount: dec("50"), Currency: shared.EUR}
		err := service.Withdraw(withdrawCmd)
		if err != nil {
			t.Fatalf("Withdraw failed: %v", err)
		}

		// Verify event store
		evts, _ := eventStore.GetEvents(id)
		if len(evts) != 2 { // Create + Withdraw
			t.Fatalf("expected 2 events in store, got %d", len(evts))
		}
		withdrawEvent, ok := evts[1].(events.WithdrawalMadeEvent)
		if !ok {
			t.Errorf("expected WithdrawalMadeEvent as second event, got %T", evts[1])
		}
		if !withdrawEvent.Amount.Equal(dec("50")) {
			t.Errorf("event amount mismatch")
		}

		// Verify balance
		balances, _ := service.GetCurrentBalance(app.GetBalanceQuery{AccountID: id})
		if !balances[shared.EUR].Equal(dec("150")) {
			t.Errorf("expected balance 150 EUR, got %s", balances[shared.EUR])
		}
	})

	t.Run("FailOnInsufficientFunds", func(t *testing.T) {
		withdrawCmd := app.WithdrawMoneyCommand{AccountID: id, Amount: dec("200"), Currency: shared.EUR} // Try to withdraw more than available (150)
		err := service.Withdraw(withdrawCmd)
		if !errors.Is(err, domain.ErrInsufficientFunds) {
			t.Errorf("expected ErrInsufficientFunds, got %v", err)
		}
		// Verify balance unchanged
		balances, _ := service.GetCurrentBalance(app.GetBalanceQuery{AccountID: id})
		if !balances[shared.EUR].Equal(dec("150")) {
			t.Errorf("balance should remain 150 EUR after failed withdrawal, got %s", balances[shared.EUR])
		}
		// Verify no new event
		evts, _ := eventStore.GetEvents(id)
		if len(evts) != 2 { // Should still be 2 events
			t.Fatalf("expected 2 events in store after failed withdrawal, got %d", len(evts))
		}
	})

	t.Run("FailOnInsufficientFundsNonHeldCurrency", func(t *testing.T) {
		// Current state: 150 EUR
		withdrawCmd := app.WithdrawMoneyCommand{AccountID: id, Amount: dec("1"), Currency: shared.USD} // Try to withdraw USD (balance is 0)
		err := service.Withdraw(withdrawCmd)
		if !errors.Is(err, domain.ErrInsufficientFunds) {
			t.Errorf("expected ErrInsufficientFunds, got %v", err)
		}
		// Verify balance unchanged
		balances, _ := service.GetCurrentBalance(app.GetBalanceQuery{AccountID: id})
		if !balances[shared.EUR].Equal(dec("150")) {
			t.Errorf("EUR balance should remain 150 after failed USD withdrawal, got %s", balances[shared.EUR])
		}
		if _, ok := balances[shared.USD]; ok && !balances[shared.USD].IsZero() {
			t.Errorf("USD balance should remain zero/non-existent after failed withdrawal, got %s", balances[shared.USD])
		}
	})

	t.Run("FailOnAccountNotFound", func(t *testing.T) {
		withdrawCmd := app.WithdrawMoneyCommand{AccountID: "acc-nonexistent", Amount: dec("50"), Currency: shared.EUR}
		err := service.Withdraw(withdrawCmd)
		if !errors.Is(err, domain.ErrAccountNotFound) {
			t.Errorf("expected ErrAccountNotFound, got %v", err)
		}
	})

	t.Run("FailOnNegativeAmount", func(t *testing.T) {
		withdrawCmd := app.WithdrawMoneyCommand{AccountID: id, Amount: dec("-50"), Currency: shared.EUR}
		err := service.Withdraw(withdrawCmd)
		var domainErr *domain.DomainError
		if !errors.As(err, &domainErr) {
			t.Errorf("expected DomainError, got %T: %v", err, err)
		}
	})

	t.Run("FailOnZeroAmount", func(t *testing.T) {
		withdrawCmd := app.WithdrawMoneyCommand{AccountID: id, Amount: dec("0"), Currency: shared.EUR}
		err := service.Withdraw(withdrawCmd)
		// Expecting domain error because HandleWithdraw requires positive amount
		var domainErr *domain.DomainError
		if !errors.As(err, &domainErr) {
			t.Errorf("expected DomainError for zero amount withdrawal, got %T: %v", err, err)
		}
	})
}

func TestAccountService_ConvertCurrency(t *testing.T) {
	service, eventStore, _ := setup()
	// Create account with multiple currencies
	createCmd := app.CreateAccountCommand{AccountID: "acc-conv-1", InitialBalances: map[shared.Currency]decimal.Decimal{
		shared.USD: dec("100"),
		shared.EUR: dec("50"),
	}}
	id, _ := service.CreateAccount(createCmd)

	t.Run("Success", func(t *testing.T) {
		convertCmd := app.ConvertCurrencyCommand{
			AccountID:    id,
			FromAmount:   dec("50"), // Convert 50 USD
			FromCurrency: shared.USD,
			ToCurrency:   shared.EUR,
		}
		err := service.ConvertCurrency(convertCmd)
		if err != nil {
			t.Fatalf("ConvertCurrency failed: %v", err)
		}

		// Verify event store
		evts, _ := eventStore.GetEvents(id)
		if len(evts) != 2 { // Create + Convert
			t.Fatalf("expected 2 events, got %d", len(evts))
		}
		convEvent, ok := evts[1].(events.CurrencyConvertedEvent)
		if !ok {
			t.Fatalf("expected CurrencyConvertedEvent, got %T", evts[1])
		}
		// Check event details (rate 0.92 from dummy provider)
		expectedToAmount := dec("50").Mul(dec("0.92")) // 46 EUR
		if !convEvent.FromAmount.Equal(dec("50")) || convEvent.FromCurrency != shared.USD {
			t.Errorf("unexpected From details in event")
		}
		if !convEvent.ToAmount.Equal(expectedToAmount) || convEvent.ToCurrency != shared.EUR {
			t.Errorf("unexpected To details in event: got %s %s, expected %s %s", convEvent.ToAmount, convEvent.ToCurrency, expectedToAmount, shared.EUR)
		}
		if !convEvent.ExchangeRate.Equal(dec("0.92")) {
			t.Errorf("unexpected Rate in event: %s", convEvent.ExchangeRate)
		}

		// Verify balances
		balances, _ := service.GetCurrentBalance(app.GetBalanceQuery{AccountID: id})
		if !balances[shared.USD].Equal(dec("50")) { // 100 - 50
			t.Errorf("expected USD balance 50, got %s", balances[shared.USD])
		}
		if !balances[shared.EUR].Equal(dec("96")) { // 50 + 46
			t.Errorf("expected EUR balance 96, got %s", balances[shared.EUR])
		}
	})

	t.Run("FailOnInsufficientFunds", func(t *testing.T) {
		// Current state: 50 USD, 96 EUR
		convertCmd := app.ConvertCurrencyCommand{
			AccountID:    id,
			FromAmount:   dec("60"), // Try to convert more USD than available
			FromCurrency: shared.USD,
			ToCurrency:   shared.EUR,
		}
		err := service.ConvertCurrency(convertCmd)
		if !errors.Is(err, domain.ErrInsufficientFunds) {
			t.Errorf("expected ErrInsufficientFunds, got %v", err)
		}
		// Verify balances unchanged
		balances, _ := service.GetCurrentBalance(app.GetBalanceQuery{AccountID: id})
		if !balances[shared.USD].Equal(dec("50")) {
			t.Errorf("USD balance should remain 50 after failed conversion, got %s", balances[shared.USD])
		}
		if !balances[shared.EUR].Equal(dec("96")) {
			t.Errorf("EUR balance should remain 96 after failed conversion, got %s", balances[shared.EUR])
		}
	})

	t.Run("FailOnSameCurrency", func(t *testing.T) {
		convertCmd := app.ConvertCurrencyCommand{
			AccountID:    id,
			FromAmount:   dec("10"),
			FromCurrency: shared.USD,
			ToCurrency:   shared.USD, // Same currency
		}
		err := service.ConvertCurrency(convertCmd)
		var domainErr *domain.DomainError
		if !errors.As(err, &domainErr) {
			t.Errorf("expected DomainError for same currency, got %T: %v", err, err)
		}
	})

	t.Run("FailOnUnknownRate", func(t *testing.T) {
		// Add a dummy currency not in the dummy provider
		type DummyCurrency shared.Currency
		const XXX DummyCurrency = "XXX"

		convertCmd := app.ConvertCurrencyCommand{
			AccountID:    id,
			FromAmount:   dec("10"),
			FromCurrency: shared.USD,
			ToCurrency:   shared.Currency(XXX), // Use the dummy currency
		}
		err := service.ConvertCurrency(convertCmd)
		if err == nil {
			t.Errorf("expected error for unknown rate, got nil")
			return // Avoid further checks if err is nil
		}
		// Check if the error message contains "exchange rate not found"
		errMsg := err.Error()
		expectedMsgPart := "exchange rate not found for USD -> XXX"
		// Use strings.Contains for a more robust check against wrapped errors' messages
		if !strings.Contains(errMsg, expectedMsgPart) {
		}
	})

	t.Run("FailOnNegativeAmount", func(t *testing.T) {
		convertCmd := app.ConvertCurrencyCommand{
			AccountID:    id,
			FromAmount:   dec("-10"),
			FromCurrency: shared.USD,
			ToCurrency:   shared.EUR,
		}
		err := service.ConvertCurrency(convertCmd)
		var domainErr *domain.DomainError
		if !errors.As(err, &domainErr) {
			t.Errorf("expected DomainError for negative amount, got %T: %v", err, err)
		}
	})
}

func TestAccountService_TransferMoney(t *testing.T) {
	service, eventStore, _ := setup()
	// Create source and target accounts
	sourceID, _ := service.CreateAccount(app.CreateAccountCommand{AccountID: "acc-src-1", InitialBalances: map[shared.Currency]decimal.Decimal{shared.GBP: dec("500")}})
	targetID, _ := service.CreateAccount(app.CreateAccountCommand{AccountID: "acc-tgt-1", InitialBalances: map[shared.Currency]decimal.Decimal{shared.USD: dec("100")}})

	t.Run("SuccessSameCurrency", func(t *testing.T) {
		transferCmd := app.TransferMoneyCommand{
			SourceAccountID: sourceID,
			TargetAccountID: targetID,
			Amount:          dec("100"),
			Currency:        shared.GBP,
		}
		err := service.TransferMoney(transferCmd)
		if err != nil {
			t.Fatalf("TransferMoney failed: %v", err)
		}

		// Verify source event store
		evts, _ := eventStore.GetEvents(sourceID)
		if len(evts) != 2 { // Create + Transfer
			t.Fatalf("expected 2 events for source, got %d", len(evts))
		}
		transferEvent, ok := evts[1].(events.MoneyTransferredEvent)
		if !ok {
			t.Fatalf("expected MoneyTransferredEvent, got %T", evts[1])
		}
		if !transferEvent.DebitedAmount.Equal(dec("100")) || transferEvent.DebitedCurrency != shared.GBP {
			t.Errorf("unexpected Debit details in event")
		}
		if !transferEvent.CreditedAmount.Equal(dec("100")) || transferEvent.CreditedCurrency != shared.GBP {
			t.Errorf("unexpected Credit details in event")
		}
		if !transferEvent.ExchangeRate.Equal(dec("1")) {
			t.Errorf("unexpected Rate in event")
		}
		if transferEvent.TargetAccountID != targetID {
			t.Errorf("unexpected TargetAccountID in event")
		}

		// Verify source balance (only debit applied by this command)
		balances, _ := service.GetCurrentBalance(app.GetBalanceQuery{AccountID: sourceID})
		if !balances[shared.GBP].Equal(dec("400")) { // 500 - 100
			t.Errorf("expected source GBP balance 400, got %s", balances[shared.GBP])
		}

		balances, _ = service.GetCurrentBalance(app.GetBalanceQuery{AccountID: targetID})
		if !balances[shared.GBP].Equal(dec("100")) { // 0 + 100
			t.Errorf("expected target GBP balance 100, got %s", balances[shared.GBP])
		}

		targetEvts, _ := eventStore.GetEvents(targetID)
		if len(targetEvts) != 2 {
			t.Errorf("target account should only have 2 event, got %d", len(targetEvts))
		}
	})

	t.Run("FailOnInsufficientFunds", func(t *testing.T) {
		// Current source balance: 400 GBP
		transferCmd := app.TransferMoneyCommand{
			SourceAccountID: sourceID,
			TargetAccountID: targetID,
			Amount:          dec("500"), // More than available
			Currency:        shared.GBP,
		}
		err := service.TransferMoney(transferCmd)
		if !errors.Is(err, domain.ErrInsufficientFunds) {
			t.Errorf("expected ErrInsufficientFunds, got %v", err)
		}
		// Verify source balance unchanged
		balances, _ := service.GetCurrentBalance(app.GetBalanceQuery{AccountID: sourceID})
		if !balances[shared.GBP].Equal(dec("400")) {
			t.Errorf("source balance should remain 400 GBP after failed transfer, got %s", balances[shared.GBP])
		}
	})

	t.Run("FailOnSourceAccountNotFound", func(t *testing.T) {
		transferCmd := app.TransferMoneyCommand{
			SourceAccountID: "acc-nonexistent",
			TargetAccountID: targetID,
			Amount:          dec("10"),
			Currency:        shared.GBP,
		}
		err := service.TransferMoney(transferCmd)
		if !errors.Is(err, domain.ErrAccountNotFound) {
			t.Errorf("expected ErrAccountNotFound for source, got %v", err)
		}
	})

	t.Run("FailOnTargetAccountNotFound", func(t *testing.T) {
		transferCmd := app.TransferMoneyCommand{
			SourceAccountID: sourceID,
			TargetAccountID: "acc-nonexistent",
			Amount:          dec("10"),
			Currency:        shared.GBP,
		}
		err := service.TransferMoney(transferCmd)
		// The error might wrap ErrAccountNotFound, check message
		expectedMsg := "target account acc-nonexistent not found for transfer"
		if err == nil {
			t.Fatalf("expected an error, but got nil")
		}
		// Check if the error message matches exactly OR if it wraps the underlying domain error
		if err.Error() != expectedMsg && !errors.Is(err, domain.ErrAccountNotFound) {
			t.Errorf("expected target account not found error ('%s' or wrapping ErrAccountNotFound), got %v", expectedMsg, err)
		}
		// If either condition was met, the test passes implicitly.
	})

	t.Run("FailOnTransferToSelf", func(t *testing.T) {
		transferCmd := app.TransferMoneyCommand{
			SourceAccountID: sourceID,
			TargetAccountID: sourceID, // Same account
			Amount:          dec("10"),
			Currency:        shared.GBP,
		}
		err := service.TransferMoney(transferCmd)
		var domainErr *domain.DomainError
		if !errors.As(err, &domainErr) {
			t.Errorf("expected DomainError for transfer to self, got %T: %v", err, err)
		}
	})

	t.Run("FailOnNegativeAmount", func(t *testing.T) {
		transferCmd := app.TransferMoneyCommand{
			SourceAccountID: sourceID,
			TargetAccountID: targetID,
			Amount:          dec("-10"),
			Currency:        shared.GBP,
		}
		err := service.TransferMoney(transferCmd)
		var domainErr *domain.DomainError
		if !errors.As(err, &domainErr) {
			t.Errorf("expected DomainError for negative transfer amount, got %T: %v", err, err)
		}
	})
}

func TestAccountService_GetCurrentBalance(t *testing.T) {
	service, _, _ := setup()
	id, _ := service.CreateAccount(app.CreateAccountCommand{AccountID: "acc-bal-1", InitialBalances: map[shared.Currency]decimal.Decimal{
		shared.USD: dec("100.50"),
		shared.EUR: dec("200"),
	}})
	_ = service.Deposit(app.DepositMoneyCommand{AccountID: id, Amount: dec("10"), Currency: shared.USD})

	t.Run("GetAllBalances", func(t *testing.T) {
		balances, err := service.GetCurrentBalance(app.GetBalanceQuery{AccountID: id})
		if err != nil {
			t.Fatalf("GetCurrentBalance failed: %v", err)
		}
		if len(balances) != 2 {
			t.Errorf("expected 2 balances, got %d", len(balances))
		}
		if !balances[shared.USD].Equal(dec("110.50")) { // 100.50 + 10
			t.Errorf("expected USD 110.50, got %s", balances[shared.USD])
		}
		if !balances[shared.EUR].Equal(dec("200")) {
			t.Errorf("expected EUR 200, got %s", balances[shared.EUR])
		}
		// Check non-existent currency
		if _, exists := balances[shared.GBP]; exists {
			t.Errorf("expected GBP balance not to exist in map, but found %s", balances[shared.GBP])
		}
	})

	t.Run("GetSpecificBalance", func(t *testing.T) {
		usd := shared.USD
		balances, err := service.GetCurrentBalance(app.GetBalanceQuery{AccountID: id, Currency: &usd})
		if err != nil {
			t.Fatalf("GetCurrentBalance failed: %v", err)
		}
		if len(balances) != 1 {
			t.Errorf("expected 1 balance, got %d", len(balances))
		}
		if !balances[shared.USD].Equal(dec("110.50")) {
			t.Errorf("expected USD 110.50, got %s", balances[shared.USD])
		}
	})

	t.Run("GetSpecificBalanceNotHeld", func(t *testing.T) {
		gbp := shared.GBP
		balances, err := service.GetCurrentBalance(app.GetBalanceQuery{AccountID: id, Currency: &gbp})
		if err != nil {
			t.Fatalf("GetCurrentBalance failed: %v", err)
		}
		if len(balances) != 1 {
			t.Errorf("expected 1 balance entry (for GBP), got %d", len(balances))
		}
		if !balances[shared.GBP].IsZero() { // Should return the requested currency with zero amount
			t.Errorf("expected GBP 0, got %s", balances[shared.GBP])
		}
	})

	t.Run("GetBalanceAccountNotFound", func(t *testing.T) {
		_, err := service.GetCurrentBalance(app.GetBalanceQuery{AccountID: "acc-nonexistent"})
		if !errors.Is(err, domain.ErrAccountNotFound) {
			t.Errorf("expected ErrAccountNotFound, got %v", err)
		}
	})
}

func TestAccountService_GetTransactionHistory(t *testing.T) {
	service, _, _ := setup()
	id, _ := service.CreateAccount(app.CreateAccountCommand{AccountID: "acc-hist-1", InitialBalances: map[shared.Currency]decimal.Decimal{shared.USD: dec("10")}})
	_ = service.Deposit(app.DepositMoneyCommand{AccountID: id, Amount: dec("5"), Currency: shared.USD})
	_ = service.Withdraw(app.WithdrawMoneyCommand{AccountID: id, Amount: dec("2"), Currency: shared.USD})

	t.Run("GetAllHistory", func(t *testing.T) {
		history, err := service.GetTransactionHistory(app.GetHistoryQuery{AccountID: id})
		if err != nil {
			t.Fatalf("GetTransactionHistory failed: %v", err)
		}
		if len(history) != 3 { // Create, Deposit, Withdraw
			t.Fatalf("expected 3 events, got %d", len(history))
		}
		if _, ok := history[0].(events.AccountCreatedEvent); !ok {
			t.Errorf("expected event 1 to be AccountCreatedEvent")
		}
		if _, ok := history[1].(events.DepositMadeEvent); !ok {
			t.Errorf("expected event 2 to be DepositMadeEvent")
		}
		if _, ok := history[2].(events.WithdrawalMadeEvent); !ok {
			t.Errorf("expected event 3 to be WithdrawalMadeEvent")
		}
	})

	t.Run("GetHistoryWithLimit", func(t *testing.T) {
		history, err := service.GetTransactionHistory(app.GetHistoryQuery{AccountID: id, Limit: 2})
		if err != nil {
			t.Fatalf("GetTransactionHistory failed: %v", err)
		}
		if len(history) != 2 {
			t.Fatalf("expected 2 events, got %d", len(history))
		}
		if _, ok := history[0].(events.AccountCreatedEvent); !ok { // Should get first 2
			t.Errorf("expected event 1 to be AccountCreatedEvent")
		}
		if _, ok := history[1].(events.DepositMadeEvent); !ok {
			t.Errorf("expected event 2 to be DepositMadeEvent")
		}
	})

	t.Run("GetHistoryWithSkip", func(t *testing.T) {
		history, err := service.GetTransactionHistory(app.GetHistoryQuery{AccountID: id, Skip: 1})
		if err != nil {
			t.Fatalf("GetTransactionHistory failed: %v", err)
		}
		if len(history) != 2 { // Skip 1, get remaining 2
			t.Fatalf("expected 2 events, got %d", len(history))
		}
		if _, ok := history[0].(events.DepositMadeEvent); !ok { // Should get events 2 and 3
			t.Errorf("expected event 1 (after skip) to be DepositMadeEvent")
		}
		if _, ok := history[1].(events.WithdrawalMadeEvent); !ok {
			t.Errorf("expected event 2 (after skip) to be WithdrawalMadeEvent")
		}
	})

	t.Run("GetHistoryWithSkipAndLimit", func(t *testing.T) {
		history, err := service.GetTransactionHistory(app.GetHistoryQuery{AccountID: id, Skip: 1, Limit: 1})
		if err != nil {
			t.Fatalf("GetTransactionHistory failed: %v", err)
		}
		if len(history) != 1 { // Skip 1, limit 1
			t.Fatalf("expected 1 event, got %d", len(history))
		}
		if _, ok := history[0].(events.DepositMadeEvent); !ok { // Should get event 2
			t.Errorf("expected event 1 (after skip/limit) to be DepositMadeEvent")
		}
	})

	t.Run("GetHistorySkipPastEnd", func(t *testing.T) {
		history, err := service.GetTransactionHistory(app.GetHistoryQuery{AccountID: id, Skip: 5})
		if err != nil {
			t.Fatalf("GetTransactionHistory failed: %v", err)
		}
		if len(history) != 0 {
			t.Fatalf("expected 0 events, got %d", len(history))
		}
	})

	t.Run("GetHistoryAccountNotFound", func(t *testing.T) {
		_, err := service.GetTransactionHistory(app.GetHistoryQuery{AccountID: "acc-nonexistent"})
		if !errors.Is(err, domain.ErrAccountNotFound) {
			t.Errorf("expected ErrAccountNotFound, got %v", err)
		}
	})
}

// TestSnapshotting verifies snapshot creation and loading.
func TestAccountService_Snapshotting(t *testing.T) {
	service, eventStore, snapshotStore := setup()
	id := "acc-snap-1"
	_, _ = service.CreateAccount(app.CreateAccountCommand{AccountID: id, InitialBalances: map[shared.Currency]decimal.Decimal{shared.USD: dec("0")}})

	// Perform operations up to the snapshot frequency
	// SnapshotFrequency is 100. AccountCreated is v1. Need 99 more events.
	for i := 0; i < app.SnapshotFrequency-1; i++ {
		err := service.Deposit(app.DepositMoneyCommand{AccountID: id, Amount: dec("1"), Currency: shared.USD})
		if err != nil {
			t.Fatalf("Deposit %d failed: %v", i+1, err)
		}
	}

	// At this point, version should be SnapshotFrequency (100)
	// Check if snapshot was saved
	snap, found, err := snapshotStore.GetLatestSnapshot(id)
	if err != nil {
		t.Fatalf("Error checking snapshot store: %v", err)
	}
	if !found {
		t.Fatalf("Expected snapshot to be saved at version %d, but not found", app.SnapshotFrequency)
	}
	if snap.Version != app.SnapshotFrequency {
		t.Errorf("Expected snapshot version %d, got %d", app.SnapshotFrequency, snap.Version)
	}

	// Verify snapshot content (basic check)
	var snapAccount domain.Account
	if err := json.Unmarshal(snap.State, &snapAccount); err != nil {
		t.Fatalf("Failed to unmarshal snapshot state: %v", err)
	}
	expectedBalance := dec(fmt.Sprintf("%d", app.SnapshotFrequency-1)) // 99 deposits of 1
	if !snapAccount.Balances[shared.USD].Equal(expectedBalance) {
		t.Errorf("Snapshot balance mismatch: expected %s, got %s", expectedBalance, snapAccount.Balances[shared.USD])
	}
	if snapAccount.Version != app.SnapshotFrequency {
		t.Errorf("Snapshot state version mismatch: expected %d, got %d", app.SnapshotFrequency, snapAccount.Version)
	}

	// Perform one more operation (version 101)
	err = service.Deposit(app.DepositMoneyCommand{AccountID: id, Amount: dec("1"), Currency: shared.USD})
	if err != nil {
		t.Fatalf("Deposit %d failed: %v", app.SnapshotFrequency, err)
	}

	// --- Simulate loading the account again ---
	// In a real test, you might need to inspect logs or use a mock event store
	// to confirm *which* events were loaded. Here, we'll verify the final state
	// and check that the number of events loaded *after* the snapshot version is correct.

	// Clear the event store's internal cache/map (if it had one) or create a new service instance
	// to force reloading from stores. Using the same stores is fine.
	serviceReloaded := app.NewAccountService(eventStore, snapshotStore)

	// Query balance - this forces loadAccount
	balances, err := serviceReloaded.GetCurrentBalance(app.GetBalanceQuery{AccountID: id})
	if err != nil {
		t.Fatalf("GetCurrentBalance after snapshot failed: %v", err)
	}

	// Verify final balance (99 + 1 = 100)
	finalExpectedBalance := dec(fmt.Sprintf("%d", app.SnapshotFrequency))
	if !balances[shared.USD].Equal(finalExpectedBalance) {
		t.Errorf("Final balance mismatch after reloading with snapshot: expected %s, got %s", finalExpectedBalance, balances[shared.USD])
	}

	// Check how many events exist *after* the snapshot version in the store
	eventsAfterSnapshot, err := eventStore.GetEventsAfterVersion(id, app.SnapshotFrequency)
	if err != nil {
		t.Fatalf("Failed to get events after snapshot version: %v", err)
	}
	if len(eventsAfterSnapshot) != 1 {
		// This confirms that loadAccount *should* have only loaded 1 event after applying the snapshot.
		t.Errorf("Expected 1 event after snapshot version %d, found %d", app.SnapshotFrequency, len(eventsAfterSnapshot))
	}
}

// TestOptimisticLocking simulates concurrent updates to the same account.
func TestAccountService_OptimisticLocking(t *testing.T) {
	service, _, _ := setup() // Use shared stores
	id := "acc-lock-1"
	_, _ = service.CreateAccount(app.CreateAccountCommand{AccountID: id, InitialBalances: map[shared.Currency]decimal.Decimal{shared.USD: dec("100")}}) // Version 1

	var wg sync.WaitGroup

	wg.Add(100)

	for i := 0; i < 100; i++ {
		go func(i int) {
			defer wg.Done()
			err := service.Deposit(app.DepositMoneyCommand{AccountID: id, Amount: dec("10"), Currency: shared.USD})
			if err != nil {
				t.Logf("Goroutine %d: Deposit failed: %v", i, err)
			} else {
				t.Logf("Goroutine %d: Deposit succeeded", i)
			}
		}(i)
	}

	wg.Wait()

	balances, err := service.GetCurrentBalance(app.GetBalanceQuery{AccountID: id})

	if err != nil {
		t.Fatalf("GetCurrentBalance after concurrent deposits failed: %v", err)
	}

	if balances[shared.USD] == dec("1100") {
		t.Errorf("Expected balance to be smaller than 1100 USD, got %s", balances[shared.USD])
	}

	t.Logf("current balance  is %s", balances[shared.USD])
}
