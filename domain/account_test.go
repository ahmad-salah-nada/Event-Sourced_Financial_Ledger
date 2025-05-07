package domain_test

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"financial-ledger/domain"
	"financial-ledger/events"
	"financial-ledger/shared"
)

// Helper to create decimals in tests, panics on error
func dec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

// Helper to check event type and count
func assertEvent[E events.Event](t *testing.T, changes []events.Event) E {
	t.Helper()
	if len(changes) != 1 {
		t.Fatalf("expected 1 event, got %d", len(changes))
	}
	event, ok := changes[0].(E)
	if !ok {
		t.Fatalf("expected event type %T, got %T", *new(E), changes[0])
	}
	return event
}

func TestAccount_NewAccount(t *testing.T) {
	acc := domain.NewAccount("acc-123")
	if acc.ID != "acc-123" {
		t.Errorf("expected ID 'acc-123', got '%s'", acc.ID)
	}
	if acc.Version != 0 {
		t.Errorf("expected Version 0, got %d", acc.Version)
	}
	if len(acc.Balances) != 0 {
		t.Errorf("expected empty Balances, got %v", acc.Balances)
	}
	if len(acc.GetUncommitedChanges()) != 0 {
		t.Errorf("expected no initial changes")
	}
}

func TestAccount_HandleCreateAccount(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		acc := domain.NewAccount("") // ID set by handler
		initial := map[shared.Currency]decimal.Decimal{
			shared.USD: dec("100.50"),
			shared.EUR: dec("50"),
		}
		err := acc.HandleCreateAccount("acc-1", initial)
		if err != nil {
			t.Fatalf("HandleCreateAccount failed: %v", err)
		}

		changes := acc.GetUncommitedChanges()
		event := assertEvent[events.AccountCreatedEvent](t, changes)

		if event.AggregateID != "acc-1" {
			t.Errorf("event AggregateID mismatch: expected 'acc-1', got '%s'", event.AggregateID)
		}
		if event.Version != 1 {
			t.Errorf("event Version mismatch: expected 1, got %d", event.Version)
		}
		if len(event.InitialBalances) != 2 {
			t.Errorf("expected 2 initial balances, got %d", len(event.InitialBalances))
		}
		// Check internal state after apply
		if acc.ID != "acc-1" {
			t.Errorf("account ID mismatch after apply: expected 'acc-1', got '%s'", acc.ID)
		}
		if acc.Version != 1 {
			t.Errorf("account Version mismatch after apply: expected 1, got %d", acc.Version)
		}
		if !acc.Balances[shared.USD].Equal(dec("100.50")) {
			t.Errorf("USD balance mismatch: expected 100.50, got %s", acc.Balances[shared.USD])
		}
		if !acc.Balances[shared.EUR].Equal(dec("50")) {
			t.Errorf("EUR balance mismatch: expected 50, got %s", acc.Balances[shared.EUR])
		}
	})

	t.Run("FailOnAlreadyCreated", func(t *testing.T) {
		acc := domain.NewAccount("acc-1")
		// Simulate already created by applying an event
		_ = acc.ApplyEvent(events.AccountCreatedEvent{
			BaseEvent: events.NewBaseEvent("acc-1", 1, events.AccountCreatedType),
		})
		acc.GetUncommitedChanges() // Clear changes from Apply

		err := acc.HandleCreateAccount("acc-1", nil)
		if !errors.Is(err, domain.ErrAccountExists) {
			t.Errorf("expected ErrAccountExists, got %v", err)
		}
	})

	t.Run("FailOnNegativeInitialBalance", func(t *testing.T) {
		acc := domain.NewAccount("")
		initial := map[shared.Currency]decimal.Decimal{
			shared.USD: dec("-100"),
		}
		err := acc.HandleCreateAccount("acc-1", initial)
		var domainErr *domain.DomainError
		if !errors.As(err, &domainErr) {
			t.Errorf("expected DomainError, got %T: %v", err, err)
		}
	})
}

