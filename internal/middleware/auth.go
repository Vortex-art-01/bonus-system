package middleware

import (
	"context"
	"net/http"
	"strings"
)

const CookieName = "auth_token"

const bearerPrefix = "Bearer "

type contextKey int

const userIDKey contextKey = iota

type TokenParser interface {
	Parse(token string) (int64, error)
}

func Auth(parser TokenParser) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}

			userID, err := parser.Parse(token)
			if err != nil {
				http.Error(w, "invalid authentication token", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), userID)))
		})
	}
}

func WithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

func UserIDFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(userIDKey).(int64)
	return userID, ok
}

func extractToken(r *http.Request) string {
	if header := strings.TrimSpace(r.Header.Get("Authorization")); header != "" {
		if len(header) > len(bearerPrefix) && strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
			return strings.TrimSpace(header[len(bearerPrefix):])
		}
		return header
	}
	if cookie, err := r.Cookie(CookieName); err == nil {
		return cookie.Value
	}
	return ""
}
