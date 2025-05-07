package cmd

import (
	"fmt"
	"strings"

	"financial-ledger/app"
	"financial-ledger/shared"

	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
)

// Variables to hold flag values for transaction commands
var (
	txAccountID    string // Use a different name to avoid conflict with account.go's accountID
	txCurrency     string
	txAmountStr    string
	txFromCurrency string
	txToCurrency   string
	txFromID       string
	txToID         string
)

// transactionCmd represents the transaction command group
var transactionCmd = &cobra.Command{
	Use:   "transaction",
	Short: "Perform financial transactions",
	Long:  `Provides commands for depositing, withdrawing, converting, and transferring funds between accounts.`,
}

// depositCmd represents the deposit command
var depositCmd = &cobra.Command{
	Use:   "deposit",
	Short: "Deposit funds into an account",
	Long:  `Adds a specified amount of a given currency to an account's balance.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Validate required flags
		if txAccountID == "" {
			exitWithError(fmt.Errorf("account ID (--id) is required"))
		}
		if txCurrency == "" {
			exitWithError(fmt.Errorf("currency (--currency) is required"))
		}
		if txAmountStr == "" {
			exitWithError(fmt.Errorf("amount (--amount) is required"))
		}

		currency := shared.Currency(txCurrency)
		if !isValidCurrency(currency) {
			exitWithError(fmt.Errorf("invalid currency code: %q. Supported: USD, EUR, GBP", currency))
		}

		amount, err := decimal.NewFromString(txAmountStr)
		if err != nil {
			exitWithError(fmt.Errorf("invalid amount format: %q. %v", txAmountStr, err))
		}
		if amount.IsNegative() || amount.IsZero() {
			exitWithError(fmt.Errorf("deposit amount must be positive: %s", amount))
		}

		depositCmdInput := app.DepositMoneyCommand{
			AccountID: txAccountID,
			Amount:    amount,
			Currency:  currency,
		}

		err = accountService.Deposit(depositCmdInput)
		if err != nil {
			exitWithError(fmt.Errorf("failed to deposit funds: %w", err))
		}

		fmt.Printf("Successfully deposited %s %s into account '%s'.\n", amount.StringFixed(2), currency, txAccountID)
	},
}

// withdrawCmd represents the withdraw command
var withdrawCmd = &cobra.Command{
	Use:   "withdraw",
	Short: "Withdraw funds from an account",
	Long:  `Removes a specified amount of a given currency from an account's balance, checking for sufficient funds.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Validate required flags (reusing txAccountID, txCurrency, txAmountStr)
		if txAccountID == "" {
			exitWithError(fmt.Errorf("account ID (--id) is required"))
		}
		if txCurrency == "" {
			exitWithError(fmt.Errorf("currency (--currency) is required"))
		}
		if txAmountStr == "" {
			exitWithError(fmt.Errorf("amount (--amount) is required"))
		}

		currency := shared.Currency(txCurrency)
		if !isValidCurrency(currency) {
			exitWithError(fmt.Errorf("invalid currency code: %q. Supported: USD, EUR, GBP", currency))
		}

		amount, err := decimal.NewFromString(txAmountStr)
		if err != nil {
			exitWithError(fmt.Errorf("invalid amount format: %q. %v", txAmountStr, err))
		}
		if amount.IsNegative() || amount.IsZero() {
			exitWithError(fmt.Errorf("withdrawal amount must be positive: %s", amount))
		}

		withdrawCmdInput := app.WithdrawMoneyCommand{
			AccountID: txAccountID,
			Amount:    amount,
			Currency:  currency,
		}

		err = accountService.Withdraw(withdrawCmdInput)
		if err != nil {
			// Specific error handling for insufficient funds is good UX
			// The service layer already logs this, but we inform the CLI user directly.
			// Note: The domain.ErrInsufficientFunds might be wrapped, so errors.Is is preferred.
			// if errors.Is(err, domain.ErrInsufficientFunds) {
			//     exitWithError(err) // Just pass the specific error message
			// }
			// For other errors, wrap them:
			exitWithError(fmt.Errorf("failed to withdraw funds: %w", err))
		}

		fmt.Printf("Successfully withdrew %s %s from account '%s'.\n", amount.StringFixed(2), currency, txAccountID)
	},
}

