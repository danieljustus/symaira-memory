package mcp

import (
	"context"
	"fmt"
	"log/slog"
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
	slog.Info("Symaira Memory API listening", "addr", addr, "url", fmt.Sprintf("http://%s", addr))

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
			writeJSONError(w, http.StatusForbidden, CodeForbidden, "origin not allowed", nil)
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
		writeJSONError(w, http.StatusUnauthorized, CodeForbidden, "missing or invalid Authorization header", nil)
		return nil, false
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	payload, err := s.jwts.VerifyToken(token)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, CodeForbidden, "invalid or expired token", err)
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
			writeJSONError(w, http.StatusForbidden, CodeForbidden, "insufficient permissions: read-only profile", nil)
			return nil, false
		}
		return payload, true
	}
	if payload != nil && payload.Subject != "" && s.db != nil {
		p, err := s.db.GetProfileByName(payload.Subject)
		if err == nil && p != nil {
			if !security.ParseRole(p.Role).CanWrite() && minRole == security.RoleReadWrite {
				writeJSONError(w, http.StatusForbidden, CodeForbidden, "insufficient permissions: read-only profile", nil)
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

	var handler http.Handler = mux
	handler = csrfProtectionHandler(handler)
	handler = securityHeadersHandler(handler)
	handler = RateLimitMiddleware(s.rateLimiter, handler)
	return requestLoggingMiddleware(handler)
}

func csrfProtectionHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST", "DELETE", "PUT", "PATCH":
		default:
			next.ServeHTTP(w, r)
			return
		}

		if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			next.ServeHTTP(w, r)
			return
		}

		origin := r.Header.Get("Origin")
		if origin != "" && isLocalOrigin(origin) {
			next.ServeHTTP(w, r)
			return
		}

		if strings.EqualFold(r.Header.Get("X-Requested-With"), "XMLHttpRequest") {
			next.ServeHTTP(w, r)
			return
		}

		slog.Warn("CSRF blocked request", "method", r.Method, "path", r.URL.Path, "origin", r.Header.Get("Origin"))
		writeJSONError(w, http.StatusForbidden, CodeForbidden, "CSRF validation failed", nil)
	})
}

func isLocalOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	rest := origin
	if strings.HasPrefix(rest, "http://") {
		rest = rest[7:]
	} else if strings.HasPrefix(rest, "https://") {
		rest = rest[8:]
	} else {
		return false
	}
	host := rest
	if idx := strings.LastIndex(host, ":"); idx >= 0 {
		host = host[:idx]
	}
	return host == "127.0.0.1" || host == "localhost" || host == "[::1]" || host == "0.0.0.0"
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

type responseCapture struct {
	http.ResponseWriter
	statusCode int
}

func (rc *responseCapture) WriteHeader(code int) {
	rc.statusCode = code
	rc.ResponseWriter.WriteHeader(code)
}

func requestLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rc := &responseCapture{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rc, r)
		slog.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rc.statusCode,
			"duration", time.Since(start).String(),
		)
	})
}