func TestAccount_HandleDeposit(t *testing.T) {
	acc := domain.NewAccount("acc-1")
	_ = acc.ApplyEvent(events.AccountCreatedEvent{ // Apply initial state
		BaseEvent: events.NewBaseEvent("acc-1", 1, events.AccountCreatedType),
		InitialBalances: []shared.Balance{
			{Currency: shared.USD, Amount: dec("100")},
		},
	})
	acc.GetUncommitedChanges() // Clear create event

	t.Run("Success", func(t *testing.T) {
		err := acc.HandleDeposit(dec("50.25"), shared.USD)
		if err != nil {
			t.Fatalf("HandleDeposit failed: %v", err)
		}
		changes := acc.GetUncommitedChanges()
		event := assertEvent[events.DepositMadeEvent](t, changes)

		if event.Version != 2 {
			t.Errorf("expected version 2, got %d", event.Version)
		}
		if !event.Amount.Equal(dec("50.25")) {
			t.Errorf("expected amount 50.25, got %s", event.Amount)
		}
		if event.Currency != shared.USD {
			t.Errorf("expected currency USD, got %s", event.Currency)
		}
		// Check internal state
		if !acc.Balances[shared.USD].Equal(dec("150.25")) {
			t.Errorf("expected final balance 150.25, got %s", acc.Balances[shared.USD])
		}
		if acc.Version != 2 {
			t.Errorf("expected final version 2, got %d", acc.Version)
		}
	})

	t.Run("FailOnNegativeAmount", func(t *testing.T) {
		// Reset state for this subtest if needed, or use a new account
		// For simplicity, continue with acc state (v2, 150.25 USD)
		err := acc.HandleDeposit(dec("-10"), shared.USD)
		var domainErr *domain.DomainError
		if !errors.As(err, &domainErr) {
			t.Errorf("expected DomainError, got %T: %v", err, err)
		}
		if len(acc.GetUncommitedChanges()) != 0 {
			t.Errorf("should not have generated events on error")
		}
	})

	t.Run("FailOnUninitializedAccount", func(t *testing.T) {
		uninitializedAcc := domain.NewAccount("acc-new")
		err := uninitializedAcc.HandleDeposit(dec("10"), shared.USD)
		var domainErr *domain.DomainError
		if !errors.As(err, &domainErr) {
			t.Errorf("expected DomainError, got %T: %v", err, err)
		}
	})
}

func TestAccount_HandleWithdraw(t *testing.T) {
	acc := domain.NewAccount("acc-1")
	_ = acc.ApplyEvent(events.AccountCreatedEvent{ // Apply initial state
		BaseEvent: events.NewBaseEvent("acc-1", 1, events.AccountCreatedType),
		InitialBalances: []shared.Balance{
			{Currency: shared.USD, Amount: dec("100")},
		},
	})
	acc.GetUncommitedChanges() // Clear create event

	t.Run("Success", func(t *testing.T) {
		err := acc.HandleWithdraw(dec("30.50"), shared.USD)
		if err != nil {
			t.Fatalf("HandleWithdraw failed: %v", err)
		}
		changes := acc.GetUncommitedChanges()
		event := assertEvent[events.WithdrawalMadeEvent](t, changes)

		if event.Version != 2 {
			t.Errorf("expected version 2, got %d", event.Version)
		}
		if !event.Amount.Equal(dec("30.50")) {
			t.Errorf("expected amount 30.50, got %s", event.Amount)
		}
		if event.Currency != shared.USD {
			t.Errorf("expected currency USD, got %s", event.Currency)
		}
		// Check internal state
		if !acc.Balances[shared.USD].Equal(dec("69.50")) {
			t.Errorf("expected final balance 69.50, got %s", acc.Balances[shared.USD])
		}
		if acc.Version != 2 {
			t.Errorf("expected final version 2, got %d", acc.Version)
		}
	})

	t.Run("FailOnInsufficientFunds", func(t *testing.T) {
		// Current state: v2, 69.50 USD
		err := acc.HandleWithdraw(dec("70"), shared.USD)
		if !errors.Is(err, domain.ErrInsufficientFunds) {
			t.Errorf("expected ErrInsufficientFunds, got %v", err)
		}
		if len(acc.GetUncommitedChanges()) != 0 {
			t.Errorf("should not have generated events on error")
		}
		// State should be unchanged
		if !acc.Balances[shared.USD].Equal(dec("69.50")) {
			t.Errorf("balance should not change on error, expected 69.50, got %s", acc.Balances[shared.USD])
		}
		if acc.Version != 2 {
			t.Errorf("version should not change on error, expected 2, got %d", acc.Version)
		}
	})

	t.Run("FailOnNegativeAmount", func(t *testing.T) {
		err := acc.HandleWithdraw(dec("-10"), shared.USD)
		var domainErr *domain.DomainError
		if !errors.As(err, &domainErr) {
			t.Errorf("expected DomainError, got %T: %v", err, err)
		}
	})
}

