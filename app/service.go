package app

import (
	"errors"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"financial-ledger/domain"
	"financial-ledger/events"
	"financial-ledger/shared"
	"financial-ledger/store"
)

const (
	SnapshotFrequency = 100
)

// AccountService acts as the application layer, orchestrating the interaction
// between incoming commands/queries, the domain aggregates (Account), and the
// persistence layers (EventStore, SnapshotStore).
type AccountService struct {
	eventStore    store.EventStore
	snapshotStore store.SnapshotStore
}

func NewAccountService(es store.EventStore, ss store.SnapshotStore) *AccountService {
	if es == nil || ss == nil {
		log.Fatal("FATAL: EventStore and SnapshotStore must not be nil")
	}
	return &AccountService{
		eventStore:    es,
		snapshotStore: ss,
	}
}

// --- Command Handlers ---
// These methods process incoming commands. They typically involve:
// 1. Loading the relevant aggregate state (using loadAccount).
// 2. Executing the command on the aggregate instance.
// 3. Persisting the resulting events (using EventStore).
// 4. Optionally saving a snapshot (using SnapshotStore).

func (s *AccountService) CreateAccount(cmd CreateAccountCommand) (string, error) {
	accountID := cmd.AccountID
	if accountID == "" {
		accountID = uuid.NewString()
		log.Printf("No AccountID provided, generated new ID: %s", accountID)
	}

	existingAccount, err := s.loadAccount(accountID)
	if err != nil && !errors.Is(err, domain.ErrAccountNotFound) {
		return "", fmt.Errorf("failed to check for existing account %s: %w", accountID, err)
	}
	if existingAccount != nil {
		log.Printf("Account creation attempt failed: Account %s already exists (version %d)", accountID, existingAccount.Version)
		return "", fmt.Errorf("%w: %s", domain.ErrAccountExists, accountID)
	}

	account := domain.NewAccount(accountID)

	err = account.HandleCreateAccount(accountID, cmd.InitialBalances)
	if err != nil {
		return "", fmt.Errorf("account creation failed validation: %w", err)
	}

	changes := account.GetUncommitedChanges()
	if len(changes) == 0 {
		log.Printf("ERROR: CreateAccount command for %s produced no events", accountID)
		return "", errors.New("internal error: create account produced no events")
	}

	err = s.eventStore.SaveEvents(accountID, 0, changes)
	if err != nil {
		return "", fmt.Errorf("failed to save creation events for account %s: %w", accountID, err)
	}

	log.Printf("Account %s created successfully. Version: %d", accountID, account.Version)

	s.saveSnapshotIfNeeded(account)

	return accountID, nil
}

func (s *AccountService) Deposit(cmd DepositMoneyCommand) error {
	account, err := s.loadAccount(cmd.AccountID)
	if err != nil {
		return fmt.Errorf("failed to load account %s for deposit: %w", cmd.AccountID, err)
	}

	initialVersion := account.Version

	err = account.HandleDeposit(cmd.Amount, cmd.Currency)
	if err != nil {
		return fmt.Errorf("deposit command failed for account %s: %w", cmd.AccountID, err)
	}

	changes := account.GetUncommitedChanges()
	if len(changes) == 0 {
		log.Printf("Deposit command for %s resulted in no state change (no events generated).", cmd.AccountID)
		return nil
	}

	err = s.eventStore.SaveEvents(cmd.AccountID, initialVersion, changes)
	if err != nil {
		return fmt.Errorf("failed to save deposit events for account %s: %w", cmd.AccountID, err)
	}

	log.Printf("Deposit of %s %s successful for account %s. New Version: %d", cmd.Amount.String(), cmd.Currency, cmd.AccountID, account.Version)

	s.saveSnapshotIfNeeded(account)
	return nil
}

func (s *AccountService) Withdraw(cmd WithdrawMoneyCommand) error {
	account, err := s.loadAccount(cmd.AccountID)
	if err != nil {
		return fmt.Errorf("failed to load account %s for withdrawal: %w", cmd.AccountID, err)
	}

	initialVersion := account.Version

	err = account.HandleWithdraw(cmd.Amount, cmd.Currency)
	if err != nil {
		if errors.Is(err, domain.ErrInsufficientFunds) {
			log.Printf("Withdrawal failed for %s: %v", cmd.AccountID, err)
			return err
		}
		return fmt.Errorf("withdrawal command failed for account %s: %w", cmd.AccountID, err)
	}

	changes := account.GetUncommitedChanges()
	if len(changes) == 0 {
		log.Printf("Withdraw command for %s resulted in no state change.", cmd.AccountID)
		return nil
	}

	err = s.eventStore.SaveEvents(cmd.AccountID, initialVersion, changes)
	if err != nil {
		return fmt.Errorf("failed to save withdrawal events for account %s: %w", cmd.AccountID, err)
	}

	log.Printf("Withdrawal of %s %s successful for account %s. New Version: %d", cmd.Amount.String(), cmd.Currency, cmd.AccountID, account.Version)
	s.saveSnapshotIfNeeded(account)
	return nil
}

