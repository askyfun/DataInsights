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
	"path/filepath"
	"syscall"
	"time"

	"data-insights/internal/config"
	"data-insights/internal/database"
	"data-insights/internal/response"
	"data-insights/internal/webui"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
)

func main() {
	var envFile string
	flag.StringVar(&envFile, "env", "", "the .env file (default: probe ./.env then ../.env)")
	flag.Parse()

	// .env 必须在 Load 之前加载：它把值写进进程环境变量，再由 Load 统一读取。
	// 默认探测两个位置，覆盖两种启动方式：从仓库根启动（.env）、从 backend/ 启动（../.env）。
	envCandidates := []string{".env", filepath.Join("..", ".env")}
	if envFile != "" {
		envCandidates = []string{envFile}
	}
	loadedEnv, err := config.LoadDotEnv(envCandidates...)
	if err != nil {
		slog.Error("Failed to load env file", "error", err)
		os.Exit(1)
	}
	if envFile != "" && loadedEnv == "" {
		slog.Error("Env file not found", "path", envFile)
		os.Exit(1)
	}
	if loadedEnv != "" {
		slog.Info("Loaded env file", "path", loadedEnv)
	}

	c := &config.Config{}
	if err := c.Load(); err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// 配置没有文件形式，连接串只能来自环境变量，所以这里必须显式兜住空值，
	// 否则会退化成一个来自驱动层的、看不懂的连接失败。
	if c.Database.Url == "" {
		slog.Error("Database URL is not configured",
			"hint", "set DATABASE_URL (or put it in a .env file)")
		os.Exit(1)
	}

	securityKey, err := resolveSecurityKey(c)
	if err != nil {
		slog.Error("Invalid Security.SecurityKey: expected 32-byte key as 64 hex chars", "error", err)
		os.Exit(1)
	}

	sentryActive := initSentry(c.Sentry.Dsn)
	if sentryActive {
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

	if sentryActive {
		r.Use(newSentryginMiddleware())
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
	SetupRoutes(r, db, securityKey, c.StaticDir != "")

	// 前端静态产物与 API 同进程同端口。配了目录就托管页面，没配就是纯 API 服务
	// （本地开发页面由 Vite dev server 提供，后端无须重复托管一份构建产物）。
	// 两者共用 404 兜底：已注册路由不受影响，其余路径命中文件就返回、
	// 未命中则回落 index.html，/api 等保留前缀回 JSON 404。
	if c.StaticDir != "" {
		ui, err := webui.New(c.StaticDir)
		if err != nil {
			slog.Error("Failed to serve the web UI", "dir", c.StaticDir, "error", err)
			os.Exit(1)
		}
		ui.Mount(r)
		slog.Info("Serving web UI", "dir", c.StaticDir)
	} else {
		slog.Warn("No static UI configured, serving API only",
			"hint", "set STATIC_DIR to the frontend build directory")
	}

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

// corsMiddleware handles cross-origin requests. An empty allowlist allows every
// origin: the platform API has no cookie/session auth, so CORS is not a security
// boundary here, and same-origin single-image deployments never need one. A
// non-empty allowlist echoes only the configured origins.
func corsMiddleware(origins []string) gin.HandlerFunc {
	allowAll := len(origins) == 0
	allowed := make(map[string]struct{}, len(origins))
	for _, o := range origins {
		allowed[o] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			_, inList := allowed[origin]
			if !allowAll {
				// Vary on every response carrying an Origin (hit or miss) so shared
				// caches never serve one origin's CORS verdict to another origin.
				c.Writer.Header().Add("Vary", "Origin")
			}
			if allowAll || inList {
				if allowAll {
					c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				}
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

// newSentryginMiddleware builds the Sentry gin middleware. Repanic is required:
// without it sentrygin swallows panics and never re-raises, so the outer
// gin.Recovery never fires and clients get a 200 with an empty body instead of
// a 500 through the unified response wrapping.
func newSentryginMiddleware() gin.HandlerFunc {
	return sentrygin.New(sentrygin.Options{Repanic: true})
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
