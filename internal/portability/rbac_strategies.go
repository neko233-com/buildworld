package portability

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/store"
)

// userStrategy migrates BuildWorld accounts. Password hashes are intentionally
// NEVER exported: a bundle cannot carry recoverable credentials, so imported
// users receive a fresh random password and must reset it through the normal
// account-recovery flow. Only the identity and role are transferred.
type userStrategy struct{ store *store.Store }

func (userStrategy) Capability() Capability {
	return Capability{
		Key:         "users",
		Version:     1,
		Description: "User accounts and roles (passwords are not exported; imported users get a reset password)",
	}
}

type userRecord struct {
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	AvatarURL string    `json:"avatar_url,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

func (s userStrategy) Export(_ context.Context, _ ExportOptions) (json.RawMessage, error) {
	users, err := s.store.ListUsers()
	if err != nil {
		return nil, err
	}
	records := make([]userRecord, 0, len(users))
	for _, u := range users {
		records = append(records, userRecord{
			Username:  u.Username,
			Email:     u.Email,
			Role:      u.Role,
			AvatarURL: u.AvatarURL,
			CreatedAt: u.CreatedAt,
		})
	}
	return json.Marshal(records)
}

func (userStrategy) Inspect(data json.RawMessage) (int, error) {
	var records []userRecord
	err := json.Unmarshal(data, &records)
	return len(records), err
}

func (s userStrategy) Import(_ context.Context, data json.RawMessage, options ImportOptions) (SectionResult, error) {
	var records []userRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return SectionResult{}, err
	}
	result := SectionResult{Count: len(records)}
	for _, record := range records {
		if record.Username == "" {
			return result, fmt.Errorf("user record requires a username")
		}
		role := record.Role
		if role == "" {
			role = "user"
		}
		existing, err := s.store.GetUserByUsername(record.Username)
		switch {
		case err == nil && options.Mode == "skip":
			result.Skipped++
		case err == nil:
			if err := s.store.UpdateUserRole(existing.ID, role); err != nil {
				return result, err
			}
			if err := s.store.UpdateUserProfile(existing.ID, record.Email, record.AvatarURL); err != nil {
				return result, err
			}
			result.Updated++
		case errors.Is(err, sql.ErrNoRows):
			hash, herr := auth.HashPassword(generateResetPassword())
			if herr != nil {
				return result, fmt.Errorf("hash reset password for %q: %w", record.Username, herr)
			}
			if _, cerr := s.store.CreateUser(record.Username, record.Email, hash, role); cerr != nil {
				return result, fmt.Errorf("create user %q: %w", record.Username, cerr)
			}
			result.Created++
		default:
			return result, err
		}
	}
	return result, nil
}

// apiTokenStrategy migrates API token grants. The original token secret is
// hashed at rest and cannot be recovered, so the bundle stores only the
// observable grant (owner, name, scopes, expiry). On import each grant is
// re-issued with a brand-new secret, and the plaintext is returned exactly
// once through SectionResult.Issued for the operator to capture.
type apiTokenStrategy struct{ store *store.Store }

func (apiTokenStrategy) Capability() Capability {
	return Capability{
		Key:         "api_tokens",
		Version:     1,
		Description: "API token grants (re-issued with new secrets on import)",
		DependsOn:   []string{"users"},
	}
}

type apiTokenRecord struct {
	Username  string     `json:"username"`
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at,omitempty"`
}

func (s apiTokenStrategy) Export(_ context.Context, _ ExportOptions) (json.RawMessage, error) {
	users, err := s.store.ListUsers()
	if err != nil {
		return nil, err
	}
	records := make([]apiTokenRecord, 0)
	for _, u := range users {
		tokens, err := s.store.ListAPITokens(u.ID)
		if err != nil {
			return nil, err
		}
		for _, t := range tokens {
			var scopes []string
			if t.Scopes != "" {
				_ = json.Unmarshal([]byte(t.Scopes), &scopes)
			}
			records = append(records, apiTokenRecord{
				Username:  u.Username,
				Name:      t.Name,
				Scopes:    scopes,
				ExpiresAt: t.ExpiresAt,
				CreatedAt: t.CreatedAt,
			})
		}
	}
	return json.Marshal(records)
}

func (apiTokenStrategy) Inspect(data json.RawMessage) (int, error) {
	var records []apiTokenRecord
	err := json.Unmarshal(data, &records)
	return len(records), err
}

func (s apiTokenStrategy) Import(_ context.Context, data json.RawMessage, options ImportOptions) (SectionResult, error) {
	var records []apiTokenRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return SectionResult{}, err
	}
	result := SectionResult{Count: len(records)}
	for _, record := range records {
		if record.Username == "" || record.Name == "" {
			return result, fmt.Errorf("api token record requires username and name")
		}
		owner, err := s.store.GetUserByUsername(record.Username)
		if err != nil {
			// The referenced user was not migrated; skip rather than fail the
			// whole bundle, but record it so the operator can see the gap.
			result.Skipped++
			continue
		}
		scopesJSON, err := json.Marshal(record.Scopes)
		if err != nil {
			return result, fmt.Errorf("encode scopes for %q/%q: %w", record.Username, record.Name, err)
		}
		existing, _ := s.store.ListAPITokens(owner.ID)
		var existingID int64 = -1
		for _, t := range existing {
			if t.Name == record.Name {
				existingID = t.ID
				break
			}
		}
		switch {
		case existingID >= 0 && options.Mode == "skip":
			result.Skipped++
			continue
		case existingID >= 0:
			if _, derr := s.store.DeleteAPITokenForUser(existingID, owner.ID); derr != nil {
				return result, fmt.Errorf("revoke existing token %q: %w", record.Name, derr)
			}
		}
		plain, hash, prefix := generateAPIToken()
		if _, cerr := s.store.CreateAPIToken(owner.ID, record.Name, hash, prefix, string(scopesJSON), record.ExpiresAt); cerr != nil {
			return result, fmt.Errorf("create token %q/%q: %w", record.Username, record.Name, cerr)
		}
		result.Created++
		result.Issued = append(result.Issued, IssuedSecret{Username: record.Username, Name: record.Name, Token: plain})
	}
	return result, nil
}

// generateResetPassword returns a cryptographically random 36-character string
// suitable as a one-time imported-user password that the account owner must
// change on first login.
func generateResetPassword() string {
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		// Fall back to a time-derived value only if the system RNG is broken.
		return fmt.Sprintf("bw-reset-%d", time.Now().UnixNano())
	}
	return "bw-reset-" + hex.EncodeToString(raw)
}

// generateAPIToken mirrors the server's token issuance format: a bw_ prefixed
// 32-hex-character secret with its SHA-256 hash and 11-character display prefix.
func generateAPIToken() (plain, hash, prefix string) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		raw = []byte(fmt.Sprintf("%d", time.Now().UnixNano()))
	}
	plain = "bw_" + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(plain))
	hash = hex.EncodeToString(sum[:])
	prefix = plain[:11]
	return plain, hash, prefix
}
