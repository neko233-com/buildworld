package auth

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// VerifyTOTP verifies a six-digit RFC 6238 code and returns the matched
// 30-second counter. Callers must atomically consume the counter per user.
func VerifyTOTP(secret, code string, now time.Time) (int64, bool) {
	if len(code) != 6 {
		return 0, false
	}
	for _, ch := range code {
		if ch < '0' || ch > '9' {
			return 0, false
		}
	}
	encoded := strings.TrimRight(strings.ToUpper(strings.TrimSpace(secret)), "=")
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(encoded)
	if err != nil || len(key) < 16 {
		return 0, false
	}
	current := now.Unix() / 30
	for _, offset := range []int64{0, -1, 1} {
		counter := current + offset
		if counter < 0 {
			continue
		}
		var message [8]byte
		binary.BigEndian.PutUint64(message[:], uint64(counter))
		mac := hmac.New(sha1.New, key)
		_, _ = mac.Write(message[:])
		digest := mac.Sum(nil)
		position := int(digest[len(digest)-1] & 0x0f)
		value := binary.BigEndian.Uint32(digest[position:position+4]) & 0x7fffffff
		expected := fmt.Sprintf("%06d", value%1_000_000)
		if subtle.ConstantTimeCompare([]byte(code), []byte(expected)) == 1 {
			return counter, true
		}
	}
	return 0, false
}
