package cmd

import (
	"fmt"
	"sort"

	"encoding/json"
	"financial-ledger/app"
	"financial-ledger/events"
	"financial-ledger/shared" // Needed for event details potentially
	"time"

	"github.com/spf13/cobra"
)

// Variables for query flags
var (
	queryAccountID string
	queryCurrency  string // Optional currency for balance query
	querySkip      int
	queryLimit     int
)

// queryCmd represents the query command group
var queryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query account information",
	Long:  `Provides commands to query account balances and transaction history.`,
}

// balanceCmd represents the balance command
var balanceCmd = &cobra.Command{
	Use:   "balance",
	Short: "Get account balance(s)",
	Long: `Retrieves the current balance for one or all currencies in a specified account.
If --currency is omitted, all balances are shown.`,
	Run: func(cmd *cobra.Command, args []string) {
		if queryAccountID == "" {
			exitWithError(fmt.Errorf("account ID (--id) is required"))
		}

		var targetCurrency *shared.Currency
		if queryCurrency != "" {
			c := shared.Currency(queryCurrency)
			if !isValidCurrency(c) { // Reusing validation func from transaction.go
				exitWithError(fmt.Errorf("invalid currency code: %q. Supported: USD, EUR, GBP", c))
			}
			targetCurrency = &c
		}

		queryInput := app.GetBalanceQuery{
			AccountID: queryAccountID,
			Currency:  targetCurrency,
		}

		balances, err := accountService.GetCurrentBalance(queryInput)
		if err != nil {
			// Handle account not found specifically
			// if errors.Is(err, domain.ErrAccountNotFound) { ... }
			exitWithError(fmt.Errorf("failed to get balance: %w", err))
		}

		if len(balances) == 0 {
			if targetCurrency != nil {
				// If a specific currency was requested and not found, it means the balance is zero
				fmt.Printf("Account '%s' Balance (%s): 0.00\n", queryAccountID, *targetCurrency)
			} else {
				// If all balances were requested and the map is empty, the account might exist but have zero balances
				// (or potentially it doesn't exist, though the service call should have errored)
				fmt.Printf("Account '%s' has no balances or does not exist.\n", queryAccountID)
			}
			return
		}

		fmt.Printf("Account '%s' Balances:\n", queryAccountID)
		// Sort currencies for consistent output
		currencies := make([]shared.Currency, 0, len(balances))
		for cur := range balances {
			currencies = append(currencies, cur)
		}
		sort.Slice(currencies, func(i, j int) bool {
			return currencies[i] < currencies[j]
		})

		for _, cur := range currencies {
			fmt.Printf("  %s: %s\n", cur, balances[cur].StringFixed(2))
		}
	},
}

// historyCmd represents the history command
var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Get account transaction history (events)",
	Long:  `Retrieves the sequence of events (transaction history) for a specified account, with optional pagination.`,
	Run: func(cmd *cobra.Command, args []string) {
		if queryAccountID == "" {
			exitWithError(fmt.Errorf("account ID (--id) is required"))
		}

		// Basic validation for pagination flags
		if querySkip < 0 {
			exitWithError(fmt.Errorf("skip value cannot be negative"))
		}
		// Allow limit 0 (meaning no limit, handled by service) or positive
		if queryLimit < 0 {
			exitWithError(fmt.Errorf("limit value cannot be negative"))
		}

		queryInput := app.GetHistoryQuery{
			AccountID: queryAccountID,
			Skip:      querySkip,
			Limit:     queryLimit, // Service likely handles 0 as 'no limit' or a default
		}

		history, err := accountService.GetTransactionHistory(queryInput)
		if err != nil {
			// Handle account not found specifically
			// if errors.Is(err, domain.ErrAccountNotFound) { ... }
			exitWithError(fmt.Errorf("failed to get history: %w", err))
		}

		if len(history) == 0 {
			fmt.Printf("No transaction history found for account '%s' (or account does not exist).\n", queryAccountID)
			return
		}

		fmt.Printf("Transaction History for Account '%s':\n", queryAccountID)
		fmt.Println("--------------------------------------------------")
		for i, event := range history {
			fmt.Printf("Event %d:\n", querySkip+i+1) // Adjust index based on skip
			printEventDetails(event)
			fmt.Println("--------------------------------------------------")
		}
	},
}