func TestAccount_HandleConvertCurrency(t *testing.T) {
	acc := domain.NewAccount("acc-1")
	_ = acc.ApplyEvent(events.AccountCreatedEvent{ // Apply initial state
		BaseEvent: events.NewBaseEvent("acc-1", 1, events.AccountCreatedType),
		InitialBalances: []shared.Balance{
			{Currency: shared.USD, Amount: dec("100")},
			{Currency: shared.EUR, Amount: dec("50")},
		},
	})
	acc.GetUncommitedChanges() // Clear create event

	t.Run("Success", func(t *testing.T) {
		rate := dec("0.9") // 1 USD = 0.9 EUR
		err := acc.HandleConvertCurrency(dec("50"), shared.USD, shared.EUR, rate)
		if err != nil {
			t.Fatalf("HandleConvertCurrency failed: %v", err)
		}
		changes := acc.GetUncommitedChanges()
		event := assertEvent[events.CurrencyConvertedEvent](t, changes)

		if event.Version != 2 {
			t.Errorf("expected version 2, got %d", event.Version)
		}
		if !event.FromAmount.Equal(dec("50")) || event.FromCurrency != shared.USD {
			t.Errorf("unexpected From: %s %s", event.FromAmount, event.FromCurrency)
		}
		expectedToAmount := dec("50").Mul(rate) // 45
		if !event.ToAmount.Equal(expectedToAmount) || event.ToCurrency != shared.EUR {
			t.Errorf("unexpected To: %s %s (expected %s)", event.ToAmount, event.ToCurrency, expectedToAmount)
		}
		if !event.ExchangeRate.Equal(rate) {
			t.Errorf("unexpected Rate: %s", event.ExchangeRate)
		}

		// Check internal state
		if !acc.Balances[shared.USD].Equal(dec("50")) { // 100 - 50
			t.Errorf("expected final USD balance 50, got %s", acc.Balances[shared.USD])
		}
		if !acc.Balances[shared.EUR].Equal(dec("95")) { // 50 + 45
			t.Errorf("expected final EUR balance 95, got %s", acc.Balances[shared.EUR])
		}
		if acc.Version != 2 {
			t.Errorf("expected final version 2, got %d", acc.Version)
		}
	})

	t.Run("FailOnInsufficientFunds", func(t *testing.T) {
		// Current state: v2, 50 USD, 95 EUR
		rate := dec("0.9")
		err := acc.HandleConvertCurrency(dec("60"), shared.USD, shared.EUR, rate)
		if !errors.Is(err, domain.ErrInsufficientFunds) {
			t.Errorf("expected ErrInsufficientFunds, got %v", err)
		}
	})

	t.Run("FailOnSameCurrency", func(t *testing.T) {
		rate := dec("1.0")
		err := acc.HandleConvertCurrency(dec("10"), shared.USD, shared.USD, rate)
		var domainErr *domain.DomainError
		if !errors.As(err, &domainErr) {
			t.Errorf("expected DomainError, got %T: %v", err, err)
		}
	})
	// Add tests for negative amount, negative rate etc.
}

