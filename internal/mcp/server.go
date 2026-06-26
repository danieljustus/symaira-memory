package mcp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/security"
)

// Standard HTTP error codes for API responses.
const (
	CodeInvalidRequest = "INVALID_REQUEST"
	CodeNotFound       = "NOT_FOUND"
	CodeForbidden      = "FORBIDDEN"
	CodeInternal       = "INTERNAL_ERROR"
	CodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
)

type Server struct {
	db             *db.DB
	extractor      *extractor.PatternExtractor
	embeddings     *extractor.EmbeddingsGenerator
	jwts           *security.JWTProvider
	allowedOrigins []string
	piiEnabled     bool
	version        string
	cfg            *config.Config
	profile        *db.Profile
	rateLimiter    *RateLimiter
}

func NewServer(database *db.DB, jwtProvider *security.JWTProvider, version string, cfg *config.Config) *Server {
	return &Server{
		db:             database,
		extractor:      extractor.NewPatternExtractor(),
		embeddings:     extractor.NewEmbeddingsGenerator(cfg),
		jwts:           jwtProvider,
		allowedOrigins: []string{"chrome-extension://*", "moz-extension://*"},
		piiEnabled:     true,
		version:        version,
		cfg:            cfg,
		rateLimiter:    NewRateLimiter(DefaultRateLimitConfig(), cfg.Security.TrustedProxies...),
	}
}

func (s *Server) SetPIIEnabled(enabled bool) {
	s.piiEnabled = enabled
}

func (s *Server) SetAllowedOrigins(origins []string) {
	s.allowedOrigins = origins
}

func (s *Server) SetProfile(p *db.Profile) {
	s.profile = p
}

// writeJSONError sends a standardized JSON error response with an error code.
// The safeMsg is safe for client exposure; internal is logged to stderr only.
func writeJSONError(w http.ResponseWriter, status int, code string, safeMsg string, internal error) {
	if internal != nil {
		slog.Error("HTTP error", "code", code, "msg", safeMsg, "err", internal)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": safeMsg,
		"code":  code,
	})
}

func mcpError(safeMsg string, internal error) (any, error) {
	if internal != nil {
		slog.Error("MCP error", "msg", safeMsg, "err", internal)
	}
	return nil, fmt.Errorf("%s", safeMsg)
}
