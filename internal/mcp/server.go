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
	CodeInvalidRequest   = "INVALID_REQUEST"
	CodeNotFound         = "NOT_FOUND"
	CodeForbidden        = "FORBIDDEN"
	CodeInternal         = "INTERNAL_ERROR"
	CodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
)

type Server struct {
	service     *MemoryService
	auth        *AuthMiddleware
	cors        *CORSMiddleware
	jwts        *security.JWTProvider
	version     string
	cfg         *config.Config
	profile     *db.Profile
	rateLimiter *RateLimiter
}

func NewServer(database *db.DB, jwtProvider *security.JWTProvider, version string, cfg *config.Config) *Server {
	embeddings := extractor.NewEmbeddingsGenerator(cfg)
	service := NewMemoryService(database, embeddings, true)
	auth := NewAuthMiddleware(jwtProvider, database)
	cors := NewCORSMiddleware([]string{"chrome-extension://*", "moz-extension://*"})

	return &Server{
		service:     service,
		auth:        auth,
		cors:        cors,
		jwts:        jwtProvider,
		version:     version,
		cfg:         cfg,
		rateLimiter: NewRateLimiter(DefaultRateLimitConfig(), cfg.Security.TrustedProxies...),
	}
}

func (s *Server) SetPIIEnabled(enabled bool) {
	s.service.SetPIIEnabled(enabled)
}

func (s *Server) SetAllowedOrigins(origins []string) {
	s.cors = NewCORSMiddleware(origins)
}

func (s *Server) SetProfile(p *db.Profile) {
	s.profile = p
	s.auth.SetProfile(p)
}

func (s *Server) DB() *db.DB {
	return s.service.db
}

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
