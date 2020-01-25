package madmin

import (
	"encoding/xml"
	"fmt"
	"net/http"
)

/* **** SAMPLE ERROR RESPONSE ****
<?xml version="1.0" encoding="UTF-8"?>
<Error>
   <Code>AccessDenied</Code>
   <Message>Access Denied</Message>
   <BucketName>bucketName</BucketName>
   <Key>objectName</Key>
   <RequestId>F19772218238A85A</RequestId>
   <HostId>GuWkjyviSiGHizehqpmsD1ndz5NClSP19DOT+s2mv7gXGQ8/X1lhbDGiIJEXpGFD</HostId>
</Error>
*/

// ErrorResponse - Is the typed error returned by all API operations.
type ErrorResponse struct {
	XMLName    xml.Name `xml:"Error,omitempty" json:"Error,omitempty"`
	Code       string   `xml:"Code,omitempty" json:"Code,omitempty"`
	Message    string   `xml:"Message,omitempty" json:"Message,omitempty"`
	BucketName string   `xml:"BucketName,omitempty" json:"BucketName,omitempty"`
	Key        string   `xml:"Key,omitempty" json:"Key,omitempty"`
	RequestID  string   `xml:"RequestId,omitempty" json:"RequestID,omitempty"`
	HostID     string   `xml:"HostId,omitempty" json:"HostId,omitempty"`

	// Region where the bucket is located. This header is returned
	// only in HEAD bucket and ListObjects response.
	Region string `xml:"Region,omitempty" json:"Region,omitempty"`
}

// Error - Returns HTTP error string
func (e ErrorResponse) Error() string {
	return fmt.Sprintf("{\"Code\":\"%s\",\"Message\":\"%s\",\"BucketName\":\"%s\",\"Region\":\"%s\"}", e.Code, e.Message, e.BucketName, e.Region)
}

const (
	reportIssue = "Please report this issue at https://github.com/minio/minio/issues."
)

// httpRespToErrorResponse returns a new encoded ErrorResponse
// structure as error.
func httpRespToErrorResponse(resp *http.Response) error {
	if resp == nil {
		msg := "Response is empty. " + reportIssue
		return ErrInvalidArgument(msg)
	}
	var errResp ErrorResponse
	// Decode the json error
	err := jsonDecoder(resp.Body, &errResp)
	if err != nil {
		return ErrorResponse{
			Code:    resp.Status,
			Message: "Failed to parse server response.",
		}
	}
	closeResponse(resp)
	return errResp
}

// ToErrorResponse - Returns parsed ErrorResponse struct from body and
// http headers.
//
// For example:
//
//   import admin "github.com/minio/minio/pkg/madmin"
//   ...
//   ...
//   ss, err := adm.ServiceStatus(...)
//   if err != nil {
//      resp := admin.ToErrorResponse(err)
//   }
//   ...
func ToErrorResponse(err error) ErrorResponse {
	switch err := err.(type) {
	case ErrorResponse:
		return err
	default:
		return ErrorResponse{}
	}
}

// ErrInvalidArgument - Invalid argument response.
func ErrInvalidArgument(message string) error {
	return ErrorResponse{
		Code:      "InvalidArgument",
		Message:   message,
		RequestID: "minio",
	}
}
