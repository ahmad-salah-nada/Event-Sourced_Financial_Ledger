package domain

import "fmt"

type DomainError struct {
	message string
}

func NewDomainError(format string, args ...interface{}) *DomainError {
	return &DomainError{message: fmt.Sprintf(format, args...)}
}

func (e *DomainError) Error() string {
	return e.message
}

var (
	ErrInsufficientFunds = NewDomainError("insufficient funds")
	ErrAccountExists     = NewDomainError("account already exists")
	ErrAccountNotFound   = NewDomainError("account not found")
)
