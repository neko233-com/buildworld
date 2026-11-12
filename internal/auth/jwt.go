package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID         int64  `json:"user_id"`
	Role           string `json:"role"`
	SessionVersion int64  `json:"session_version"`
	MFA            bool   `json:"mfa,omitempty"`
	MFAKeyID       string `json:"mfa_key_id,omitempty"`
	jwt.RegisteredClaims
}

type JWT struct {
	secret []byte
}

func NewJWT(secret string) *JWT {
	return &JWT{secret: []byte(secret)}
}

func (j *JWT) Generate(userID int64, role string, sessionVersion int64, duration time.Duration) (string, error) {
	return j.generate(userID, role, sessionVersion, duration, "")
}

// GenerateMFA issues a session after successful second-factor verification.
func (j *JWT) GenerateMFA(userID int64, role string, sessionVersion int64, duration time.Duration, totpSecret string) (string, error) {
	if strings.TrimSpace(totpSecret) == "" {
		return "", fmt.Errorf("missing MFA secret")
	}
	return j.generate(userID, role, sessionVersion, duration, totpSecret)
}

func (j *JWT) generate(userID int64, role string, sessionVersion int64, duration time.Duration, totpSecret string) (string, error) {
	claims := &Claims{
		UserID:         userID,
		Role:           role,
		SessionVersion: sessionVersion,
		MFA:            totpSecret != "",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	if totpSecret != "" {
		claims.MFAKeyID = j.mfaKeyID(userID, totpSecret)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.secret)
}

// MatchesMFASecret invalidates sessions when the user's configured TOTP secret changes.
func (j *JWT) MatchesMFASecret(claims *Claims, totpSecret string) bool {
	if j == nil || claims == nil || !claims.MFA || claims.MFAKeyID == "" || strings.TrimSpace(totpSecret) == "" {
		return false
	}
	expected := j.mfaKeyID(claims.UserID, totpSecret)
	return subtle.ConstantTimeCompare([]byte(claims.MFAKeyID), []byte(expected)) == 1
}

func (j *JWT) mfaKeyID(userID int64, secret string) string {
	mac := hmac.New(sha256.New, j.secret)
	_, _ = fmt.Fprintf(mac, "buildworld-mfa-key-v1/%d/%s", userID, strings.TrimRight(strings.ToUpper(strings.TrimSpace(secret)), "="))
	return hex.EncodeToString(mac.Sum(nil))
}

func (j *JWT) Validate(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return j.secret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}
