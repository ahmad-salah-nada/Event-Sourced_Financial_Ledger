package cmd

import (
	"fmt"
	"strings"

	"financial-ledger/app"
	"financial-ledger/shared"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
)

var (
	accountID string
	balances  []string
)

// accountCmd represents the account command group
var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Manage financial accounts",
	Long:  `Provides commands to create and manage financial accounts.`,
}

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new financial account",
	Long: `Creates a new financial account with an optional ID and initial balances.
If --id is not provided, a new UUID will be generated.
Initial balances can be set using the --balance flag multiple times,
e.g., --balance USD:100.50 --balance EUR:50`,
	Run: func(cmd *cobra.Command, args []string) {
		// Generate ID if not provided
		if accountID == "" {
			accountID = uuid.New().String()
			// ID generation is handled by the service now if empty
		}

		initialBalancesMap := make(map[shared.Currency]decimal.Decimal)
		for _, b := range balances {
			parts := strings.SplitN(b, ":", 2)
			if len(parts) != 2 {
				exitWithError(fmt.Errorf("invalid balance format: %q. Use CURRENCY:AMOUNT (e.g., USD:100.50)", b))
			}
			currency := shared.Currency(strings.ToUpper(parts[0]))
			// Basic validation - could be more robust
			if currency != shared.USD && currency != shared.EUR && currency != shared.GBP {
				exitWithError(fmt.Errorf("invalid currency code: %q. Supported: USD, EUR, GBP", currency))
			}
			amount, err := decimal.NewFromString(parts[1])
			if err != nil {
				exitWithError(fmt.Errorf("invalid amount format for %s: %q. %v", currency, parts[1], err))
			}
			if amount.IsNegative() {
				exitWithError(fmt.Errorf("initial balance cannot be negative: %s %s", currency, amount))
			}
			if _, exists := initialBalancesMap[currency]; exists {
				exitWithError(fmt.Errorf("duplicate initial balance provided for currency: %s", currency))
			}
			initialBalancesMap[currency] = amount
		}

		createCmdInput := app.CreateAccountCommand{
			AccountID:       accountID, // Pass the user-provided ID (or empty string)
			InitialBalances: initialBalancesMap,
		}

		// The service now handles ID generation if cmd.AccountID is empty and returns the ID used
		accountIDUsed, err := accountService.CreateAccount(createCmdInput)
		if err != nil {
			// Check for specific domain errors if needed, e.g., account exists
			exitWithError(fmt.Errorf("failed to create account: %w", err))
		}

		fmt.Printf("Account '%s' created successfully.\n", accountIDUsed)
		// Optionally display initial balances
		if len(initialBalancesMap) > 0 {
			fmt.Println("Initial Balances:")
			// Iterate over the map for display
			for cur, amt := range initialBalancesMap {
				fmt.Printf("  %s: %s\n", cur, amt.StringFixed(2))
			}
		}
	},
}

func init() {
	// Add accountCmd to root command
	rootCmd.AddCommand(accountCmd)

	// Add createCmd to accountCmd
	accountCmd.AddCommand(createCmd)

	// Define flags for createCmd
	createCmd.Flags().StringVar(&accountID, "id", "", "Optional unique ID for the account (UUID generated if empty)")
	createCmd.Flags().StringSliceVarP(&balances, "balance", "b", []string{}, "Initial balance(s) in CURRENCY:AMOUNT format (e.g., USD:100.50). Can be used multiple times.")
}
