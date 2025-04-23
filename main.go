package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/shopspring/decimal"

	"financial-ledger/app"
	"financial-ledger/domain"
	"financial-ledger/events"
	"financial-ledger/shared"
	"financial-ledger/store"
)

func main() {
	log.SetOutput(os.Stdout)
	// Ldate | Ltime for date and time, Lshortfile for file:line
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	log.Println("Starting Financial Ledger Service...")

	eventStore := store.NewInMemoryEventStore()
	snapshotStore := store.NewInMemorySnapshotStore()
	accountService := app.NewAccountService(eventStore, snapshotStore)

	fmt.Println("\n--- Simulating Operations ---")

	fmt.Println("\n[Step 1] Creating Accounts...")
	initialBalancesAlice := map[shared.Currency]decimal.Decimal{
		shared.USD: decimal.NewFromFloat(1000.50), // Use float for easier example init
		shared.EUR: decimal.NewFromInt(500),
	}
	aliceCmd := app.CreateAccountCommand{InitialBalances: initialBalancesAlice}
	aliceID, err := accountService.CreateAccount(aliceCmd)
	if err != nil {
		log.Fatalf("Failed to create Alice's account: %v", err)
	}
	fmt.Printf(" -> Alice's Account ID: %s\n", aliceID)

	initialBalancesBob := map[shared.Currency]decimal.Decimal{
		shared.GBP: decimal.NewFromInt(800),
	}
	bobCmd := app.CreateAccountCommand{InitialBalances: initialBalancesBob}
	bobID, err := accountService.CreateAccount(bobCmd)
	if err != nil {
		log.Fatalf("Failed to create Bob's account: %v", err)
	}
	fmt.Printf(" -> Bob's Account ID: %s\n", bobID)

	fmt.Println("\n[Step 1b] Testing duplicate account creation (should fail)...")
	_, err = accountService.CreateAccount(app.CreateAccountCommand{AccountID: aliceID, InitialBalances: initialBalancesAlice})
	if err != nil && errors.Is(err, domain.ErrAccountExists) {
		fmt.Printf(" -> Attempt to re-create Alice's account failed as expected: %v\n", err)
	} else if err != nil {
		log.Fatalf("Expected ErrAccountExists, but got different error: %v", err)
	} else {
		log.Fatalf("Error: Should not be able to create existing account %s", aliceID)
	}

	fmt.Println("\n[Step 2] Making Deposits...")
	depositCmd := app.DepositMoneyCommand{
		AccountID: aliceID,
		Amount:    decimal.NewFromInt(200),
		Currency:  shared.USD,
	}
	err = accountService.Deposit(depositCmd)
	handleOperationError("Deposit to Alice (USD)", err)

	fmt.Println("\n[Step 3] Making Withdrawals...")
	withdrawCmd := app.WithdrawMoneyCommand{
		AccountID: aliceID,
		Amount:    decimal.NewFromInt(50),
		Currency:  shared.EUR,
	}
	err = accountService.Withdraw(withdrawCmd)
	handleOperationError("Withdrawal from Alice (EUR)", err)

	fmt.Println("\n[Step 3b] Testing insufficient funds withdrawal (should fail)...")
	insufficientWithdrawCmd := app.WithdrawMoneyCommand{
		AccountID: bobID,
		Amount:    decimal.NewFromInt(1000),
		Currency:  shared.GBP,
	}
	err = accountService.Withdraw(insufficientWithdrawCmd)
	if err != nil && errors.Is(err, domain.ErrInsufficientFunds) {
		fmt.Printf(" -> Withdrawal failed for Bob due to insufficient funds, as expected: %v\n", err)
	} else if err != nil {
		handleOperationError("Insufficient Withdrawal from Bob (GBP)", err) // Log unexpected error
	} else {
		log.Fatalf("Error: Withdrawal of insufficient funds from Bob should have failed, but succeeded.")
	}

	fmt.Println("\n[Step 4] Converting Currency...")
	convertCmd := app.ConvertCurrencyCommand{
		AccountID:    aliceID,
		FromAmount:   decimal.NewFromInt(100),
		FromCurrency: shared.USD,
		ToCurrency:   shared.EUR,
	}
	err = accountService.ConvertCurrency(convertCmd)
	handleOperationError("Currency Conversion for Alice (USD->EUR)", err)

	fmt.Println("\n[Step 5] Transferring Money (Alice USD -> Bob USD)...")
	fmt.Println("   (Note: This simulation only performs the debit; credit is simulated separately and is not atomic)")
	transferCmd := app.TransferMoneyCommand{
		SourceAccountID: aliceID,
		TargetAccountID: bobID,
		Amount:          decimal.NewFromInt(75),
		Currency:        shared.USD, // Alice sends USD
	}
	err = accountService.TransferMoney(transferCmd)
	if err != nil {
		handleOperationError("Transfer Debit from Alice", err)
	} else {
		fmt.Println(" -> Transfer Debit successful from Alice.")
		// Simulate the credit part (normally done by a Saga/Process Manager)
		fmt.Println("   Simulating corresponding deposit to Bob...")
		creditCmd := app.DepositMoneyCommand{AccountID: bobID, Amount: decimal.NewFromInt(75), Currency: shared.USD}
		err = accountService.Deposit(creditCmd)
		handleOperationError("Simulated Transfer Credit to Bob", err)
	}

	fmt.Println("\n[Step 6] Querying Final Balances...")
	displayBalances("Alice", aliceID, accountService)
	displayBalances("Bob", bobID, accountService)

	fmt.Println("\n[Step 7] Querying Transaction History...")
	displayHistory("Alice", aliceID, accountService)
	displayHistory("Bob", bobID, accountService)

	fmt.Println("\n[Step 8] Testing Snapshotting...")
	snapTestCmd := app.CreateAccountCommand{InitialBalances: map[shared.Currency]decimal.Decimal{shared.USD: decimal.Zero}}
	snapTestID, err := accountService.CreateAccount(snapTestCmd)
	if err != nil {
		log.Fatalf("Failed to create Snapshot Test account: %v", err)
	}
	fmt.Printf(" -> Snapshot Test Account ID: %s\n", snapTestID)
	fmt.Printf(" -> Applying %d events to trigger snapshot (Frequency: %d)...\n", app.SnapshotFrequency+5, app.SnapshotFrequency)

	snapshotTriggered := false
	snapshotChecked := false
	for i := 0; i < app.SnapshotFrequency+5; i++ {
		depositLoopCmd := app.DepositMoneyCommand{AccountID: snapTestID, Amount: decimal.NewFromInt(1), Currency: shared.USD}
		err = accountService.Deposit(depositLoopCmd)
		if err != nil {
			log.Printf("Error during snapshot test deposit %d for %s: %v", i+1, snapTestID, err)
			break
		}

		if !snapshotChecked && i == (app.SnapshotFrequency-2) {
			_, found, _ := snapshotStore.GetLatestSnapshot(snapTestID)
			if found {
				snapshotTriggered = true
				snapshotChecked = true
				fmt.Println(" -> Snapshot detected in store.")
			}
		}
	}

	if snapshotTriggered {
		fmt.Println(" -> Snapshot appears to have been triggered and saved during the loop.")
	} else {
		_, found, _ := snapshotStore.GetLatestSnapshot(snapTestID)
		if found {
			fmt.Println(" -> Snapshot detected in store after loop finished.")
		} else {
			fmt.Println(" -> Snapshot was NOT detected after sufficient events. Check logs/logic.")
		}
	}

	fmt.Println(" -> Reloading snapshot test account (check logs for snapshot usage):")
	displayBalances("Snapshot Test Account", snapTestID, accountService)

	fmt.Println("\n--- Simulation Complete ---")
}

