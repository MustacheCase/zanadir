package models

type EnforceError struct {
	Message string
}

func (e *EnforceError) Error() string {
	return e.Message
}

func NewEnforceError(message string) error {
	return &EnforceError{Message: message}
}
