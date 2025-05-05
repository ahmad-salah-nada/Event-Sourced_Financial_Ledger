# `ledger-cli` Command-Line Interface (CLI) Documentation

This document outlines the usage of the `ledger-cli` tool for interacting with the bank system.

## CLI Commands

### Account Commands

- `ledger-cli account create --id <account-id> [--balance <currency>:<amount>...]`

  Creates a new account.

  - `--id`: Optional account identifier. If not specified, a UUID will be generated.
  - `--balance`: Optional, repeatable flag to set initial balances (e.g., `--balance USD:100.50 --balance EUR:50`). Uses `decimal` for precise amounts.

### Transaction Commands

- `ledger-cli transaction deposit --id <account-id> --currency <currency> --amount <amount>`

  Deposits funds into an account.

- `ledger-cli transaction withdraw --id <account-id> --currency <currency> --amount <amount>`

  Withdraws funds from an account, with a balance check.

- `ledger-cli transaction convert --id <account-id> --from <currency> --to <currency> --amount <amount>`

  Converts an amount between currencies within the same account using internal exchange rate logic.

- `ledger-cli transaction transfer --from-id <source-account-id> --to-id <target-account-id> --currency <currency> --amount <amount>`

  Debits the source account to initiate a transfer. Per design, this performs only the *debit* part of the transfer.

### Query Commands

- `ledger-cli query balance --id <account-id> [--currency <currency>]`

  Displays the balance(s) of an account. If `--currency` is not specified, shows all balances.

- `ledger-cli query history --id <account-id> [--skip <n>] [--limit <n>]`

  Retrieves the transaction history (event stream) for an account.

  - `--skip`, `--limit`: Optional flags for pagination.

### Interactive Mode

- `ledger-cli repl`

  Launches an interactive REPL (Read-Eval-Print Loop) environment for the CLI. This allows users to issue commands in a session-like interface for exploring or interacting with the bank system efficiently.
