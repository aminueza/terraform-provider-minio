package minio

import (
	"encoding/json"
	"fmt"
)

// NewResourceError creates a new error with the given msg argument.
func NewResourceError(msg string, resource string, err error) error {
	return &Error{
		Message: fmt.Sprintf("[FATAL] %s (%s): %s", msg, resource, err),
	}
}

func (e *Error) Error() string {
	return e.Message
}

//ErrToMinioError parses error to Minio Error
func ErrToMinioError(err error) *ResponseError {
	errResp := &ResponseError{}

	json.Unmarshal([]byte(err.Error()), &errResp)

	return errResp
}

func minioErr(message string, id string, err error) error {
	errMinio := ErrToMinioError(err)

	return fmt.Errorf(message, id, errMinio.Message)
}

// func isMinioErr(code string, message string) (string, int) {
// 	for errorCode, valueCode := range ErrorCodes {
// 		if valueCode.Code == code && strings.Contains(valueCode.Description, message) {
// 			return fmt.Sprintf("{\"Code\":\"%s\",\"Message\":\"%s\"}", errorCode, valueCode.Description), valueCode.HTTPStatusCode

// 		}
// 	}
// 	return fmt.Sprintf("{\"Code\":\"%s\",\"Message\":\"%s\"}", code, message), http.StatusBadRequest
// }