func TestAccount_HandleInitiateTransfer(t *testing.T) {
	acc := domain.NewAccount("acc-source")
	_ = acc.ApplyEvent(events.AccountCreatedEvent{ // Apply initial state
		BaseEvent: events.NewBaseEvent("acc-source", 1, events.AccountCreatedType),
		InitialBalances: []shared.Balance{
			{Currency: shared.USD, Amount: dec("200")},
			{Currency: shared.GBP, Amount: dec("100")},
		},
	})
	acc.GetUncommitedChanges() // Clear create event

	transferID := "transfer-123"

	t.Run("SuccessSameCurrency", func(t *testing.T) {
		debit := dec("50")
		credit := dec("50")
		rate := dec("1")
		err := acc.HandleInitiateTransfer(transferID, "acc-target", debit, shared.USD, credit, shared.USD, rate)
		if err != nil {
			t.Fatalf("HandleInitiateTransfer failed: %v", err)
		}
		changes := acc.GetUncommitedChanges()
		event := assertEvent[events.MoneyTransferredEvent](t, changes)

		if event.Version != 2 {
			t.Errorf("expected version 2, got %d", event.Version)
		}
		if event.TransferID != transferID {
			t.Errorf("expected transferID %s, got %s", transferID, event.TransferID)
		}
		if event.SourceAccountID != acc.ID {
			t.Errorf("expected sourceAccountID %s, got %s", acc.ID, event.SourceAccountID)
		}
		if !event.DebitedAmount.Equal(debit) || event.DebitedCurrency != shared.USD {
			t.Errorf("unexpected Debit: %s %s", event.DebitedAmount, event.DebitedCurrency)
		}
		if !event.CreditedAmount.Equal(credit) || event.CreditedCurrency != shared.USD {
			t.Errorf("unexpected Credit: %s %s", event.CreditedAmount, event.CreditedCurrency)
		}
		if !event.ExchangeRate.Equal(rate) {
			t.Errorf("unexpected Rate: %s", event.ExchangeRate)
		}
		if event.TargetAccountID != "acc-target" {
			t.Errorf("unexpected TargetAccountID: %s", event.TargetAccountID)
		}

		// Check internal state (debit applied to source)
		if !acc.Balances[shared.USD].Equal(dec("150")) { // 200 - 50
			t.Errorf("expected final USD balance 150, got %s", acc.Balances[shared.USD])
		}
		if acc.Version != 2 {
			t.Errorf("expected final version 2, got %d", acc.Version)
		}
	})

	t.Run("SuccessCrossCurrency", func(t *testing.T) {
		// Current state: v2, 150 USD, 100 GBP
		debit := dec("80")        // Debit 80 GBP
		rate := dec("1.25")       // 1 GBP = 1.25 USD
		credit := debit.Mul(rate) // Expected credit 100 USD
		err := acc.HandleInitiateTransfer(transferID+"-2", "acc-target-2", debit, shared.GBP, credit, shared.USD, rate)
		if err != nil {
			t.Fatalf("HandleInitiateTransfer failed: %v", err)
		}
		changes := acc.GetUncommitedChanges()
		event := assertEvent[events.MoneyTransferredEvent](t, changes)

		if event.Version != 3 {
			t.Errorf("expected version 3, got %d", event.Version)
		}
		if event.SourceAccountID != acc.ID {
			t.Errorf("expected sourceAccountID %s, got %s", acc.ID, event.SourceAccountID)
		}
		if !event.DebitedAmount.Equal(debit) || event.DebitedCurrency != shared.GBP {
			t.Errorf("unexpected Debit: %s %s", event.DebitedAmount, event.DebitedCurrency)
		}
		if !event.CreditedAmount.Equal(credit) || event.CreditedCurrency != shared.USD {
			t.Errorf("unexpected Credit: %s %s (expected %s)", event.CreditedAmount, event.CreditedCurrency, credit)
		}
		if !event.ExchangeRate.Equal(rate) {
			t.Errorf("unexpected Rate: %s", event.ExchangeRate)
		}

		// Check internal state (debit applied to source)
		if !acc.Balances[shared.GBP].Equal(dec("20")) { // 100 - 80
			t.Errorf("expected final GBP balance 20, got %s", acc.Balances[shared.GBP])
		}
		if acc.Version != 3 {
			t.Errorf("expected final version 3, got %d", acc.Version)
		}
	})

	t.Run("FailOnInsufficientFunds", func(t *testing.T) {
		// Current state: v3, 150 USD, 20 GBP
		err := acc.HandleInitiateTransfer(transferID+"-3", "acc-target", dec("30"), shared.GBP, dec("30"), shared.GBP, dec("1"))
		if !errors.Is(err, domain.ErrInsufficientFunds) {
			t.Errorf("expected ErrInsufficientFunds, got %v", err)
		}
	})

	t.Run("FailOnTransferToSelf", func(t *testing.T) {
		err := acc.HandleInitiateTransfer(transferID+"-4", "acc-source", dec("10"), shared.USD, dec("10"), shared.USD, dec("1"))
		var domainErr *domain.DomainError
		if !errors.As(err, &domainErr) {
			t.Errorf("expected DomainError, got %T: %v", err, err)
		}
	})
	// Add tests for negative amount, invalid rate for cross-currency, mismatched credit amount etc.
}

