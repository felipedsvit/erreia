package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/felipedsvit/erreia/internal/auth"
	"github.com/felipedsvit/erreia/internal/avatar"
	"github.com/felipedsvit/erreia/internal/board"
	"github.com/felipedsvit/erreia/internal/config"
	"github.com/felipedsvit/erreia/internal/database"
	"github.com/felipedsvit/erreia/internal/realtime"
	"github.com/felipedsvit/erreia/internal/session"
	"github.com/felipedsvit/erreia/internal/storage"
	"github.com/felipedsvit/erreia/internal/user"
	"github.com/felipedsvit/erreia/internal/web"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "err", err)
		os.Exit(1)
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Open(rootCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("open database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	logger.Info("database ready")

	store, err := storage.NewMinIO(cfg)
	if err != nil {
		logger.Error("init storage", "err", err)
		os.Exit(1)
	}

	sm := session.NewManager(pool, cfg.SessionCookieName, cfg.SessionLifetime, cfg.SessionCookieSecure)
	users := user.NewRepo(pool)
	authSvc := auth.NewService(users)
	hub := realtime.NewHub(logger)
	listener := realtime.NewListener(cfg.DatabaseURL, cfg.RealtimeChannel, hub, logger)
	publisher := realtime.NewPublisher(pool, cfg.RealtimeChannel)
	boardSvc := board.NewService(pool, publisher)
	avatarUploader := avatar.NewUploader(store)

	go func() {
		listener.Run(rootCtx)
		logger.Info("realtime listener stopped")
	}()
	logger.Info("realtime listener running", "channel", cfg.RealtimeChannel)

	handler := web.New(cfg, logger, sm, web.Dependencies{
		Users:  users,
		Auth:   authSvc,
		Boards: boardSvc,
		Store:  store,
		Hub:    hub,
	}, avatarUploader)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		// WriteTimeout is 0 because SSE connections are long-lived and must
		// not be terminated by a server write timeout. For non-SSE routes,
		// the per-handler code is protected by ReadTimeout (30s) on body
		// reads and IdleTimeout (120s) for keep-alive. A slow-write DoS on
		// non-SSE routes is theoretically possible but mitigated by the
		// 120s idle timeout. Use http.ResponseController for per-route
		// deadlines if tighter control is needed.
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logger.Info("http listening", "addr", cfg.HTTPAddr, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http serve", "err", err)
			stop()
		}
	}()

	<-rootCtx.Done()
	logger.Info("shutdown initiated")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown", "err", err)
	}
	logger.Info("bye")
}
