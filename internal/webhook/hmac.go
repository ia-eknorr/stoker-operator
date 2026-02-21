package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

// ValidateHMAC verifies an X-Hub-Signature-256 header value against the payload.
// The signature must be in the format "sha256=<hex-encoded HMAC>".
// Uses crypto/subtle.ConstantTimeCompare to prevent timing oracle attacks.
func ValidateHMAC(payload []byte, signature, secret string) error {
	if secret == "" {
		return fmt.Errorf("HMAC secret is empty")
	}

	if !strings.HasPrefix(signature, "sha256=") {
		return fmt.Errorf("HMAC validation failed")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) != 1 {
		return fmt.Errorf("HMAC validation failed")
	}
	return nil
}
