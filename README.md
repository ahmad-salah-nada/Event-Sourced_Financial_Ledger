# Event-Sourced Multi-Currency Financial Ledger (Go)

This project implements a financial ledger system using the **Event Sourcing** pattern in Go. It manages user accounts with balances in multiple currencies (USD, EUR, GBP), supports various financial transactions, and allows for state reconstruction by replaying events.

## Core Concepts

*   **Event Sourcing**: Instead of storing the current state of an account, the system stores a sequence of immutable events that represent every change made to the account. The current state is derived by replaying these events.
*   **Aggregate Root**: The `Account` entity acts as the aggregate root, encapsulating state and enforcing business rules.
*   **Commands & Events**: User intentions are captured as Commands (e.g., `DepositMoneyCommand`), which, upon successful validation, generate Events (e.g., `DepositMadeEvent`) that are persisted.
*   **Snapshots**: To optimize state reconstruction for accounts with long histories, the system periodically saves snapshots of the account's state.

## Features

*   **Account Management**: Create accounts with initial balances in multiple currencies.
*   **Transactions**:
    *   **Deposit**: Add funds to a specific currency balance.
    *   **Withdraw**: Remove funds from a specific currency balance (with insufficient funds check).
    *   **Currency Conversion**: Convert funds between currencies within the same account using exchange rates.
    *   **Money Transfer**: Initiate a transfer of funds from one account to another (currently implements the debit part; a full Saga/Process Manager would handle the credit).
*   **Querying**:
    *   Get current account balances (all or specific currency).
    *   Get transaction history (full or paginated event stream).
*   **State Reconstruction**: Rebuild account state from events and snapshots.
*   **Snapshotting**: Automatically creates snapshots at configurable intervals (`SnapshotFrequency`).
*   **Persistence**: Includes simple in-memory implementations for `EventStore` and `SnapshotStore` for demonstration.

## Project Structure

*   `main.go`: Entry point and CLI simulation driver. Demonstrates various operations.
*   `app/`: Application layer (Service, Commands, Queries). Orchestrates use cases.
*   `domain/`: Core domain logic (Aggregate Root `Account`, Value Objects `Money`, `Snapshot`, domain errors).
*   `events/`: Event definitions (interface, base event, specific event types).
*   `store/`: Persistence interfaces (`EventStore`, `SnapshotStore`) and in-memory implementations.
*   `shared/`: Common types used across layers (e.g., `Currency`, `Balance`).
*   `DESIGN_DOC.md`: Detailed design document explaining the architecture and implementation choices.

## Getting Started

### Prerequisites

*   Go (version 1.24 or later recommended)

### Build & Run

1.  **Clone the repository** (or ensure you are in the project's root directory).
2.  **Option 1: Build and Run**
    *   Build the executable:
        ```bash
        go build -o financial-ledger-cli main.go
        ```
    *   Run the simulation:
        ```bash
        ./financial-ledger-cli
        ```
3.  **Option 2: Run Directly**
    *   Run the simulation directly using `go run`:
        ```bash
        go run main.go
        ```
    Both run methods will execute the sequence of operations defined in `main.go` and print logs/output to the console, demonstrating account creation, transactions, queries, and snapshotting.

### Testing

Run all unit tests in the project:

```bash
go test ./...
```

You can add the `-v` flag for verbose output:

```bash
go test -v ./...
```

## Design

For a detailed explanation of the system's design, architecture, event flow, state reconstruction, and snapshotting strategy, please refer to the [DESIGN_DOC.md](DESIGN_DOC.md) file.