// convertCmd represents the convert command
var convertCmd = &cobra.Command{
	Use:   "convert",
	Short: "Convert currency within an account",
	Long:  `Converts a specified amount from one currency to another within the same account, using predefined exchange rates.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Validate required flags
		if txAccountID == "" {
			exitWithError(fmt.Errorf("account ID (--id) is required"))
		}
		if txFromCurrency == "" {
			exitWithError(fmt.Errorf("source currency (--from) is required"))
		}
		if txToCurrency == "" {
			exitWithError(fmt.Errorf("target currency (--to) is required"))
		}
		if txAmountStr == "" {
			exitWithError(fmt.Errorf("amount (--amount) is required"))
		}

		fromCurrency := shared.Currency(txFromCurrency)
		if !isValidCurrency(fromCurrency) {
			exitWithError(fmt.Errorf("invalid source currency code: %q. Supported: USD, EUR, GBP", fromCurrency))
		}
		toCurrency := shared.Currency(txToCurrency)
		if !isValidCurrency(toCurrency) {
			exitWithError(fmt.Errorf("invalid target currency code: %q. Supported: USD, EUR, GBP", toCurrency))
		}
		if fromCurrency == toCurrency {
			exitWithError(fmt.Errorf("source and target currencies cannot be the same"))
		}

		amount, err := decimal.NewFromString(txAmountStr)
		if err != nil {
			exitWithError(fmt.Errorf("invalid amount format: %q. %v", txAmountStr, err))
		}
		if amount.IsNegative() || amount.IsZero() {
			exitWithError(fmt.Errorf("conversion amount must be positive: %s", amount))
		}

		convertCmdInput := app.ConvertCurrencyCommand{
			AccountID:    txAccountID,
			FromAmount:   amount,
			FromCurrency: fromCurrency,
			ToCurrency:   toCurrency,
		}

		err = accountService.ConvertCurrency(convertCmdInput)
		if err != nil {
			// Handle insufficient funds specifically if desired
			// if errors.Is(err, domain.ErrInsufficientFunds) { ... }
			exitWithError(fmt.Errorf("failed to convert currency: %w", err))
		}

		// Note: The actual converted amount isn't directly returned by the service call.
		// We could query the balance afterwards to show the result, but for simplicity,
		// we just confirm the operation was initiated.
		fmt.Printf("Successfully initiated currency conversion of %s %s to %s for account '%s'.\n",
			amount.StringFixed(2), fromCurrency, toCurrency, txAccountID)
	},
}

// transferCmd represents the transfer command
var transferCmd = &cobra.Command{
	Use:   "transfer",
	Short: "Transfer funds between two accounts",
	Long: `Initiates a transfer by debiting the specified amount and currency from the source account.
Note: This currently only performs the debit part of the transfer.
A separate process would be needed to handle the corresponding credit.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Validate required flags
		if txFromID == "" {
			exitWithError(fmt.Errorf("source account ID (--from-id) is required"))
		}
		if txToID == "" {
			exitWithError(fmt.Errorf("target account ID (--to-id) is required"))
		}
		if txCurrency == "" {
			exitWithError(fmt.Errorf("currency (--currency) is required"))
		}
		if txAmountStr == "" {
			exitWithError(fmt.Errorf("amount (--amount) is required"))
		}
		if txFromID == txToID {
			exitWithError(fmt.Errorf("source and target account IDs cannot be the same"))
		}

		currency := shared.Currency(txCurrency)
		if !isValidCurrency(currency) {
			exitWithError(fmt.Errorf("invalid currency code: %q. Supported: USD, EUR, GBP", currency))
		}

		amount, err := decimal.NewFromString(txAmountStr)
		if err != nil {
			exitWithError(fmt.Errorf("invalid amount format: %q. %v", txAmountStr, err))
		}
		if amount.IsNegative() || amount.IsZero() {
			exitWithError(fmt.Errorf("transfer amount must be positive: %s", amount))
		}

		transferCmdInput := app.TransferMoneyCommand{
			SourceAccountID: txFromID,
			TargetAccountID: txToID,
			Amount:          amount,
			Currency:        currency,
		}

		err = accountService.TransferMoney(transferCmdInput)
		if err != nil {
			// Handle specific errors like insufficient funds or target account not found
			// if errors.Is(err, domain.ErrInsufficientFunds) { ... }
			// if errors.Is(err, domain.ErrAccountNotFound) { ... } // Check if target exists
			exitWithError(fmt.Errorf("failed to initiate transfer: %w", err))
		}

		fmt.Printf("Successfully initiated transfer (debit) of %s %s from account '%s' to account '%s'.\n",
			amount.StringFixed(2), currency, txFromID, txToID)
	},
}

