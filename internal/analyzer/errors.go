package analyzer

import "fmt"

// UserError is a user-facing error with a hint on how to fix it.
type UserError struct {
	Message string
	Hint    string
}

func (e *UserError) Error() string {
	return fmt.Sprintf("%s\n\n%s", e.Message, e.Hint)
}
