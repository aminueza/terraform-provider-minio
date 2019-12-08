package s3minio

import (
	"log"
	"strconv"
)

//ParseString parses a string to bool
func ParseString(s string) bool {
	debugbool, err := strconv.ParseBool(s)
	if err != nil {
		log.Print(err)
	}
	return debugbool
}