func (s *AccountService) ConvertCurrency(cmd ConvertCurrencyCommand) error {
	account, err := s.loadAccount(cmd.AccountID)
	if err != nil {
		return fmt.Errorf("failed to load account %s for currency conversion: %w", cmd.AccountID, err)
	}

	initialVersion := account.Version

	rate, err := s.getExchangeRate(cmd.FromCurrency, cmd.ToCurrency)
	if err != nil {
		return fmt.Errorf("could not get exchange rate for %s -> %s: %w", cmd.FromCurrency, cmd.ToCurrency, err)
	}

	err = account.HandleConvertCurrency(cmd.FromAmount, cmd.FromCurrency, cmd.ToCurrency, rate)
	if err != nil {
		if errors.Is(err, domain.ErrInsufficientFunds) {
			log.Printf("Currency conversion failed for %s: %v", cmd.AccountID, err)
			return err
		}
		return fmt.Errorf("currency conversion command failed for account %s: %w", cmd.AccountID, err)
	}

	changes := account.GetUncommitedChanges()
	if len(changes) == 0 {
		log.Printf("ConvertCurrency command for %s resulted in no state change.", cmd.AccountID)
		return nil
	}

	err = s.eventStore.SaveEvents(cmd.AccountID, initialVersion, changes)
	if err != nil {
		return fmt.Errorf("failed to save conversion events for account %s: %w", cmd.AccountID, err)
	}

	log.Printf("Conversion of %s %s -> %s successful for account %s. Rate: %s. New Version: %d",
		cmd.FromAmount.String(), cmd.FromCurrency, cmd.ToCurrency, cmd.AccountID, rate.String(), account.Version)
	s.saveSnapshotIfNeeded(account)
	return nil
}

func (s *AccountService) TransferMoney(cmd TransferMoneyCommand) error {
	sourceAccount, err := s.loadAccount(cmd.SourceAccountID)
	if err != nil {
		return fmt.Errorf("failed to load source account %s for transfer: %w", cmd.SourceAccountID, err)
	}
	initialSourceVersion := sourceAccount.Version

	targetAccount, err := s.loadAccount(cmd.TargetAccountID)
	if err != nil {
		if errors.Is(err, domain.ErrAccountNotFound) {
			log.Printf("Transfer failed: Target account %s not found.", cmd.TargetAccountID)
			return fmt.Errorf("target account %s not found for transfer: %w", cmd.TargetAccountID, err)
		}
		return fmt.Errorf("failed to load target account %s for transfer: %w", cmd.TargetAccountID, err)
	}
	initialTargetVersion := targetAccount.Version

	transferID := uuid.NewString()

	debitAmount := cmd.Amount
	debitCurrency := cmd.Currency
	creditAmount := cmd.Amount     // For same-currency transfer
	creditCurrency := cmd.Currency // For same-currency transfer
	rate := decimal.NewFromInt(1)  // For same-currency transfer

	err = sourceAccount.HandleInitiateTransfer(transferID, cmd.TargetAccountID, debitAmount, debitCurrency, creditAmount, creditCurrency, rate)
	if err != nil {
		log.Printf("Transfer failed (debit phase) for source %s: %v", cmd.SourceAccountID, err)
		return fmt.Errorf("transfer command failed for source account %s: %w", cmd.SourceAccountID, err)
	}

	sourceChanges := sourceAccount.GetUncommitedChanges()
	if len(sourceChanges) > 0 {
		err = s.eventStore.SaveEvents(cmd.SourceAccountID, initialSourceVersion, sourceChanges)
		if err != nil {
			log.Printf("CRITICAL ERROR: Failed to save transfer debit events for account %s (TransferID: %s): %v. State is inconsistent.", cmd.SourceAccountID, transferID, err)
			return fmt.Errorf("failed to save transfer debit events for account %s (TransferID: %s): %w. System may be in an inconsistent state", cmd.SourceAccountID, transferID, err)
		}
		log.Printf("Transfer (Debit) of %s %s from %s to %s successful (TransferID: %s). Source New Version: %d",
			debitAmount.String(), debitCurrency, cmd.SourceAccountID, cmd.TargetAccountID, transferID, sourceAccount.Version)
		s.saveSnapshotIfNeeded(sourceAccount)
	} else {
		log.Printf("Warning: HandleInitiateTransfer for source %s (TransferID: %s) resulted in no state change.", cmd.SourceAccountID, transferID)
	}

	err = targetAccount.HandleReceiveTransfer(transferID, cmd.SourceAccountID, cmd.TargetAccountID, debitAmount, debitCurrency, creditAmount, creditCurrency, rate)
	if err != nil {
		// Should implement a compensating action for source account if this fails.
		log.Printf("CRITICAL ERROR: Transfer partially failed (TransferID: %s). Source %s debited, but crediting target %s failed: %v. Manual intervention may be required.", transferID, cmd.SourceAccountID, cmd.TargetAccountID, err)
		return fmt.Errorf("transfer failed during credit to target account %s (TransferID: %s): %w. Source account %s was debited. Manual intervention likely required", cmd.TargetAccountID, transferID, err, cmd.SourceAccountID)
	}

	targetChanges := targetAccount.GetUncommitedChanges()
	if len(targetChanges) > 0 {
		err = s.eventStore.SaveEvents(cmd.TargetAccountID, initialTargetVersion, targetChanges)
		if err != nil {
			// Should implement a compensating action for source account.
			log.Printf("CRITICAL ERROR: Failed to save transfer credit events for target account %s (TransferID: %s): %v. State is inconsistent.", cmd.TargetAccountID, transferID, err)
			return fmt.Errorf("failed to save transfer credit events for target account %s (TransferID: %s): %w. System may be in an inconsistent state", cmd.TargetAccountID, transferID, err)
		}
		log.Printf("Transfer (Credit) of %s %s to %s from %s successful (TransferID: %s). Target New Version: %d",
			creditAmount.String(), creditCurrency, cmd.TargetAccountID, cmd.SourceAccountID, transferID, targetAccount.Version)
		s.saveSnapshotIfNeeded(targetAccount)
	} else {
		log.Printf("Warning: HandleReceiveTransfer for target %s (TransferID: %s) resulted in no state change.", cmd.TargetAccountID, transferID)
	}

	log.Printf("Transfer (TransferID: %s) from %s to %s completed successfully.", transferID, cmd.SourceAccountID, cmd.TargetAccountID)
	return nil
}

