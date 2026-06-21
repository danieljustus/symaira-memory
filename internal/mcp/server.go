package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/danieljustus/symaira-memory/internal/config"
	"github.com/danieljustus/symaira-memory/internal/db"
	"github.com/danieljustus/symaira-memory/internal/extractor"
	"github.com/danieljustus/symaira-memory/internal/security"
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

func writeJSONError(w http.ResponseWriter, status int, safeMsg string, internal error) {
	if internal != nil {
		fmt.Fprintf(os.Stderr, "[HTTP ERROR] %s: %v\n", safeMsg, internal)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": safeMsg})
}

func mcpError(safeMsg string, internal error) (any, error) {
	if internal != nil {
		fmt.Fprintf(os.Stderr, "[MCP ERROR] %s: %v\n", safeMsg, internal)
	}
	return nil, fmt.Errorf("%s", safeMsg)
}
