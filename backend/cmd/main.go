package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dataray/internal/config"
	"dataray/internal/database"
	"dataray/internal/response"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
)

func main() {
	var configFile string
	flag.StringVar(&configFile, "f", "etc/config.toml", "the config file")
	flag.Parse()

	c := &config.Config{}
	if err := c.LoadConfig(configFile); err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	securityKey, err := resolveSecurityKey(c)
	if err != nil {
		slog.Error("Invalid Security.SecurityKey: expected 32-byte key as 64 hex chars", "error", err)
		os.Exit(1)
	}

	if initSentry(c.Sentry.Dsn) {
		defer sentry.Flush(2 * time.Second)
	}

	db, err := database.InitDB(c.Database.Url)
	if err != nil {
		slog.Error("Failed to connect database", "error", err)
		os.Exit(1)
	}

	if err := database.RunMigrations(db); err != nil {
		slog.Error("Failed to run migrations", "error", err)
		os.Exit(1)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	if c.Sentry.Dsn != "" {
		r.Use(sentrygin.New(sentrygin.Options{}))
	}

	// CORS middleware
	r.Use(corsMiddleware(c.CORS.AllowedOrigins))

	// Request ID middleware
	r.Use(requestIDMiddleware())

	// Access log middleware
	r.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start).Milliseconds()
		slog.Info("access", "method", c.Request.Method, "path", c.Request.URL.Path, "duration_ms", duration, "client_ip", c.ClientIP())
	})

	registerHealthRoute(r)

	// Setup routes
	SetupRoutes(r, db, securityKey)

	addr := fmt.Sprintf("%s:%d", c.Host, c.Port)
	slog.Info("Server starting", "addr", addr)

	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Failed to start server", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("Server exited")
}

// registerHealthRoute registers the health check endpoint through the unified response wrapper so
// even non-/api operational endpoints follow the same JSON envelope and null-safety contract.
func registerHealthRoute(r *gin.Engine) {
	r.GET("/health", func(c *gin.Context) {
		response.Success(c, gin.H{"status": "ok"})
	})
}

func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			b := make([]byte, 16)
			if _, err := rand.Read(b); err != nil {
				slog.Error("generate request id failed", "error", err)
				requestID = fmt.Sprintf("%d", time.Now().UnixNano())
			} else {
				requestID = hex.EncodeToString(b)
			}
			c.Request.Header.Set("X-Request-ID", requestID)
		}
		c.Header("X-Request-ID", requestID)
		c.Set("requestID", requestID)
		c.Next()
	}
}

// corsMiddleware returns a middleware that echoes Access-Control-Allow-Origin only
// for origins in the configured allowlist. An empty allowlist defaults to the local
// frontend origin (http://localhost:3000). Origins not in the allowlist receive no
// CORS headers.
func corsMiddleware(origins []string) gin.HandlerFunc {
	if len(origins) == 0 {
		origins = []string{"http://localhost:3000"}
	}
	allowed := make(map[string]struct{}, len(origins))
	for _, o := range origins {
		allowed[o] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if _, ok := allowed[origin]; ok {
				c.Writer.Header().Add("Vary", "Origin")
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH, HEAD")
				c.Writer.Header().Set("Access-Control-Allow-Headers", "*")
				c.Writer.Header().Set("Access-Control-Expose-Headers", "*")
			}
			if c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
		}
		c.Next()
	}
}

// initSentry initializes Sentry when a DSN is configured and reports whether the
// SDK is active. An empty DSN skips initialization entirely so tests and local
// development run without any Sentry side effects; an initialization failure is
// logged but does not stop the server (monitoring must not block serving).
func initSentry(dsn string) bool {
	if dsn == "" {
		return false
	}
	if err := sentry.Init(sentry.ClientOptions{Dsn: dsn}); err != nil {
		slog.Error("Failed to initialize Sentry, continuing without it", "error", err)
		return false
	}
	slog.Info("Sentry initialized")
	return true
}

// resolveSecurityKey decodes the configured 32-byte hex security key.
// An empty value disables encryption (plaintext passthrough) and is reported
// with a warning; a malformed or wrong-length key is an error.
func resolveSecurityKey(c *config.Config) ([]byte, error) {
	if c.Security.SecurityKey == "" {
		slog.Warn("security key not configured: datasource passwords will be stored as plaintext")
		return nil, nil
	}
	key, err := hex.DecodeString(c.Security.SecurityKey)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("expected 32 bytes, got %d", len(key))
	}
	return key, nil
}
