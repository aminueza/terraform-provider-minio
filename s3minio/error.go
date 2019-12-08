package s3minio

import "fmt"

// NewBucketError creates a new error with the given msg argument.
func NewBucketError(msg string, bucket string) error {
	return &Error{
		Message: fmt.Sprintf("[FATAL] %s: %s", msg, bucket),
	}
}

func (e *Error) Error() string {
	return e.Message
}
