package auth

import (
	"testing"
	"time"
)

func TestVerifyTOTPRFC6238AndWindow(t *testing.T) {
	const secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	// RFC 6238 SHA-1 test vector 94287082, truncated to six digits.
	if counter, ok := VerifyTOTP(secret, "287082", time.Unix(59, 0)); !ok || counter != 1 {
		t.Fatalf("RFC vector rejected: counter=%d ok=%v", counter, ok)
	}
	if _, ok := VerifyTOTP(secret, "287082", time.Unix(120, 0)); ok {
		t.Fatal("expired TOTP accepted")
	}
	for _, input := range []struct{ secret, code string }{{"SHORT", "287082"}, {secret, "28708x"}, {secret, "2870820"}} {
		if _, ok := VerifyTOTP(input.secret, input.code, time.Unix(59, 0)); ok {
			t.Fatalf("invalid input accepted: %+v", input)
		}
	}
}
