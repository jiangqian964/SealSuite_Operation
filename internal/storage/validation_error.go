package storage

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

func NewValidationError(msg string) error {
	return &ValidationError{Message: msg}
}

func IsValidationError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*ValidationError)
	return ok
}
