package util

import (
	"crypto/sha256"
	"fmt"
)

// GenerateETag generates an ETag string from content using SHA256 hash.
// The ETag is wrapped in double quotes as per HTTP specification.
func GenerateETag(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf(`"%x"`, hash)
}

// NotModifiedResult represents a 304 Not Modified response
type NotModifiedResult struct {
	StatusCode int
	Headers    map[string]string
}

// CheckIfNotModified checks if the request's If-None-Match header matches the given ETag.
// Returns a 304 Not Modified result if matched, and a boolean indicating if it matched.
// The match supports weak ETags (W/"etag") and wildcard (*) as per HTTP spec.
func CheckIfNotModified(reqHeaders map[string]string, etag string) (*NotModifiedResult, bool) {
	reqETag := reqHeaders["If-None-Match"]
	if reqETag == "" {
		return nil, false
	}

	// Support weak ETags (W/"etag") and wildcard (*)
	if reqETag == etag || reqETag == "W/"+etag || reqETag == "*" {
		return &NotModifiedResult{
			StatusCode: 304,
			Headers: map[string]string{
				"ETag": etag,
			},
		}, true
	}

	return nil, false
}

// GenerateETagAndCheck checks If-None-Match, generates ETag, and returns appropriate response.
// This is a convenience function that combines both operations.
func GenerateETagAndCheck(data []byte, reqHeaders map[string]string) (*NotModifiedResult, string) {
	etag := GenerateETag(data)

	if result, matched := CheckIfNotModified(reqHeaders, etag); matched {
		return result, etag
	}

	return nil, etag
}