func handleOperationError(operationName string, err error) {
	if err != nil {
		log.Printf(" -> ERROR during operation '%s': %v", operationName, err)
	} else {
		fmt.Printf(" -> Operation '%s' successful.\n", operationName)
	}
}

func displayBalances(accountName, accountID string, service *app.AccountService) {
	balances, err := service.GetCurrentBalance(app.GetBalanceQuery{AccountID: accountID})
	if err != nil {
		log.Printf("Error getting %s's balances: %v", accountName, err)
	} else {
		fmt.Printf("%s's Balances (ID: %s):\n", accountName, accountID)
		if len(balances) == 0 {
			fmt.Println("  (No balances held)")
		}
		for cur, bal := range balances {
			// Use .StringFixed(2) for typical currency formatting
			fmt.Printf("  %s: %s\n", cur, bal.StringFixed(2))
		}
	}
}

func displayHistory(accountName, accountID string, service *app.AccountService) {
	history, err := service.GetTransactionHistory(app.GetHistoryQuery{AccountID: accountID}) // Get all history
	if err != nil {
		log.Printf("Error getting %s's history: %v", accountName, err)
	} else {
		fmt.Printf("%s's History (ID: %s) (%d events):\n", accountName, accountID, len(history))
		if len(history) == 0 {
			fmt.Println("  (No history found)")
		}
		for i, event := range history {
			base := event.GetBase()
			// Use a more readable timestamp format
			fmt.Printf("  %d: [%s] %T (v%d)\n", i+1, base.Timestamp.Format(time.RFC3339), event, base.Version)
			// Add specific details based on event type for clarity
			switch e := event.(type) {
			case events.AccountCreatedEvent:
				fmt.Printf("     Initial Balances: %v\n", e.InitialBalances)
			case events.DepositMadeEvent:
				fmt.Printf("     Amount: %s %s\n", e.Amount.StringFixed(2), e.Currency)
			case events.WithdrawalMadeEvent:
				fmt.Printf("     Amount: %s %s\n", e.Amount.StringFixed(2), e.Currency)
			case events.CurrencyConvertedEvent:
				fmt.Printf("     From: %s %s, To: %s %s, Rate: %s\n", e.FromAmount.StringFixed(2), e.FromCurrency, e.ToAmount.StringFixed(2), e.ToCurrency, e.ExchangeRate.String())
			case events.MoneyTransferredEvent:
				fmt.Printf("     To Account: %s, Debited: %s %s, Credited*: %s %s, Rate: %s\n",
					e.TargetAccountID, e.DebitedAmount.StringFixed(2), e.DebitedCurrency, e.CreditedAmount.StringFixed(2), e.CreditedCurrency, e.ExchangeRate.String())
				fmt.Printf("     (*Credit amount/currency intended for target account)\n")
			default:
				fmt.Printf("     (Details not displayed for this event type)\n")
			}
		}
	}
}
