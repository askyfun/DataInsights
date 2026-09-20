package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(corsMiddleware([]string{"http://localhost:23351"}))
	r.GET("/ping", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", "http://localhost:23351")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:23351" {
		t.Fatalf("expected origin echo, got %q", w.Header().Get("Access-Control-Allow-Origin"))
	}

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req2.Header.Set("Origin", "http://evil.example")
	r.ServeHTTP(w2, req2)
	if w2.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("unexpected origin allowed: %q", w2.Header().Get("Access-Control-Allow-Origin"))
	}
	if got := w2.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("expected Vary: Origin on non-allowed origin response, got %q", got)
	}
}

func TestCORSPreflightAllowedOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(corsMiddleware([]string{"http://localhost:3000", "https://data-insights.example"}))
	r.GET("/ping", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "https://data-insights.example")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected preflight 204, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://data-insights.example" {
		t.Fatalf("expected preflight origin echo, got %q", got)
	}
	if w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Fatal("expected Access-Control-Allow-Methods on preflight")
	}
}

func TestCORSPreflightOriginNotAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(corsMiddleware([]string{"http://localhost:3000"}))
	r.GET("/ping", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Set("Origin", "http://evil.example")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("unexpected preflight origin allowed: %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSEmptyOriginsAllowsAll(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(corsMiddleware(nil))
	r.GET("/ping", func(c *gin.Context) { c.Status(200) })

	for _, origin := range []string{"http://localhost:23351", "http://evil.example"} {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Fatalf("expected wildcard for %q, got %q", origin, got)
		}
	}
}

func TestInitSentrySkipsEmptyDSN(t *testing.T) {
	if initSentry("") {
		t.Fatal("expected initSentry to skip initialization for empty DSN")
	}
}