// --- Query Handlers ---
// These methods retrieve information about accounts without changing state.

func (s *AccountService) GetCurrentBalance(query GetBalanceQuery) (map[shared.Currency]decimal.Decimal, error) {
	account, err := s.loadAccount(query.AccountID)
	if err != nil {
		if errors.Is(err, domain.ErrAccountNotFound) {
			return nil, fmt.Errorf("cannot get balance: %w", err)
		}
		return nil, fmt.Errorf("failed to load account %s for balance query: %w", query.AccountID, err)
	}

	balancesCopy := make(map[shared.Currency]decimal.Decimal)

	if query.Currency != nil {
		balance := account.Balances[*query.Currency]
		balancesCopy[*query.Currency] = balance
	} else {
		for cur, bal := range account.Balances {
			balancesCopy[cur] = bal
		}
	}
	return balancesCopy, nil
}

func (s *AccountService) GetTransactionHistory(query GetHistoryQuery) ([]events.Event, error) {
	history, err := s.eventStore.GetEvents(query.AccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get event history for account %s: %w", query.AccountID, err)
	}

	if len(history) == 0 {
		_, errLoad := s.loadAccount(query.AccountID)
		if errors.Is(errLoad, domain.ErrAccountNotFound) {
			return nil, fmt.Errorf("%w: cannot get history: account %s not found", domain.ErrAccountNotFound, query.AccountID)
		}
		return []events.Event{}, nil
	}

	totalEvents := len(history)
	start := query.Skip
	if start < 0 {
		start = 0
	}
	if start >= totalEvents {
		return []events.Event{}, nil
	}

	end := start + query.Limit
	if query.Limit <= 0 || end > totalEvents {
		end = totalEvents
	}

	return history[start:end], nil
}

// --- Aggregate Loading & Snapshotting Logic ---

