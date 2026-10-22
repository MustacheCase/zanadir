package models

// EnforceError is a scan that ran but failed the enforcement asked for.
type EnforceError struct {
	Message string
}

func (e *EnforceError) Error() string {
	return e.Message
}

// NewEnforceError returns an EnforceError with message.
func NewEnforceError(message string) error {
	return &EnforceError{Message: message}
}
