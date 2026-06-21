package mcp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/danieljustus/symaira-memory/internal/security"
)

func (s *Server) StartHTTPServer(port int) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	srv := &http.Server{
		Addr:    addr,
		Handler: s.httpMux(),
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind to %s: %w", addr, err)
	}
	fmt.Fprintf(os.Stderr, "⚡ Symaira Memory API Listening on http://%s\n", addr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

func (s *Server) enableCORS(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	allowed := false
	for _, o := range s.allowedOrigins {
		if matchOrigin(origin, o) {
			allowed = true
			break
		}
	}
	if !allowed {
		if origin != "" {
			http.Error(w, `{"error":"origin not allowed"}`, http.StatusForbidden)
			return true
		}
	} else {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return true
	}
	return false
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) (*security.JWTPayload, bool) {
	if s.jwts == nil {
		return nil, true
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		http.Error(w, `{"error":"missing or invalid Authorization header"}`, http.StatusUnauthorized)
		return nil, false
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	payload, err := s.jwts.VerifyToken(token)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "invalid or expired token", err)
		return nil, false
	}
	return payload, true
}

func (s *Server) requireRole(w http.ResponseWriter, r *http.Request, minRole security.Role) (*security.JWTPayload, bool) {
	payload, ok := s.requireAuth(w, r)
	if !ok {
		return nil, false
	}
	if s.profile != nil {
		if !security.ParseRole(s.profile.Role).CanWrite() && minRole == security.RoleReadWrite {
			writeJSONError(w, http.StatusForbidden, "insufficient permissions: read-only profile", nil)
			return nil, false
		}
		return payload, true
	}
	if payload != nil && payload.Subject != "" && s.db != nil {
		p, err := s.db.GetProfileByName(payload.Subject)
		if err == nil && p != nil {
			if !security.ParseRole(p.Role).CanWrite() && minRole == security.RoleReadWrite {
				writeJSONError(w, http.StatusForbidden, "insufficient permissions: read-only profile", nil)
				return nil, false
			}
		}
	}
	return payload, true
}

func (s *Server) httpMux() http.Handler {
	mux := http.NewServeMux()

	routes := []struct {
		pattern string
		handler http.HandlerFunc
	}{
		{"/api/status", s.handleStatus},
		{"/api/search", s.handleSearch},
		{"/api/set", s.handleSet},
		{"/api/list", s.handleList},
		{"/api/sync/changes", s.handleSyncChanges},
		{"/api/sync/apply", s.handleSyncApply},
		{"/api/get", s.handleGet},
		{"/api/delete", s.handleDelete},
		{"/api/rules", s.handleRules},
		{"/api/entities", s.handleEntities},
		{"/", s.handleStatic},
	}
	for _, rt := range routes {
		mux.HandleFunc(rt.pattern, rt.handler)
	}

	classify := func(r *http.Request) string {
		if strings.HasPrefix(r.URL.Path, "/api/token") || strings.HasPrefix(r.URL.Path, "/api/login") {
			return "auth"
		}
		return "data"
	}

	var handler http.Handler = mux
	handler = securityHeadersHandler(handler)
	return RateLimitMiddleware(s.rateLimiter, handler, classify)
}

func matchOrigin(origin, pattern string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "://*") {
		scheme := strings.TrimSuffix(pattern, "://*")
		return strings.HasPrefix(origin, scheme+"://")
	}
	return origin == pattern
}

func securityHeadersHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}