// Helper function for currency validation
func isValidCurrency(c shared.Currency) bool {
	cUpper := shared.Currency(strings.ToUpper(string(c)))
	return cUpper == shared.USD || cUpper == shared.EUR || cUpper == shared.GBP
}

func init() {
	// Add transactionCmd to root command
	rootCmd.AddCommand(transactionCmd)

	// Add depositCmd to transactionCmd
	transactionCmd.AddCommand(depositCmd)

	// Define flags for depositCmd
	depositCmd.Flags().StringVar(&txAccountID, "id", "", "Account ID to deposit into (required)")
	depositCmd.Flags().StringVar(&txCurrency, "currency", "", "Currency code (USD, EUR, GBP) (required)")
	depositCmd.Flags().StringVar(&txAmountStr, "amount", "", "Amount to deposit (required)")
	_ = depositCmd.MarkFlagRequired("id") // Mark flags as required for better UX
	_ = depositCmd.MarkFlagRequired("currency")
	_ = depositCmd.MarkFlagRequired("amount")

	// Add withdrawCmd to transactionCmd
	transactionCmd.AddCommand(withdrawCmd)

	// Define flags for withdrawCmd (reusing variables, but attaching to withdrawCmd)
	// Note: Cobra allows reusing flag variables if they are attached to different commands,
	// but it's often clearer to use distinct variables per command if logic differs significantly.
	// Here, the flags are identical, so reuse is acceptable.
	withdrawCmd.Flags().StringVar(&txAccountID, "id", "", "Account ID to withdraw from (required)")
	withdrawCmd.Flags().StringVar(&txCurrency, "currency", "", "Currency code (USD, EUR, GBP) (required)")
	withdrawCmd.Flags().StringVar(&txAmountStr, "amount", "", "Amount to withdraw (required)")
	_ = withdrawCmd.MarkFlagRequired("id")
	_ = withdrawCmd.MarkFlagRequired("currency")
	_ = withdrawCmd.MarkFlagRequired("amount")

	// Add convertCmd to transactionCmd
	transactionCmd.AddCommand(convertCmd)

	// Define flags for convertCmd
	convertCmd.Flags().StringVar(&txAccountID, "id", "", "Account ID for the conversion (required)")
	convertCmd.Flags().StringVar(&txFromCurrency, "from", "", "Source currency code (USD, EUR, GBP) (required)")
	convertCmd.Flags().StringVar(&txToCurrency, "to", "", "Target currency code (USD, EUR, GBP) (required)")
	convertCmd.Flags().StringVar(&txAmountStr, "amount", "", "Amount in source currency to convert (required)")
	_ = convertCmd.MarkFlagRequired("id")
	_ = convertCmd.MarkFlagRequired("from")
	_ = convertCmd.MarkFlagRequired("to")
	_ = convertCmd.MarkFlagRequired("amount")

	// Add transferCmd to transactionCmd
	transactionCmd.AddCommand(transferCmd)

	// Define flags for transferCmd
	transferCmd.Flags().StringVar(&txFromID, "from-id", "", "Source account ID (required)")
	transferCmd.Flags().StringVar(&txToID, "to-id", "", "Target account ID (required)")
	transferCmd.Flags().StringVar(&txCurrency, "currency", "", "Currency code (USD, EUR, GBP) (required)")
	transferCmd.Flags().StringVar(&txAmountStr, "amount", "", "Amount to transfer (required)")
	_ = transferCmd.MarkFlagRequired("from-id")
	_ = transferCmd.MarkFlagRequired("to-id")
	_ = transferCmd.MarkFlagRequired("currency")
	_ = transferCmd.MarkFlagRequired("amount")

}