func (s *AccountService) loadAccount(accountID string) (*domain.Account, error) {
	var account *domain.Account
	var snapshotVersion int = 0

	snapshot, found, err := s.snapshotStore.GetLatestSnapshot(accountID)
	if err != nil {
		log.Printf("Warning: Error loading snapshot for account %s: %v. Attempting full event replay.", accountID, err)
		found = false
	}

	if found {
		account, err = domain.ApplySnapshot(snapshot)
		if err != nil {
			log.Printf("ERROR: Failed to apply snapshot version %d for account %s: %v. Rebuilding from all events.", snapshot.Version, accountID, err)
			account = domain.NewAccount(accountID)
			snapshotVersion = 0
		} else {
			log.Printf("Loaded account %s from snapshot version %d", accountID, account.Version)
			snapshotVersion = account.Version
		}
	} else {
		account = domain.NewAccount(accountID)
		if err == nil {
			log.Printf("No snapshot found for account %s, loading events.", accountID)
		}
	}

	eventsToApply, err := s.eventStore.GetEventsAfterVersion(accountID, snapshotVersion)
	if err != nil {
		if snapshotVersion == 0 {
			return nil, fmt.Errorf("failed to load initial events for account %s: %w", accountID, err)
		}
		log.Printf("Warning: Failed to get events after version %d for %s: %v. State might be up-to-date if no newer events exist.", snapshotVersion, accountID, err)
	}

	if len(eventsToApply) > 0 {
		log.Printf("Applying %d events to account %s starting after version %d", len(eventsToApply), accountID, snapshotVersion)
		err = account.ApplyEvents(eventsToApply)
		if err != nil {
			return nil, fmt.Errorf("critical error applying events to account %s after snapshot/initial load: %w", accountID, err)
		}
	}

	if account.Version == 0 {
		log.Printf("Account %s not found (version is 0 after loading attempts).", accountID)
		return nil, fmt.Errorf("%w: %s", domain.ErrAccountNotFound, accountID)
	}

	log.Printf("Account %s loaded successfully. Current Version: %d", accountID, account.Version)
	return account, nil
}

func (s *AccountService) saveSnapshotIfNeeded(account *domain.Account) {
	if account.Version%SnapshotFrequency == 0 && account.Version > 0 {
		log.Printf("Snapshot condition met for account %s at version %d (Frequency: %d)", account.ID, account.Version, SnapshotFrequency)

		snapshot, err := domain.CreateSnapshot(account)
		if err != nil {
			log.Printf("ERROR: Failed to create snapshot for account %s at version %d: %v", account.ID, account.Version, err)
			return
		}

		err = s.snapshotStore.SaveSnapshot(snapshot)
		if err != nil {
			log.Printf("ERROR: Failed to save snapshot for account %s at version %d: %v", account.ID, account.Version, err)
		} else {
			log.Printf("Snapshot saved successfully for account %s at version %d", account.ID, account.Version)
		}
	}
}

// --- Dummy Exchange Rate Provider ---
// Replace with a real implementation reading from config, cache, or external API.
func (s *AccountService) getExchangeRate(from, to shared.Currency) (decimal.Decimal, error) {
	if from == to {
		return decimal.NewFromInt(1), nil // Rate is 1 for same currency
	}

	// Example hardcoded rates
	rates := map[shared.Currency]map[shared.Currency]string{
		shared.USD: {shared.EUR: "0.92", shared.GBP: "0.80"},
		shared.EUR: {shared.USD: "1.08", shared.GBP: "0.87"},
		shared.GBP: {shared.USD: "1.25", shared.EUR: "1.15"},
	}

	// Look up direct rate
	if rateMap, ok := rates[from]; ok {
		if rateStr, ok2 := rateMap[to]; ok2 {
			rate, err := decimal.NewFromString(rateStr)
			if err != nil {
				// Should not happen with valid hardcoded strings
				log.Printf("ERROR: Internal error parsing rate string '%s' for %s->%s: %v", rateStr, from, to, err)
				return decimal.Zero, fmt.Errorf("internal rate parse error for %s->%s", from, to)
			}
			log.Printf("Found direct exchange rate %s -> %s: %s", from, to, rate.String())
			return rate, nil
		}
	}

	// Look up inverse rate (e.g., if only USD->EUR is stored, calculate EUR->USD)
	if rateMap, ok := rates[to]; ok {
		if inverseRateStr, ok2 := rateMap[from]; ok2 {
			inverseRate, err := decimal.NewFromString(inverseRateStr)
			if err != nil {
				log.Printf("ERROR: Internal error parsing inverse rate string '%s' for %s->%s: %v", inverseRateStr, to, from, err)
				return decimal.Zero, fmt.Errorf("internal inverse rate parse error for %s->%s", from, to)
			}
			if inverseRate.IsZero() {
				log.Printf("ERROR: Cannot invert zero rate for %s -> %s", to, from)
				return decimal.Zero, fmt.Errorf("cannot invert zero rate for %s -> %s", to, from)
			}
			// Calculate rate = 1 / inverseRate
			// Use appropriate precision for division
			rate := decimal.NewFromInt(1).Div(inverseRate) // Consider .DivRound(inverseRate, precision)
			log.Printf("Calculated inverse exchange rate %s -> %s: %s (from %s->%s: %s)", from, to, rate.String(), to, from, inverseRate.String())
			return rate, nil
		}
	}

	log.Printf("ERROR: Exchange rate not found for %s -> %s", from, to)
	return decimal.Zero, fmt.Errorf("exchange rate not found for %s -> %s", from, to)
}
