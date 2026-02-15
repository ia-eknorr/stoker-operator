/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
