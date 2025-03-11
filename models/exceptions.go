package models

import "fmt"

type StrictError struct {
	Message string
}

func (e *StrictError) Error() string {
	return fmt.Sprintf("Strict error: %s", e.Message)
}

func NewStrictError(message string) error {
	return &StrictError{Message: message}
}
