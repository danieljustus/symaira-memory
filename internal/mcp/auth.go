package mcp

import (
	"context"
	"net/http"
	"strings"

	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/security"
)

type contextKey string

const payloadKey contextKey = "jwt_payload"

func contextWithPayload(ctx context.Context, p *security.JWTPayload) context.Context {
	return context.WithValue(ctx, payloadKey, p)
}

func payloadFromContext(ctx context.Context) *security.JWTPayload {
	p, _ := ctx.Value(payloadKey).(*security.JWTPayload)
	return p
}

// AuthMiddleware handles JWT authentication and role-based access control.
type AuthMiddleware struct {
	jwts    *security.JWTProvider
	db      *db.DB
	profile *db.Profile
}

// NewAuthMiddleware creates an auth middleware with the given dependencies.
func NewAuthMiddleware(jwts *security.JWTProvider, database *db.DB) *AuthMiddleware {
	return &AuthMiddleware{jwts: jwts, db: database}
}

// SetProfile sets the active profile for role-based access control.
func (a *AuthMiddleware) SetProfile(p *db.Profile) {
	a.profile = p
}

// RequireAuth returns a middleware that requires a valid JWT token.
// If jwts is nil, authentication is skipped (open access).
func (a *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.jwts == nil {
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeJSONError(w, http.StatusUnauthorized, CodeForbidden, "missing or invalid Authorization header", nil)
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		payload, err := a.jwts.VerifyToken(token)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, CodeForbidden, "invalid or expired token", err)
			return
		}
		ctx := contextWithPayload(r.Context(), payload)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole returns a middleware that requires a valid JWT with at least the given role.
func (a *AuthMiddleware) RequireRole(minRole security.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			payload := payloadFromContext(r.Context())
			if payload == nil {
				writeJSONError(w, http.StatusUnauthorized, CodeForbidden, "authentication required", nil)
				return
			}

			profile := a.profile
			if profile == nil && payload.Subject != "" && a.db != nil {
				p, err := a.db.GetProfileByName(payload.Subject)
				if err == nil {
					profile = p
				}
			}

			if profile != nil {
				if !security.ParseRole(profile.Role).CanWrite() && minRole == security.RoleReadWrite {
					writeJSONError(w, http.StatusForbidden, CodeForbidden, "insufficient permissions: read-only profile", nil)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
