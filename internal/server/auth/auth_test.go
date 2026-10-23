package auth

import (
	"encoding/hex"
	"testing"
	"time"
)

func TestPassword(t *testing.T) {
	h := HashPassword("correct horse")
	if !VerifyPassword("correct horse", h) {
		t.Fatal("valid password rejected")
	}
	if VerifyPassword("wrong", h) || VerifyPassword("correct horse", "garbage") {
		t.Fatal("invalid password accepted")
	}
}

// RFC 6238 appendix B test vectors (SHA-1, 8 digits truncated to the last 6).
func TestTOTPVectors(t *testing.T) {
	key, _ := hex.DecodeString("3132333435363738393031323334353637383930")
	cases := map[int64]string{59: "287082", 1111111109: "081804", 1234567890: "005924", 2000000000: "279037"}
	for ts, want := range cases {
		if got := totpCode(key, uint64(ts)/30); got != want {
			t.Errorf("t=%d: got %s want %s", ts, got, want)
		}
	}
}

func TestVerifyTOTP(t *testing.T) {
	secret := NewTOTPSecret()
	if !ValidTOTPSecret(secret) {
		t.Fatal("generated secret invalid")
	}
	key, _ := b32.DecodeString(secret)
	now := time.Unix(1_800_000_000, 0)
	code := totpCode(key, uint64(now.Unix())/30)
	c, ok := VerifyTOTP(secret, code, now.Add(25*time.Second))
	if !ok || c != uint64(now.Unix())/30 {
		t.Fatal("code within window rejected")
	}
	if _, ok := VerifyTOTP(secret, code, now.Add(5*time.Minute)); ok {
		t.Fatal("stale code accepted")
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(2, time.Minute)
	now := time.Now()
	l.Fail("a", now)
	if l.Blocked("a", now) {
		t.Fatal("blocked too early")
	}
	l.Fail("a", now)
	if !l.Blocked("a", now) || l.Blocked("b", now) {
		t.Fatal("limit not applied per key")
	}
	if l.Blocked("a", now.Add(2*time.Minute)) {
		t.Fatal("window did not expire")
	}
}