func TestAccount_HandleReceiveTransfer(t *testing.T) {
	accTarget := domain.NewAccount("acc-target")
	_ = accTarget.ApplyEvent(events.AccountCreatedEvent{ // Apply initial state
		BaseEvent: events.NewBaseEvent("acc-target", 1, events.AccountCreatedType),
		InitialBalances: []shared.Balance{
			{Currency: shared.USD, Amount: dec("50")},
		},
	})
	accTarget.GetUncommitedChanges() // Clear create event

	transferID := "transfer-456"
	sourceAccountID := "acc-source"

	t.Run("Success", func(t *testing.T) {
		debitedAmt := dec("100")
		debitedCur := shared.USD
		creditedAmt := dec("100")
		creditedCur := shared.USD
		exRate := dec("1")

		err := accTarget.HandleReceiveTransfer(transferID, sourceAccountID, accTarget.ID, debitedAmt, debitedCur, creditedAmt, creditedCur, exRate)
		if err != nil {
			t.Fatalf("HandleReceiveTransfer failed: %v", err)
		}
		changes := accTarget.GetUncommitedChanges()
		event := assertEvent[events.MoneyTransferredEvent](t, changes)

		if event.Version != 2 {
			t.Errorf("expected version 2, got %d", event.Version)
		}
		if event.TransferID != transferID {
			t.Errorf("expected transferID %s, got %s", transferID, event.TransferID)
		}
		if event.SourceAccountID != sourceAccountID {
			t.Errorf("expected sourceAccountID %s, got %s", sourceAccountID, event.SourceAccountID)
		}
		if event.TargetAccountID != accTarget.ID {
			t.Errorf("expected targetAccountID %s, got %s", accTarget.ID, event.TargetAccountID)
		}
		if !event.CreditedAmount.Equal(creditedAmt) || event.CreditedCurrency != creditedCur {
			t.Errorf("unexpected Credit: %s %s", event.CreditedAmount, event.CreditedCurrency)
		}
		if !event.DebitedAmount.Equal(debitedAmt) || event.DebitedCurrency != debitedCur {
			t.Errorf("unexpected original Debit info: %s %s", event.DebitedAmount, event.DebitedCurrency)
		}
		if !event.ExchangeRate.Equal(exRate) {
			t.Errorf("unexpected original Rate info: %s", event.ExchangeRate)
		}

		if !accTarget.Balances[shared.USD].Equal(dec("150")) {
			t.Errorf("expected final USD balance 150, got %s", accTarget.Balances[shared.USD])
		}
		if accTarget.Version != 2 {
			t.Errorf("expected final version 2, got %d", accTarget.Version)
		}
	})

	t.Run("FailIfTargetIDMismatch", func(t *testing.T) {
		err := accTarget.HandleReceiveTransfer(transferID, sourceAccountID, "some-other-target", dec("10"), shared.USD, dec("10"), shared.USD, dec("1"))
		var domainErr *domain.DomainError
		if !errors.As(err, &domainErr) {
			t.Errorf("expected DomainError for target ID mismatch, got %T: %v", err, err)
		}
	})

	t.Run("FailOnNegativeCreditAmount", func(t *testing.T) {
		err := accTarget.HandleReceiveTransfer(transferID, sourceAccountID, accTarget.ID, dec("10"), shared.USD, dec("-10"), shared.USD, dec("1"))
		var domainErr *domain.DomainError
		if !errors.As(err, &domainErr) {
			t.Errorf("expected DomainError for negative credit, got %T: %v", err, err)
		}
	})
}

