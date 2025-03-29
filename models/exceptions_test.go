package models

import (
	"testing"
)

func TestEnforceError_Error(t *testing.T) {
	message := "test error message"
	err := &EnforceError{Message: message}

	if err.Error() != message {
		t.Errorf("expected %s, got %s", message, err.Error())
	}
}

func TestNewEnforceError(t *testing.T) {
	message := "new enforce error"
	err := NewEnforceError(message)

	enforceErr, ok := err.(*EnforceError)
	if !ok {
		t.Errorf("expected *EnforceError, got %T", err)
	}

	if enforceErr.Message != message {
		t.Errorf("expected %s, got %s", message, enforceErr.Message)
	}
}
