package cmd

import (
	"bufio" // Added for REPL input
	"fmt"
	"os"
	"strings" // Added for REPL input processing

	"financial-ledger/app"
	"financial-ledger/store"

	"github.com/spf13/cobra"
)

var (
	// Shared application service instance
	accountService *app.AccountService
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "ledger-cli",
	Short: "A CLI for interacting with the event-sourced financial ledger",
	Long: `ledger-cli is a command-line interface to manage accounts and transactions
in the event-sourced financial ledger system.

It allows creating accounts, performing deposits, withdrawals,
currency conversions, transfers, and querying account balances and history.`,
	// Run: func(cmd *cobra.Command, args []string) { }, // No action for root command itself
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Initialize shared services here
	// Using in-memory stores as per the original design
	eventStore := store.NewInMemoryEventStore()
	snapshotStore := store.NewInMemorySnapshotStore()
	accountService = app.NewAccountService(eventStore, snapshotStore)

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	// rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	// Add subcommands here (will be done in other files)
	// e.g., rootCmd.AddCommand(accountCmd)
	// e.g., rootCmd.AddCommand(transactionCmd)
	// e.g., rootCmd.AddCommand(queryCmd)
	rootCmd.AddCommand(replCmd) // Add the repl command
}

// Helper function to print errors and exit
func exitWithError(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	// In REPL mode, we don't exit the process on command error
	// We just print the error and continue the loop.
	// We can check if we are in REPL mode by checking a flag or context.
	// For simplicity now, we'll just print and return.
	// A more robust solution might use a context.Context or a global flag.
	// For this simple implementation, we'll just return after printing.
}

// replCmd represents the repl command
var replCmd = &cobra.Command{
	Use:   "repl",
	Short: "Start an interactive REPL session",
	Long:  `Starts an interactive Read-Eval-Print Loop session to interact with the ledger.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Starting ledger CLI REPL. Type 'exit' or 'quit' to exit.")

		reader := bufio.NewReader(os.Stdin)

		for {
			fmt.Print("> ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)

			if input == "exit" || input == "quit" {
				break
			}

			if input == "" {
				continue
			}

			// Split input into args, similar to how the OS shell does it
			// This is a simple split, a real shell parser would be more complex
			commandArgs := strings.Fields(input)

			// Execute the command using the root command
			// We need to temporarily set os.Args to the command and its args
			// and then restore it. This is a bit hacky but works with Cobra.
			// A better approach might involve using Cobra's ExecuteC or similar
			// with a custom input stream, but this is simpler for a basic REPL.
			originalArgs := os.Args
			os.Args = append([]string{originalArgs[0]}, commandArgs...) // Prepend program name

			// Execute the command. Errors will be printed by exitWithError.
			// We capture the output to avoid exiting the process on command errors.
			// This requires modifying Cobra's output streams, which is more complex.
			// For this simple REPL, we'll rely on exitWithError not actually exiting
			// when called from the REPL context (as modified above).
			// A more robust REPL would handle command execution errors gracefully
			// without relying on modifying exitWithError's behavior.

			// Execute the command. Errors will be printed by exitWithError.
			// We don't need to check the return value here because exitWithError
			// handles the error reporting.
			rootCmd.Execute()

			// Restore original os.Args
			os.Args = originalArgs
		}

		fmt.Println("Exiting REPL.")
	},
}
