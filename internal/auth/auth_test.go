package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("s3cret")
	require.NoError(t, err)
	assert.NotEqual(t, "s3cret", hash)

	assert.True(t, CheckPassword(hash, "s3cret"))
	assert.False(t, CheckPassword(hash, "S3cret"))
	assert.False(t, CheckPassword("not-a-hash", "s3cret"))
}

func TestHashPassword_TooLong(t *testing.T) {
	_, err := HashPassword(strings.Repeat("x", MaxPasswordLength+1))
	assert.Error(t, err)
}

func TestGenerateSecret(t *testing.T) {
	a, err := GenerateSecret()
	require.NoError(t, err)
	b, err := GenerateSecret()
	require.NoError(t, err)

	assert.Len(t, a, 64)
	assert.NotEqual(t, a, b)
}

func TestTokenManager_IssueAndParse(t *testing.T) {
	m := NewTokenManager("secret", time.Hour)

	token, err := m.Issue(42)
	require.NoError(t, err)

	userID, err := m.Parse(token)
	require.NoError(t, err)
	assert.Equal(t, int64(42), userID)
}

func TestTokenManager_Parse_Errors(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	newManager := func(secret string, ttl time.Duration, now time.Time) *TokenManager {
		m := NewTokenManager(secret, ttl)
		m.now = func() time.Time { return now }
		return m
	}

	tests := []struct {
		name  string
		token func(t *testing.T) string
		mgr   *TokenManager
	}{
		{
			name:  "malformed",
			token: func(*testing.T) string { return "not.a.token" },
			mgr:   newManager("secret", time.Hour, base),
		},
		{
			name: "wrong secret",
			token: func(t *testing.T) string {
				tok, err := newManager("other", time.Hour, base).Issue(1)
				require.NoError(t, err)
				return tok
			},
			mgr: newManager("secret", time.Hour, base),
		},
		{
			name: "expired",
			token: func(t *testing.T) string {
				tok, err := newManager("secret", time.Minute, base.Add(-2*time.Minute)).Issue(1)
				require.NoError(t, err)
				return tok
			},
			mgr: newManager("secret", time.Minute, base),
		},
		{
			name: "no expiration claim",
			token: func(t *testing.T) string {
				tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{Subject: "1"}).
					SignedString([]byte("secret"))
				require.NoError(t, err)
				return tok
			},
			mgr: newManager("secret", time.Hour, base),
		},
		{
			name: "unsigned algorithm",
			token: func(t *testing.T) string {
				tok, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{
					Subject:   "1",
					ExpiresAt: jwt.NewNumericDate(base.Add(time.Hour)),
				}).SignedString(jwt.UnsafeAllowNoneSignatureType)
				require.NoError(t, err)
				return tok
			},
			mgr: newManager("secret", time.Hour, base),
		},
		{
			name: "non-numeric subject",
			token: func(t *testing.T) string {
				tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
					Subject:   "alice",
					ExpiresAt: jwt.NewNumericDate(base.Add(time.Hour)),
				}).SignedString([]byte("secret"))
				require.NoError(t, err)
				return tok
			},
			mgr: newManager("secret", time.Hour, base),
		},
		{
			name: "non-positive subject",
			token: func(t *testing.T) string {
				tok, err := newManager("secret", time.Hour, base).Issue(0)
				require.NoError(t, err)
				return tok
			},
			mgr: newManager("secret", time.Hour, base),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID, err := tt.mgr.Parse(tt.token(t))
			assert.ErrorIs(t, err, ErrInvalidToken)
			assert.Zero(t, userID)
		})
	}
}