func TestAccount_ApplyEvents(t *testing.T) {
	acc := domain.NewAccount("acc-apply")
	transferID := "tf-apply-123"
	sourceID := "acc-apply" // This account will be the source
	targetID := "acc-other-target"

	history := []events.Event{
		events.AccountCreatedEvent{
			BaseEvent: events.NewBaseEvent(sourceID, 1, events.AccountCreatedType),
			InitialBalances: []shared.Balance{
				{Currency: shared.USD, Amount: dec("100")},
				{Currency: shared.EUR, Amount: dec("200")},
			},
		},
		events.DepositMadeEvent{
			BaseEvent: events.NewBaseEvent(sourceID, 2, events.DepositMadeType),
			Amount:    dec("50"),
			Currency:  shared.USD, // USD: 150
		},
		events.WithdrawalMadeEvent{
			BaseEvent: events.NewBaseEvent(sourceID, 3, events.WithdrawalMadeType),
			Amount:    dec("20"),
			Currency:  shared.USD, // USD: 130
		},
		// This account (acc-apply/sourceID) initiates a transfer (debit)
		events.MoneyTransferredEvent{
			BaseEvent:        events.NewBaseEvent(sourceID, 4, events.MoneyTransferredType),
			TransferID:       transferID,
			SourceAccountID:  sourceID,
			TargetAccountID:  targetID,
			DebitedAmount:    dec("30"),
			DebitedCurrency:  shared.USD, // USD: 100
			CreditedAmount:   dec("25"),  // Assume 30 USD became 25 EUR
			CreditedCurrency: shared.EUR,
			ExchangeRate:     dec("0.83333333"), // Placeholder
		},
	}

	err := acc.ApplyEvents(history)
	if err != nil {
		t.Fatalf("ApplyEvents failed for source account: %v", err)
	}

	if acc.ID != sourceID {
		t.Errorf("account ID mismatch: expected %s, got %s", sourceID, acc.ID)
	}
	if acc.Version != 4 {
		t.Errorf("expected final version 4 for source, got %d", acc.Version)
	}
	if !acc.Balances[shared.USD].Equal(dec("100")) { // 100 + 50 - 20 - 30
		t.Errorf("expected final USD balance 100 for source, got %s", acc.Balances[shared.USD])
	}
	if !acc.Balances[shared.EUR].Equal(dec("200")) { // EUR unchanged for source
		t.Errorf("expected final EUR balance 200 for source, got %s", acc.Balances[shared.EUR])
	}
	if len(acc.GetUncommitedChanges()) != 0 {
		t.Errorf("ApplyEvents should not add events to changes")
	}

	// Test applying MoneyTransferredEvent to a target account
	accTarget := domain.NewAccount(targetID)
	targetHistory := []events.Event{
		events.AccountCreatedEvent{
			BaseEvent: events.NewBaseEvent(targetID, 1, events.AccountCreatedType),
			InitialBalances: []shared.Balance{
				{Currency: shared.EUR, Amount: dec("50")},
			},
		},
		// This account (targetID) receives the transfer (credit)
		events.MoneyTransferredEvent{
			BaseEvent:        events.NewBaseEvent(targetID, 2, events.MoneyTransferredType),
			TransferID:       transferID,
			SourceAccountID:  sourceID,  // Original source
			TargetAccountID:  targetID,  // This account is the target
			DebitedAmount:    dec("30"), // Informational
			DebitedCurrency:  shared.USD,
			CreditedAmount:   dec("25"),  // Amount credited to this target
			CreditedCurrency: shared.EUR, // EUR: 50 + 25 = 75
			ExchangeRate:     dec("0.83333333"),
		},
	}
	err = accTarget.ApplyEvents(targetHistory)
	if err != nil {
		t.Fatalf("ApplyEvents failed for target account: %v", err)
	}
	if accTarget.ID != targetID {
		t.Errorf("target account ID mismatch: expected %s, got %s", targetID, accTarget.ID)
	}
	if accTarget.Version != 2 {
		t.Errorf("expected final version 2 for target, got %d", accTarget.Version)
	}
	if !accTarget.Balances[shared.EUR].Equal(dec("75")) { // 50 + 25
		t.Errorf("expected final EUR balance 75 for target, got %s", accTarget.Balances[shared.EUR])
	}
}

func TestAccount_Apply_VersionMismatch(t *testing.T) {
	acc := domain.NewAccount("acc-ver")
	_ = acc.ApplyEvent(events.AccountCreatedEvent{ // Apply v1
		BaseEvent: events.NewBaseEvent("acc-ver", 1, events.AccountCreatedType),
	})

	// Try applying v3 when v2 is expected
	err := acc.ApplyEvent(events.DepositMadeEvent{
		BaseEvent: events.NewBaseEvent("acc-ver", 3, events.DepositMadeType),
		Amount:    dec("10"),
		Currency:  shared.USD,
	})
	if err == nil {
		t.Fatalf("expected version mismatch error, got nil")
	}
	if acc.Version != 1 {
		t.Errorf("version should remain 1 after failed apply, got %d", acc.Version)
	}
}

func TestAccount_GetUncommitedChanges(t *testing.T) {
	acc := domain.NewAccount("acc-changes")
	_ = acc.HandleCreateAccount("acc-changes", nil)
	changes1 := acc.GetUncommitedChanges()
	if len(changes1) != 1 {
		t.Fatalf("expected 1 change after create, got %d", len(changes1))
	}
	changes2 := acc.GetUncommitedChanges()
	if len(changes2) != 0 {
		t.Fatalf("expected 0 changes after GetUncommitedChanges called, got %d", len(changes2))
	}
}
