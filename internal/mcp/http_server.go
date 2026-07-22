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

// routeAuth is the access level a route requires. The mux wraps each
// route's handler with the matching AuthMiddleware chain so RequireAuth /
// RequireRole stay the single implementation of auth semantics.
type routeAuth int

const (
	authNone routeAuth = iota
	authRequired
	authReadWrite
)

func (s *Server) httpMux() http.Handler {
	mux := http.NewServeMux()

	routes := []struct {
		pattern string
		handler http.HandlerFunc
		auth    routeAuth
	}{
		{"/api/status", s.handleStatus, authNone},
		{"/api/search", s.handleSearch, authRequired},
		{"/api/set", s.handleSet, authReadWrite},
		{"/api/list", s.handleList, authRequired},
		{"/api/sync/changes", s.handleSyncChanges, authRequired},
		{"/api/sync/apply", s.handleSyncApply, authReadWrite},
		{"/api/sync/relay", s.handleSyncRelay, authReadWrite},
		{"/api/get", s.handleGet, authRequired},
		{"/api/delete", s.handleDelete, authReadWrite},
		{"/api/rules", s.handleRules, authRequired},
		{"/api/entities", s.handleEntities, authRequired},
	}
	for _, rt := range routes {
		var h http.Handler = rt.handler
		switch rt.auth {
		case authReadWrite:
			h = s.auth.RequireAuth(s.auth.RequireRole(security.RoleReadWrite)(h))
		case authRequired:
			h = s.auth.RequireAuth(h)
		}
		mux.Handle(rt.pattern, s.cors.Handler(h))
	}
	mux.HandleFunc("/", s.handleStatic)

	var handler http.Handler = mux
	handler = csrfProtectionHandler(handler)
	handler = securityHeadersHandler(handler)
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