// printEventDetails formats and prints the details of a single event.
// This function uses type assertions to print specific fields for known event types.
func printEventDetails(event events.Event) {
	base := event.GetBase()
	fmt.Printf("  Type:      %s\n", base.Type)
	fmt.Printf("  EventID:   %s\n", base.EventID.String()) // Use .String() for UUID
	fmt.Printf("  Version:   %d\n", base.Version)
	fmt.Printf("  Timestamp: %s\n", base.Timestamp.Format(time.RFC3339))

	// Use type assertion to get specific event details
	switch e := event.(type) {
	case *events.AccountCreatedEvent:
		fmt.Println("  Details:")
		if len(e.InitialBalances) > 0 {
			for _, bal := range e.InitialBalances {
				fmt.Printf("    Initial Balance: %s %s\n", bal.Currency, bal.Amount.StringFixed(2))
			}
		} else {
			fmt.Println("    (No initial balances)")
		}
	case *events.DepositMadeEvent:
		fmt.Println("  Details:")
		fmt.Printf("    Amount:   %s\n", e.Amount.StringFixed(2))
		fmt.Printf("    Currency: %s\n", e.Currency)
	case *events.WithdrawalMadeEvent:
		fmt.Println("  Details:")
		fmt.Printf("    Amount:   %s\n", e.Amount.StringFixed(2))
		fmt.Printf("    Currency: %s\n", e.Currency)
	case *events.CurrencyConvertedEvent:
		fmt.Println("  Details:")
		fmt.Printf("    From:     %s %s\n", e.FromCurrency, e.FromAmount.StringFixed(2))
		fmt.Printf("    To:       %s %s\n", e.ToCurrency, e.ToAmount.StringFixed(2))
		fmt.Printf("    Rate:     %s\n", e.ExchangeRate.String())
	case *events.MoneyTransferredEvent:
		fmt.Println("  Details (Debit from Source):")
		fmt.Printf("    Target Account: %s\n", e.TargetAccountID)
		fmt.Printf("    Debited:        %s %s\n", e.DebitedCurrency, e.DebitedAmount.StringFixed(2))
		fmt.Printf("    Intended Credit: %s %s\n", e.CreditedCurrency, e.CreditedAmount.StringFixed(2))
		fmt.Printf("    Rate:           %s\n", e.ExchangeRate.String())
	default:
		// Fallback for unknown event types: print JSON representation
		fmt.Println("  Details (Raw JSON):")
		jsonData, err := json.MarshalIndent(event, "    ", "  ")
		if err != nil {
			fmt.Printf("    Error marshalling event: %v\n", err)
		} else {
			fmt.Printf("    %s\n", string(jsonData))
		}
	}
}

func init() {
	// Add queryCmd to root command
	rootCmd.AddCommand(queryCmd)

	// Add balanceCmd to queryCmd
	queryCmd.AddCommand(balanceCmd)

	// Define flags for balanceCmd
	balanceCmd.Flags().StringVar(&queryAccountID, "id", "", "Account ID to query (required)")
	balanceCmd.Flags().StringVar(&queryCurrency, "currency", "", "Optional currency code (USD, EUR, GBP) to get specific balance")
	_ = balanceCmd.MarkFlagRequired("id")

	// Add historyCmd to queryCmd
	queryCmd.AddCommand(historyCmd)

	// Define flags for historyCmd
	historyCmd.Flags().StringVar(&queryAccountID, "id", "", "Account ID to query (required)")
	historyCmd.Flags().IntVar(&querySkip, "skip", 0, "Number of events to skip (for pagination)")
	historyCmd.Flags().IntVar(&queryLimit, "limit", 0, "Maximum number of events to return (0 for no limit)")
	_ = historyCmd.MarkFlagRequired("id")
}
