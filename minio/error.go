package minio

import "fmt"

// NewResourceError creates a new error with the given msg argument.
func NewResourceError(msg string, resource string, err error) error {
	return &Error{
		Message: fmt.Sprintf("[FATAL] %s (%s): %s", msg, resource, err),
	}
}

func (e *Error) Error() string {
	return e.Message
}

// // NewPolicyError creates a new error with the given msg argument.
// func NewPolicyError(msg string, resource string, err error) error {
// 	return &Error{
// 		Message: fmt.Sprintf("[FATAL] %s (%s): %s", msg, resource, err),
// 	}
// }
